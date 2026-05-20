package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

	m := newModel(harnessDir, true, "0.0.0-test")
	m.width = 100
	view := m.View()
	for _, expected := range []string{
		"harness",
		"Autonomous Development Pipeline",
		"v0.0.0-test",
		"Sprints",
		"Verdict",
		"Activity",
		"watching .harness",
		"QA PASS",
		"score 98/100",
		"contract-validator",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("expected view to contain %q\n%s", expected, view)
		}
	}
}

func TestSprintTableKeepsGoalInFixedColumn(t *testing.T) {
	row := sprintRow{
		Number:   1,
		Goal:     "Criar app Vite React todo-local-test com um nome longo que nao deve empurrar a tabela",
		Contract: "AGREED",
		Build:    "DONE",
		QA:       "PASS",
		Score:    "100",
		Time:     "4ms",
		Findings: 0,
		Scored:   true,
	}
	rendered := renderSprintHeader(84) + "\n" + renderSprintRow(row, 84, 0)
	if !strings.Contains(rendered, "#") || !strings.Contains(rendered, "Goal") || !strings.Contains(rendered, "Contract") {
		t.Fatalf("expected fixed sprint columns\n%s", rendered)
	}
	for _, expected := range []string{"Criar app", "✓ AGREED", "✓ DONE", "✓ PASS", "✓ 100"} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("expected %q in fixed sprint row\n%s", expected, rendered)
		}
	}
	lines := strings.Split(rendered, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected header and one compact sprint row, got %d lines\n%s", len(lines), rendered)
	}
	for _, line := range lines {
		if len([]rune(line)) > 84 {
			t.Fatalf("expected row to stay within the requested width\n%s", rendered)
		}
	}
}

func TestRefreshDetectsHarnessArtifactChanges(t *testing.T) {
	root := t.TempDir()
	harnessDir := filepath.Join(root, ".harness")
	if err := os.MkdirAll(filepath.Join(harnessDir, "reports"), 0o755); err != nil {
		t.Fatal(err)
	}

	m := newModel(harnessDir, true, "dev")
	initial := m.signature
	reportPath := filepath.Join(harnessDir, "reports", "latest.json")
	report := `{
  "schema_version": "2",
  "sprint_number": 1,
  "total_score": 100,
  "verdict": "PASS",
  "dimensions": {},
  "duration_seconds": 1.1
}`
	if err := os.WriteFile(reportPath, []byte(report), 0o644); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(reportPath, future, future); err != nil {
		t.Fatal(err)
	}

	m.refresh()
	if m.signature == initial {
		t.Fatal("expected watch signature to change after report update")
	}
	if m.lastEvent != "qa report updated" {
		t.Fatalf("expected last event to be qa report updated, got %q", m.lastEvent)
	}
	if !strings.Contains(m.View(), "qa report updated") {
		t.Fatalf("expected view to include the latest watch event\n%s", m.View())
	}
}
