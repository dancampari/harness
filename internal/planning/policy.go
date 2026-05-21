// Package planning enforces planning-mode policies at the moment a sprint
// contract is proposed. The CLI's setup wizard records the chosen mode in
// .harness/setup.json; this package reads that file and decides which
// structural rules apply.
//
// Modes:
//   - ModeSpecDriven: requires `## Requirements`, REQ-IDs on every
//     criterion, and mechanical Evidence on every criterion.
//   - ModeContract: optional automation, no extra structural rules.
//   - ModeManual: legacy hand-written contracts; no extra rules.
//
// Doctor and contract.Propose use this package so the actual CLI behavior
// matches what the chosen mode promises. Before v0.6 the planning mode
// flag was cosmetic: it only changed agent docs, not validation.
package planning

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/dancampari/harness/internal/planner"
)

const (
	ModeSpecDriven = "spec-driven"
	ModeContract   = "contract"
	ModeManual     = "manual"
	ModeAuto       = "auto"
)

type setupFile struct {
	PlanningMode          string `json:"planning_mode"`
	ContractSkillsEnabled bool   `json:"contract_skills_enabled"`
}

// ReadMode returns the planning mode recorded by `harness setup` for the
// .harness directory rooted at harnessDir. When the file is missing or
// the recorded mode is empty/auto, ReadMode falls back to manual so
// legacy projects keep their pre-v0.6 behavior.
func ReadMode(harnessDir string) string {
	path := filepath.Join(harnessDir, "setup.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return ModeManual
	}
	var s setupFile
	if err := json.Unmarshal(b, &s); err != nil {
		return ModeManual
	}
	mode := normalize(s.PlanningMode)
	if mode == ModeAuto || mode == "" {
		if s.ContractSkillsEnabled {
			return ModeContract
		}
		return ModeManual
	}
	return mode
}

func normalize(mode string) string {
	v := strings.ToLower(strings.TrimSpace(mode))
	switch v {
	case "":
		return ""
	case "spec-driven", "spec_driven", "specdriven":
		return ModeSpecDriven
	case "contract", "contract-only", "contract_only":
		return ModeContract
	case "manual":
		return ModeManual
	case "auto":
		return ModeAuto
	}
	return v
}

// RequiresRequirements reports whether a contract must declare a
// `## Requirements` section under the given mode.
func RequiresRequirements(mode string) bool {
	return normalize(mode) == ModeSpecDriven
}

// RequiresCriterionRequirementID reports whether every acceptance
// criterion must reference a declared REQ-NNN.
func RequiresCriterionRequirementID(mode string) bool {
	return normalize(mode) == ModeSpecDriven
}

// RequiresCriterionEvidence reports whether every acceptance criterion
// must declare mechanical Evidence (tests/e2e/fixture).
func RequiresCriterionEvidence(mode string) bool {
	return normalize(mode) == ModeSpecDriven
}

// RequiresTasks reports whether the sprint must ship a tasks plan at
// .harness/tasks/sprint-NNN.md. Medium and large sprints need explicit
// task decomposition; small sprints do not.
func RequiresTasks(size planner.Size) bool {
	switch size {
	case planner.SizeMedium, planner.SizeLarge:
		return true
	}
	return false
}

// RequiresDesign reports whether the sprint must ship a design doc at
// .harness/design/sprint-NNN.md. Only large sprints require an explicit
// decision record; small and medium sprints can carry decisions inline
// in the contract goal.
func RequiresDesign(size planner.Size) bool {
	return size == planner.SizeLarge
}

// ArtifactPresence tells the policy which planning artifacts the
// caller has on disk. Filled by the agreement manager before policy
// evaluation; tests can construct it directly.
type ArtifactPresence struct {
	HasDesign bool
	HasTasks  bool
}

// ContractPolicyErrors collects policy-specific violations for the
// contract. It is layered on top of planner.Contract.Validate, which
// stays structural-only so legacy contracts keep parsing.
//
// Mode rules (spec-driven) require Requirements, REQ-IDs, and Evidence.
// Size rules (medium/large) require tasks and design files. Rules
// compose: a large spec-driven sprint must satisfy both layers.
func ContractPolicyErrors(mode string, c *planner.Contract) []string {
	return ContractPolicyErrorsWith(mode, c, ArtifactPresence{HasDesign: true, HasTasks: true})
}

// ContractPolicyErrorsWith adds size-based gating that depends on the
// presence of design/tasks artifacts. The agreement manager passes the
// real presence flags; the simpler ContractPolicyErrors variant assumes
// both files exist for callers that do not have file-system context.
func ContractPolicyErrorsWith(mode string, c *planner.Contract, presence ArtifactPresence) []string {
	if c == nil {
		return nil
	}
	var errs []string
	if RequiresRequirements(mode) && len(c.Requirements) == 0 {
		errs = append(errs, "spec-driven mode requires a `## Requirements` section with at least one REQ-NNN entry")
	}
	if RequiresCriterionRequirementID(mode) {
		for _, cr := range c.Criteria {
			if cr.RequirementID == "" {
				errs = append(errs,
					formatCriterion("must declare a REQ-NNN in the REQ column", cr.Number))
			}
		}
	}
	if RequiresCriterionEvidence(mode) {
		for _, cr := range c.Criteria {
			if cr.Evidence.Kind == "" || cr.Evidence.Kind == "inspection" {
				errs = append(errs,
					formatCriterion("must declare mechanical Evidence (tests:/e2e:/fixture:)", cr.Number))
			}
		}
	}
	size := c.EffectiveSize()
	if RequiresTasks(size) && !presence.HasTasks {
		errs = append(errs, "size="+string(size)+" requires .harness/tasks/sprint-NNN.md with the task plan")
	}
	if RequiresDesign(size) && !presence.HasDesign {
		errs = append(errs, "size=large requires .harness/design/sprint-NNN.md with explicit design decisions")
	}
	return errs
}

func formatCriterion(verb string, n int) string {
	return "spec-driven mode: criterion #" + itoa(n) + " " + verb
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if negative {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}
