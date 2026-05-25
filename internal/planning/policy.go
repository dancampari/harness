// Package planning enforces planning-mode policies at the moment a sprint
// contract is proposed. The CLI's setup wizard records the chosen mode in
// .harness/setup.json; this package reads that file and decides which
// structural rules apply.
//
// Modes:
//   - ModeSpecDriven: requires `## Requirements`, REQ-IDs on every
//     criterion, mechanical Evidence on every criterion, plus TLC's
//     specify.md structural patterns (WHEN/THEN/SHALL acceptance form,
//     an Edge Cases section, and an Out of Scope section).
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
	"regexp"
	"strconv"
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
//
// TaskPlanPath, when non-empty, lets the policy load and lint tasks.md
// for TLC's granularity check and diagram-definition cross-check. The
// TestingMatrixPath, when non-empty, points at .specs/codebase/
// TESTING.md so the test co-location validator can verify each task's
// Tests: field references a path declared in the coverage matrix.
type ArtifactPresence struct {
	HasDesign         bool
	HasTasks          bool
	TaskPlanPath      string
	TestingMatrixPath string
}

// MaxFilesPerTask is the granularity threshold TLC's tasks.md describes:
// each task should touch one component, one function, or one endpoint.
// We allow up to three paths so a task can name its implementation file,
// its test file, and one fixture without tripping the gate. Anything
// larger must explicitly carry `Cohesive: true` to opt out.
const MaxFilesPerTask = 3

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
		errs = append(errs, "size="+string(size)+" requires a tasks plan at .specs/features/sprint-NNN/tasks.md (or legacy .harness/tasks/sprint-NNN.md)")
	}
	if RequiresDesign(size) && !presence.HasDesign {
		errs = append(errs, "size=large requires a design doc at .specs/features/sprint-NNN/design.md (or legacy .harness/design/sprint-NNN.md) with explicit decisions")
	}
	if normalize(mode) == ModeSpecDriven {
		errs = append(errs, planner.TemplatePlaceholderErrors(c.RawMarkdown)...)
		errs = append(errs, tlcSpecPatternErrors(c)...)
		errs = append(errs, tlcTaskGranularityErrors(presence.TaskPlanPath)...)
		errs = append(errs, tlcDiagramDefinitionErrors(presence.TaskPlanPath)...)
		errs = append(errs, tlcTestCoLocationErrors(presence.TaskPlanPath, presence.TestingMatrixPath)...)
	}
	return errs
}

// tlcTaskGranularityErrors reads tasks.md (when present) and reports
// tasks that violate TLC's granularity rule: one component / one
// function / one endpoint per task. Tasks that legitimately span more
// files can opt out by adding `Cohesive: true`.
//
// Empty path or missing file is a no-op: the size-based RequiresTasks
// gate handles "missing tasks plan" separately.
func tlcTaskGranularityErrors(taskPlanPath string) []string {
	if taskPlanPath == "" {
		return nil
	}
	plan, err := planner.ParseTasks(taskPlanPath)
	if err != nil || plan == nil || len(plan.Tasks) == 0 {
		return nil
	}
	var errs []string
	for _, t := range plan.Tasks {
		if t.Cohesive {
			continue
		}
		if len(t.Where) > MaxFilesPerTask {
			errs = append(errs, formatTask("touches "+itoa(len(t.Where))+" files (max "+itoa(MaxFilesPerTask)+" without `Cohesive: true`); split into smaller tasks per TLC's granularity rule", t.Number))
		}
	}
	return errs
}

func formatTask(verb string, n int) string {
	return "spec-driven mode: task #" + itoa(n) + " " + verb
}

// tlcDiagramDefinitionErrors implements TLC tasks.md's Diagram-
// Definition Cross-Check: every edge declared in the embedded
// dependency diagram (Mermaid or ASCII) must match the union of
// `Depends on:` lines across tasks, and vice versa. Mismatches are
// reported in both directions so the agent can repair either side.
//
// No diagram or no Depends-on metadata is a no-op — the check only
// fires when the spec is making a claim it could verify.
func tlcDiagramDefinitionErrors(taskPlanPath string) []string {
	if taskPlanPath == "" {
		return nil
	}
	plan, err := planner.ParseTasks(taskPlanPath)
	if err != nil || plan == nil || len(plan.Tasks) == 0 {
		return nil
	}
	if len(plan.Diagram.Edges) == 0 {
		return nil
	}

	known := taskIDSet(plan)
	declared := dependencyEdgesFromTasks(plan)
	diagram := canonicalEdges(plan.Diagram.Edges)

	var errs []string
	for edge := range diagram {
		if _, ok := declared[edge]; ok {
			continue
		}
		errs = append(errs,
			"diagram-definition mismatch: edge "+edge.From+" → "+edge.To+
				" appears in the tasks.md diagram but not in any task's `Depends on:` field")
	}
	for edge := range declared {
		if _, ok := diagram[edge]; ok {
			continue
		}
		errs = append(errs,
			"diagram-definition mismatch: dependency "+edge.From+" → "+edge.To+
				" is declared by a task's `Depends on:` field but missing from the tasks.md diagram")
	}
	// Edges pointing at task IDs that no task declares mean the diagram
	// is stale; report once per orphan.
	for edge := range diagram {
		if edge.From != "" && !known[edge.From] {
			errs = append(errs,
				"diagram-definition mismatch: diagram references task `"+edge.From+
					"` that has no matching `## Task` heading")
		}
		if edge.To != "" && !known[edge.To] {
			errs = append(errs,
				"diagram-definition mismatch: diagram references task `"+edge.To+
					"` that has no matching `## Task` heading")
		}
	}
	return uniqueStrings(errs)
}

type canonicalEdge struct {
	From string
	To   string
}

func canonicalEdges(edges []planner.DependencyEdge) map[canonicalEdge]struct{} {
	out := map[canonicalEdge]struct{}{}
	for _, e := range edges {
		if e.From == "" || e.To == "" {
			continue
		}
		out[canonicalEdge{From: e.From, To: e.To}] = struct{}{}
	}
	return out
}

func dependencyEdgesFromTasks(plan *planner.TaskPlan) map[canonicalEdge]struct{} {
	out := map[canonicalEdge]struct{}{}
	for _, t := range plan.Tasks {
		to := normalisePolicyTaskID(itoa(t.Number))
		for _, dep := range t.DependsOn {
			from := normalisePolicyTaskID(dep)
			if from == "" {
				continue
			}
			out[canonicalEdge{From: from, To: to}] = struct{}{}
		}
	}
	return out
}

func taskIDSet(plan *planner.TaskPlan) map[string]bool {
	out := map[string]bool{}
	for _, t := range plan.Tasks {
		out[normalisePolicyTaskID(itoa(t.Number))] = true
	}
	return out
}

// normalisePolicyTaskID mirrors planner.normaliseTaskID without
// importing the unexported helper — task IDs in the policy come from
// either parsed integers or markdown text, so we coerce them to the
// same canonical form (leading-zeros stripped, optional `T` removed).
func normalisePolicyTaskID(raw string) string {
	id := strings.TrimSpace(raw)
	if id == "" {
		return ""
	}
	if len(id) > 1 && (id[0] == 'T' || id[0] == 't') {
		id = id[1:]
	}
	id = strings.TrimLeft(id, "0")
	if id == "" {
		return "0"
	}
	return id
}

func uniqueStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// tlcTestCoLocationErrors implements TLC tasks.md's Test Co-location
// Validation. For every task that declares a `Tests:` value, we
// require at least one substring of that value to appear in
// .specs/codebase/TESTING.md so the coverage matrix actually
// references the test the task plans to add or extend.
//
// Both arguments are advisory: missing TESTING.md or missing Tests:
// lines fall through without complaint so legacy projects keep
// working until the matrix is authored.
func tlcTestCoLocationErrors(taskPlanPath, testingMatrixPath string) []string {
	if taskPlanPath == "" || testingMatrixPath == "" {
		return nil
	}
	matrix, err := os.ReadFile(testingMatrixPath)
	if err != nil {
		return nil
	}
	plan, err := planner.ParseTasks(taskPlanPath)
	if err != nil || plan == nil || len(plan.Tasks) == 0 {
		return nil
	}
	body := string(matrix)
	if strings.TrimSpace(body) == "" {
		return nil
	}
	var errs []string
	for _, t := range plan.Tasks {
		raw := strings.TrimSpace(t.Tests)
		if raw == "" {
			continue
		}
		tokens := splitTestsField(raw)
		matched := false
		for _, tok := range tokens {
			if tok == "" {
				continue
			}
			if strings.Contains(body, tok) {
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		errs = append(errs,
			formatTask("`Tests:` value "+strconv.Quote(raw)+
				" is not referenced anywhere in .specs/codebase/TESTING.md; declare the coverage in the matrix or fix the task's Tests field",
				t.Number))
	}
	return errs
}

// splitTestsField breaks a Tests: field into substrings the matrix
// might contain. The split is intentionally liberal — commas, slashes,
// pipes, and quotes are all treated as separators so a task can write
// `tests/auth/user.test.ts | tests user-auth happy path` and we still
// look for both halves.
func splitTestsField(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	separators := []string{",", "|", ";"}
	parts := []string{value}
	for _, sep := range separators {
		var next []string
		for _, p := range parts {
			for _, fragment := range strings.Split(p, sep) {
				fragment = strings.TrimSpace(fragment)
				if fragment != "" {
					next = append(next, fragment)
				}
			}
		}
		parts = next
	}
	// Also include the raw value so substring matches on the full
	// string still satisfy the gate.
	parts = append(parts, value)
	return parts
}

// tlcSpecPatternErrors enforces the structural patterns TLC's specify.md
// describes as mandatory for spec-driven work:
//
//   - Every acceptance criterion is in the WHEN/THEN/SHALL form so the
//     condition, action, and observable outcome are unambiguous.
//   - The contract has an `## Edge Cases` section listing the boundary
//     and failure scenarios. TLC treats an empty edge case list as a sign
//     the spec is under-considered.
//   - The contract has an `## Out of Scope` section so deferred work is
//     explicit rather than ambient.
//
// These are gates: contract.Propose blocks until the agent fixes them.
// Strings are matched case-insensitively and tolerate the alternate
// spellings TLC uses ("Out-of-Scope", "Edge-Cases", etc.).
func tlcSpecPatternErrors(c *planner.Contract) []string {
	var errs []string
	for _, cr := range c.Criteria {
		if !isWhenThenShall(cr.Statement) {
			errs = append(errs,
				formatCriterion("must be in WHEN/THEN/SHALL form (see TLC specify.md acceptance criteria pattern)", cr.Number))
		}
	}
	if !contractHasSection(c, edgeCasesSectionPatterns) {
		errs = append(errs, "spec-driven mode requires an `## Edge Cases` section listing boundary and failure scenarios")
	}
	if !contractHasSection(c, outOfScopeSectionPatterns) {
		errs = append(errs, "spec-driven mode requires an `## Out of Scope` section so deferred work is explicit")
	}
	return errs
}

// whenThenShallRe accepts the canonical TLC pattern plus minor variants:
//
//	"WHEN <action> THEN system SHALL <outcome>"
//	"WHEN <action> THEN <subject> SHALL <outcome>"
//	"GIVEN ... WHEN ... THEN ... SHALL ..."
//
// All three tokens (WHEN, THEN, SHALL) must be present and ordered so the
// criterion expresses a precondition, action, and observable outcome.
var whenThenShallRe = regexp.MustCompile(`(?i)\bWHEN\b[\s\S]+?\bTHEN\b[\s\S]+?\bSHALL\b`)

func isWhenThenShall(statement string) bool {
	return whenThenShallRe.MatchString(statement)
}

var edgeCasesSectionPatterns = []string{
	"edge cases",
	"edge-cases",
	"edge case",
}

var outOfScopeSectionPatterns = []string{
	"out of scope",
	"out-of-scope",
	"non-goals",
	"non goals",
}

// contractHasSection scans the raw markdown for any `## <heading>` line
// whose normalised text matches one of patterns. Returns true on first
// match. Falls back to false when RawMarkdown is empty (legacy
// constructed-in-tests contracts).
func contractHasSection(c *planner.Contract, patterns []string) bool {
	if c == nil || c.RawMarkdown == "" {
		return false
	}
	for _, raw := range strings.Split(c.RawMarkdown, "\n") {
		trimmed := strings.TrimSpace(raw)
		if !strings.HasPrefix(trimmed, "##") {
			continue
		}
		heading := strings.ToLower(strings.TrimSpace(strings.TrimLeft(trimmed, "#")))
		for _, p := range patterns {
			if strings.Contains(heading, p) {
				return true
			}
		}
	}
	return false
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
