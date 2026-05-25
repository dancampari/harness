package planning

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const tasksWithMermaidDiagram = `# Tasks

` + "```" + `mermaid
graph LR
  T1 --> T2
  T2 --> T3
` + "```" + `

## Task 001 — wire foundation
- Where: a.ts, a.test.ts

## Task 002 — build feature
- Where: b.ts, b.test.ts
- Depends on: 001

## Task 003 — connect to feature
- Where: c.ts, c.test.ts
- Depends on: 002
`

func TestDiagramDefinitionAcceptsMatchingMermaid(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "tasks.md")
	if err := os.WriteFile(path, []byte(tasksWithMermaidDiagram), 0o644); err != nil {
		t.Fatal(err)
	}
	c := compliantSpecDrivenContract()
	presence := ArtifactPresence{HasDesign: true, HasTasks: true, TaskPlanPath: path}
	errs := ContractPolicyErrorsWith(ModeSpecDriven, c, presence)
	for _, e := range errs {
		if strings.Contains(e, "diagram-definition") {
			t.Fatalf("expected no diagram-definition errors, got: %v", errs)
		}
	}
}

const tasksWithExtraEdgeInDiagram = `# Tasks

` + "```" + `mermaid
graph LR
  T1 --> T2
  T2 --> T3
  T2 --> T4
` + "```" + `

## Task 001 — wire foundation
- Where: a.ts, a.test.ts

## Task 002 — build feature
- Where: b.ts, b.test.ts
- Depends on: 001

## Task 003 — connect to feature
- Where: c.ts, c.test.ts
- Depends on: 002
`

func TestDiagramDefinitionDetectsOrphanEdge(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "tasks.md")
	if err := os.WriteFile(path, []byte(tasksWithExtraEdgeInDiagram), 0o644); err != nil {
		t.Fatal(err)
	}
	c := compliantSpecDrivenContract()
	presence := ArtifactPresence{HasDesign: true, HasTasks: true, TaskPlanPath: path}
	errs := ContractPolicyErrorsWith(ModeSpecDriven, c, presence)
	joined := strings.Join(errs, "\n")
	if !strings.Contains(joined, "2 → 4") {
		t.Fatalf("expected mismatch on edge 2 → 4, got: %v", errs)
	}
	if !strings.Contains(joined, "task `4`") {
		t.Fatalf("expected orphan-task warning for `4`, got: %v", errs)
	}
}

const tasksMissingDeclaredEdgeInDiagram = `# Tasks

` + "```" + `mermaid
graph LR
  T1 --> T2
` + "```" + `

## Task 001 — wire foundation
- Where: a.ts, a.test.ts

## Task 002 — build feature
- Where: b.ts, b.test.ts
- Depends on: 001

## Task 003 — connect to feature
- Where: c.ts, c.test.ts
- Depends on: 002
`

func TestDiagramDefinitionDetectsMissingEdgeInDiagram(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "tasks.md")
	if err := os.WriteFile(path, []byte(tasksMissingDeclaredEdgeInDiagram), 0o644); err != nil {
		t.Fatal(err)
	}
	c := compliantSpecDrivenContract()
	presence := ArtifactPresence{HasDesign: true, HasTasks: true, TaskPlanPath: path}
	errs := ContractPolicyErrorsWith(ModeSpecDriven, c, presence)
	joined := strings.Join(errs, "\n")
	if !strings.Contains(joined, "missing from the tasks.md diagram") {
		t.Fatalf("expected missing-edge mismatch, got: %v", errs)
	}
}

const tasksWithASCIIDiagram = `# Tasks

` + "```" + `
T1 → T2 → T3
` + "```" + `

## Task 001 — wire foundation
- Where: a.ts, a.test.ts

## Task 002 — build feature
- Where: b.ts, b.test.ts
- Depends on: 001

## Task 003 — connect to feature
- Where: c.ts, c.test.ts
- Depends on: 002
`

func TestDiagramDefinitionParsesASCIIDiagram(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "tasks.md")
	if err := os.WriteFile(path, []byte(tasksWithASCIIDiagram), 0o644); err != nil {
		t.Fatal(err)
	}
	c := compliantSpecDrivenContract()
	presence := ArtifactPresence{HasDesign: true, HasTasks: true, TaskPlanPath: path}
	errs := ContractPolicyErrorsWith(ModeSpecDriven, c, presence)
	for _, e := range errs {
		if strings.Contains(e, "diagram-definition") {
			t.Fatalf("ASCII diagram should validate cleanly, got: %v", errs)
		}
	}
}

func TestDiagramDefinitionNoOpWhenNoDiagram(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "tasks.md")
	if err := os.WriteFile(path, []byte("# Tasks\n\n## Task 001 — solo\n- Where: a.ts, a.test.ts\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := compliantSpecDrivenContract()
	presence := ArtifactPresence{HasDesign: true, HasTasks: true, TaskPlanPath: path}
	errs := ContractPolicyErrorsWith(ModeSpecDriven, c, presence)
	for _, e := range errs {
		if strings.Contains(e, "diagram-definition") {
			t.Fatalf("expected no-op without diagram, got: %v", errs)
		}
	}
}

const testingMatrixBody = `# Test Coverage Matrix

| Component | Unit Tests                 | Integration Tests       |
|-----------|----------------------------|-------------------------|
| auth      | tests/auth/user.test.ts    | tests/integration/auth  |
| billing   | tests/billing/invoice.test | tests/integration/billing |
`

const tasksWithTestsReferences = `# Tasks

## Task 001 — auth feature
- Where: src/auth/user.ts
- Tests: tests/auth/user.test.ts

## Task 002 — billing feature
- Where: src/billing/invoice.ts
- Tests: tests/billing/invoice.test
`

func TestTestCoLocationAcceptsTasksWithMatrixReferences(t *testing.T) {
	root := t.TempDir()
	matrixPath := filepath.Join(root, "TESTING.md")
	tasksPath := filepath.Join(root, "tasks.md")
	if err := os.WriteFile(matrixPath, []byte(testingMatrixBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tasksPath, []byte(tasksWithTestsReferences), 0o644); err != nil {
		t.Fatal(err)
	}
	c := compliantSpecDrivenContract()
	presence := ArtifactPresence{HasDesign: true, HasTasks: true, TaskPlanPath: tasksPath, TestingMatrixPath: matrixPath}
	errs := ContractPolicyErrorsWith(ModeSpecDriven, c, presence)
	for _, e := range errs {
		if strings.Contains(e, "TESTING.md") {
			t.Fatalf("expected no co-location errors, got: %v", errs)
		}
	}
}

const tasksWithUntrackedTestRef = `# Tasks

## Task 001 — orphaned tests
- Where: src/auth/user.ts
- Tests: tests/never/declared/in/matrix.test.ts
`

func TestTestCoLocationFlagsTaskWithoutMatrixReference(t *testing.T) {
	root := t.TempDir()
	matrixPath := filepath.Join(root, "TESTING.md")
	tasksPath := filepath.Join(root, "tasks.md")
	if err := os.WriteFile(matrixPath, []byte(testingMatrixBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tasksPath, []byte(tasksWithUntrackedTestRef), 0o644); err != nil {
		t.Fatal(err)
	}
	c := compliantSpecDrivenContract()
	presence := ArtifactPresence{HasDesign: true, HasTasks: true, TaskPlanPath: tasksPath, TestingMatrixPath: matrixPath}
	errs := ContractPolicyErrorsWith(ModeSpecDriven, c, presence)
	joined := strings.Join(errs, "\n")
	if !strings.Contains(joined, "TESTING.md") || !strings.Contains(joined, "task #1") {
		t.Fatalf("expected co-location error for task 1, got: %v", errs)
	}
}

func TestTestCoLocationSkippedWhenMatrixPathEmpty(t *testing.T) {
	root := t.TempDir()
	tasksPath := filepath.Join(root, "tasks.md")
	if err := os.WriteFile(tasksPath, []byte(tasksWithUntrackedTestRef), 0o644); err != nil {
		t.Fatal(err)
	}
	c := compliantSpecDrivenContract()
	presence := ArtifactPresence{HasDesign: true, HasTasks: true, TaskPlanPath: tasksPath}
	errs := ContractPolicyErrorsWith(ModeSpecDriven, c, presence)
	for _, e := range errs {
		if strings.Contains(e, "TESTING.md") {
			t.Fatalf("expected co-location check to skip without matrix path, got: %v", errs)
		}
	}
}
