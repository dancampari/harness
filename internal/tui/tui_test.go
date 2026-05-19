package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestViewRendersDashboardSections(t *testing.T) {
	root := t.TempDir()
	harnessDir := filepath.Join(root, ".harness")
	if err := os.MkdirAll(filepath.Join(harnessDir, "contracts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(harnessDir, "reports"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(harnessDir, "evaluations"), 0o755); err != nil {
		t.Fatal(err)
	}
	contract := `# Sprint 001 - demo

## Goal
Ship a demo dashboard.

## Deliverables
- ` + "`src/index.ts`" + `

## Acceptance Criteria
| # | Criterion | Threshold |
|---|-----------|-----------|
| 1 | Works | 8/10 |
`
	if err := os.WriteFile(filepath.Join(harnessDir, "contracts", "sprint-001.md"), []byte(contract), 0o644); err != nil {
		t.Fatal(err)
	}
	report := `{
  "schema_version": "2",
  "sprint_number": 1,
  "total_score": 98,
  "verdict": "PASS",
  "dimensions": {
    "contract": {
      "dimension": "contract",
      "score": 100,
      "threshold": 80,
      "passed": true,
      "sensors_used": ["contract-validator"]
    }
  },
  "duration_seconds": 2.4
}`
	if err := os.WriteFile(filepath.Join(harnessDir, "reports", "sprint-001.json"), []byte(report), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(harnessDir, "reports", "latest.json"), []byte(report), 0o644); err != nil {
		t.Fatal(err)
	}

	m := newModel(harnessDir, true)
	m.width = 100
	view := m.View()
	for _, expected := range []string{
		"harness",
		"Autonomous Development Pipeline",
		"Sprints",
		"Activity",
		"QA PASS",
		"score 98/100",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("expected view to contain %q\n%s", expected, view)
		}
	}
}
