package planning

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dancampari/harness/internal/planner"
)

func TestReadModeDefaultsToManual(t *testing.T) {
	dir := t.TempDir()
	if got := ReadMode(dir); got != ModeManual {
		t.Fatalf("expected manual default, got %q", got)
	}
}

func TestReadModeFromSetupJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "setup.json"),
		[]byte(`{"planning_mode":"spec-driven"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ReadMode(dir); got != ModeSpecDriven {
		t.Fatalf("expected spec-driven, got %q", got)
	}
}

func TestReadModeAutoFallsBackToContractWhenSkillsEnabled(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "setup.json"),
		[]byte(`{"planning_mode":"auto","contract_skills_enabled":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ReadMode(dir); got != ModeContract {
		t.Fatalf("expected contract, got %q", got)
	}
}

func TestPolicyErrorsRejectSpecDrivenContractWithoutRequirements(t *testing.T) {
	c := &planner.Contract{
		SprintNumber: 1,
		Title:        "weak spec-driven contract",
		Goal:         "ship something",
		Deliverables: []planner.Deliverable{{Path: "x.ts"}},
		Criteria: []planner.AcceptanceCriterion{
			{Number: 1, Statement: "does the thing", Threshold: 8},
		},
	}
	errs := ContractPolicyErrors(ModeSpecDriven, c)
	if len(errs) == 0 {
		t.Fatal("expected spec-driven policy errors for criterion without REQ-ID or Evidence")
	}
	joined := strings.Join(errs, "\n")
	for _, want := range []string{
		"requires a `## Requirements` section",
		"must declare a REQ-NNN",
		"must declare mechanical Evidence",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("expected policy error to contain %q, got:\n%s", want, joined)
		}
	}
}

func TestPolicyErrorsAcceptCompliantSpecDrivenContract(t *testing.T) {
	c := &planner.Contract{
		SprintNumber: 2,
		Title:        "compliant spec-driven contract",
		Goal:         "ship traceable feature",
		Requirements: []planner.Requirement{
			{ID: "REQ-001", Statement: "ships"},
		},
		Deliverables: []planner.Deliverable{
			{Path: "x.ts", RequirementID: "REQ-001"},
		},
		Criteria: []planner.AcceptanceCriterion{
			{
				Number:        1,
				RequirementID: "REQ-001",
				Statement:     "WHEN the agent runs the build THEN the system SHALL emit x.ts",
				Evidence:      planner.Evidence{Kind: "tests", Ref: "delivers x"},
				Threshold:     8,
			},
		},
		RawMarkdown: "# Sprint 002\n\n## Edge Cases\n- empty input\n\n## Out of Scope\n- migrations\n",
	}
	if errs := ContractPolicyErrors(ModeSpecDriven, c); len(errs) != 0 {
		t.Fatalf("expected no policy errors, got %v", errs)
	}
}

func TestPolicyErrorsRejectSpecDrivenContractWithoutWhenThenShall(t *testing.T) {
	c := &planner.Contract{
		SprintNumber: 3,
		Title:        "non-tlc criterion",
		Goal:         "ship",
		Requirements: []planner.Requirement{{ID: "REQ-001", Statement: "ships"}},
		Deliverables: []planner.Deliverable{{Path: "x.ts", RequirementID: "REQ-001"}},
		Criteria: []planner.AcceptanceCriterion{{
			Number:        1,
			RequirementID: "REQ-001",
			Statement:     "the system delivers x",
			Evidence:      planner.Evidence{Kind: "tests", Ref: "delivers x"},
			Threshold:     8,
		}},
		RawMarkdown: "# Sprint 003\n\n## Edge Cases\n- empty input\n\n## Out of Scope\n- migrations\n",
	}
	errs := ContractPolicyErrors(ModeSpecDriven, c)
	joined := strings.Join(errs, "\n")
	if !strings.Contains(joined, "WHEN/THEN/SHALL") {
		t.Fatalf("expected WHEN/THEN/SHALL violation, got: %v", errs)
	}
}

func TestPolicyErrorsRejectSpecDrivenContractMissingEdgeCases(t *testing.T) {
	c := &planner.Contract{
		SprintNumber: 4,
		Title:        "no edge cases",
		Goal:         "ship",
		Requirements: []planner.Requirement{{ID: "REQ-001", Statement: "ships"}},
		Deliverables: []planner.Deliverable{{Path: "x.ts", RequirementID: "REQ-001"}},
		Criteria: []planner.AcceptanceCriterion{{
			Number:        1,
			RequirementID: "REQ-001",
			Statement:     "WHEN built THEN system SHALL emit x",
			Evidence:      planner.Evidence{Kind: "tests", Ref: "delivers x"},
			Threshold:     8,
		}},
		RawMarkdown: "# Sprint 004\n\n## Out of Scope\n- migrations\n",
	}
	errs := ContractPolicyErrors(ModeSpecDriven, c)
	joined := strings.Join(errs, "\n")
	if !strings.Contains(joined, "Edge Cases") {
		t.Fatalf("expected Edge Cases violation, got: %v", errs)
	}
}

func TestPolicyErrorsSkippedForManualMode(t *testing.T) {
	c := &planner.Contract{
		SprintNumber: 3,
		Title:        "legacy",
		Goal:         "ship something",
		Deliverables: []planner.Deliverable{{Path: "x.ts"}},
		Criteria: []planner.AcceptanceCriterion{
			{Number: 1, Statement: "does the thing", Threshold: 8},
		},
	}
	if errs := ContractPolicyErrors(ModeManual, c); len(errs) != 0 {
		t.Fatalf("expected manual mode to skip policy checks, got %v", errs)
	}
	if errs := ContractPolicyErrors(ModeContract, c); len(errs) != 0 {
		t.Fatalf("expected contract mode to skip policy checks, got %v", errs)
	}
}

func TestSizePolicyRequiresTasksForMedium(t *testing.T) {
	c := &planner.Contract{
		SprintNumber: 1,
		Title:        "medium with no tasks",
		Goal:         "ship something medium-sized",
		Size:         planner.SizeMedium,
		Deliverables: []planner.Deliverable{{Path: "x.ts"}},
		Criteria: []planner.AcceptanceCriterion{
			{Number: 1, Statement: "works", Threshold: 8},
		},
	}
	errs := ContractPolicyErrorsWith(ModeManual, c, ArtifactPresence{HasTasks: false})
	found := false
	for _, e := range errs {
		if strings.Contains(e, "size=medium requires") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected size=medium policy error, got %v", errs)
	}
}

func TestSizePolicyRequiresDesignAndTasksForLarge(t *testing.T) {
	c := &planner.Contract{
		SprintNumber: 1,
		Title:        "large with no design",
		Goal:         "ship a large feature",
		Size:         planner.SizeLarge,
		Deliverables: []planner.Deliverable{{Path: "x.ts"}},
		Criteria: []planner.AcceptanceCriterion{
			{Number: 1, Statement: "works", Threshold: 8},
		},
	}
	errs := ContractPolicyErrorsWith(ModeManual, c, ArtifactPresence{HasTasks: true, HasDesign: false})
	foundDesign := false
	for _, e := range errs {
		if strings.Contains(e, "size=large requires") {
			foundDesign = true
		}
	}
	if !foundDesign {
		t.Fatalf("expected size=large design policy error, got %v", errs)
	}
}

func TestSizePolicyAllowsSmallWithoutExtraArtifacts(t *testing.T) {
	c := &planner.Contract{
		SprintNumber: 1,
		Title:        "small contract",
		Goal:         "ship a small thing",
		Size:         planner.SizeSmall,
		Deliverables: []planner.Deliverable{{Path: "x.ts"}},
		Criteria: []planner.AcceptanceCriterion{
			{Number: 1, Statement: "works", Threshold: 8},
		},
	}
	if errs := ContractPolicyErrorsWith(ModeManual, c, ArtifactPresence{}); len(errs) != 0 {
		t.Fatalf("small sprint should not require design/tasks, got %v", errs)
	}
}

func TestSizePolicyIsSkippedWhenSizeUndeclared(t *testing.T) {
	c := &planner.Contract{
		SprintNumber: 1,
		Title:        "legacy contract",
		Goal:         "no size declared, legacy behavior",
		Deliverables: []planner.Deliverable{{Path: "x.ts"}},
		Criteria: []planner.AcceptanceCriterion{
			{Number: 1, Statement: "works", Threshold: 8},
		},
	}
	if errs := ContractPolicyErrorsWith(ModeManual, c, ArtifactPresence{}); len(errs) != 0 {
		t.Fatalf("legacy contracts without Size should pass policy, got %v", errs)
	}
}

func TestInspectionEvidenceFailsSpecDrivenPolicy(t *testing.T) {
	c := &planner.Contract{
		SprintNumber: 4,
		Title:        "inspection-only",
		Goal:         "needs mechanical evidence",
		Requirements: []planner.Requirement{{ID: "REQ-001", Statement: "x"}},
		Deliverables: []planner.Deliverable{{Path: "x.ts", RequirementID: "REQ-001"}},
		Criteria: []planner.AcceptanceCriterion{
			{
				Number:        1,
				RequirementID: "REQ-001",
				Statement:     "look at it",
				Evidence:      planner.Evidence{Kind: "inspection", Ref: "manual review"},
				Threshold:     8,
			},
		},
	}
	errs := ContractPolicyErrors(ModeSpecDriven, c)
	if len(errs) == 0 {
		t.Fatal("expected inspection-only criterion to fail spec-driven policy")
	}
	if !strings.Contains(errs[0], "mechanical Evidence") {
		t.Fatalf("expected mechanical Evidence message, got %v", errs)
	}
}
