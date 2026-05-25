package planning

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dancampari/harness/internal/planner"
)

func TestTaskGranularityRejectsTaskTouchingTooManyFiles(t *testing.T) {
	tmp := t.TempDir()
	tasksPath := filepath.Join(tmp, "tasks.md")
	body := `# Tasks

## Task 001 — wire too much at once
- Where: a.ts, b.ts, c.ts, d.ts
- Tests: covers a-d

## Task 002 — wire a single thing
- Where: e.ts, e.test.ts

## Task 003 — explicitly cohesive multi-file change
- Where: f.ts, g.ts, h.ts, i.ts, j.ts
- Cohesive: true
`
	if err := os.WriteFile(tasksPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	c := compliantSpecDrivenContract()
	presence := ArtifactPresence{HasDesign: true, HasTasks: true, TaskPlanPath: tasksPath}
	errs := ContractPolicyErrorsWith(ModeSpecDriven, c, presence)
	joined := strings.Join(errs, "\n")
	if !strings.Contains(joined, "task #1") || !strings.Contains(joined, "granularity") {
		t.Fatalf("expected granularity error for task #1, got: %v", errs)
	}
	if strings.Contains(joined, "task #2") {
		t.Fatalf("did not expect a granularity error for the small task, got: %v", errs)
	}
	if strings.Contains(joined, "task #3") {
		t.Fatalf("Cohesive: true should opt out of the granularity check, got: %v", errs)
	}
}

func TestTaskGranularitySkippedWhenTaskPlanPathEmpty(t *testing.T) {
	c := compliantSpecDrivenContract()
	if errs := ContractPolicyErrorsWith(ModeSpecDriven, c, ArtifactPresence{HasDesign: true, HasTasks: true}); len(errs) != 0 {
		t.Fatalf("expected no errors when no task plan path is supplied, got: %v", errs)
	}
}

func compliantSpecDrivenContract() *planner.Contract {
	return &planner.Contract{
		SprintNumber: 1,
		Title:        "compliant",
		Goal:         "ship traceable feature",
		Requirements: []planner.Requirement{{ID: "REQ-001", Statement: "ships"}},
		Deliverables: []planner.Deliverable{{Path: "x.ts", RequirementID: "REQ-001"}},
		Criteria: []planner.AcceptanceCriterion{{
			Number:        1,
			RequirementID: "REQ-001",
			Statement:     "WHEN built THEN system SHALL emit x",
			Evidence:      planner.Evidence{Kind: "tests", Ref: "delivers x"},
			Threshold:     8,
		}},
		RawMarkdown: "# Sprint 001\n\n## Edge Cases\n- empty input\n\n## Out of Scope\n- migrations\n",
	}
}
