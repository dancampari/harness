package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestViewRendersPremiumOverviewSections(t *testing.T) {
	harnessDir := writeHarnessFixture(t)

	m := newModel(harnessDir, true, "0.4.7-test")
	m.width = 132
	m.height = 38
	view := m.View()

	for _, expected := range []string{
		"harness",
		"v0.4.7-test",
		"[1] Overview",
		"Current run",
		"Quality gate",
		"Pipeline",
		"Latest activity",
		"Exportar helpers formatados",
		"98/100",
		"correctness",
		"validation.passed",
	} {
		if !strings.Contains(view, expected) {
			t.Fatalf("expected view to contain %q\n%s", expected, view)
		}
	}
}

func TestViewWorksWithoutCurrentRun(t *testing.T) {
	root := t.TempDir()
	harnessDir := filepath.Join(root, ".harness")
	if err := os.MkdirAll(harnessDir, 0o755); err != nil {
		t.Fatal(err)
	}

	m := newModel(harnessDir, true, "dev")
	m.width = 110
	m.height = 30
	view := m.View()

	for _, expected := range []string{"Current run", "No active run found", "harness sprint new"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("expected empty overview to contain %q\n%s", expected, view)
		}
	}
}

func TestViewFitsTerminalSize(t *testing.T) {
	harnessDir := writeHarnessFixture(t)
	m := newModel(harnessDir, true, "dev")
	m.width = 80
	m.height = 12
	view := m.View()
	lines := strings.Split(view, "\n")
	if len(lines) != m.height {
		t.Fatalf("expected view to fit %d terminal rows, got %d\n%s", m.height, len(lines), view)
	}
	for _, line := range lines {
		if lipgloss.Width(line) < m.width {
			t.Fatalf("expected line to clear terminal width %d, got %d\n%s", m.width, lipgloss.Width(line), view)
		}
	}
}

func TestOverviewDoesNotRenderTruncationEllipsis(t *testing.T) {
	harnessDir := writeHarnessFixture(t)
	appendRun(t, harnessDir, "2026-05-20_22-42-00_sprint-005",
		"Uma meta deliberadamente longa para confirmar que o terminal recorta sem reticencias laterais",
		"pass", 94)

	m := newModel(harnessDir, true, "dev")
	m.width = 92
	m.height = 26
	view := m.View()
	if strings.Contains(view, "...") {
		t.Fatalf("dashboard should clip cleanly without lateral ellipsis\n%s", view)
	}
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(line) > m.width {
			t.Fatalf("line exceeds terminal width %d: %d\n%s", m.width, lipgloss.Width(line), line)
		}
	}
}

func TestRunsViewHandlesLongGoals(t *testing.T) {
	harnessDir := writeHarnessFixture(t)
	appendRun(t, harnessDir, "2026-05-20_22-42-00_sprint-005",
		"Uma meta muito longa que precisa ser truncada sem quebrar a tabela do terminal e sem empurrar colunas",
		"fail", 63)

	m := newModel(harnessDir, true, "dev")
	m.width = 94
	m.height = 24
	m.activeView = viewRuns
	view := m.View()

	for _, expected := range []string{"Runs history", "status", "score", "Uma meta muito"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("expected runs view to contain %q\n%s", expected, view)
		}
	}
	for _, line := range strings.Split(view, "\n") {
		if lipgloss.Width(line) > m.width {
			t.Fatalf("line exceeds terminal width %d: %d\n%s", m.width, lipgloss.Width(line), line)
		}
	}
}

func TestLegacyDraftContractMakesExistingQAStale(t *testing.T) {
	root := t.TempDir()
	harnessDir := filepath.Join(root, ".harness")
	for _, dir := range []string{"contracts", "reports", "evaluations"} {
		if err := os.MkdirAll(filepath.Join(harnessDir, dir), 0o755); err != nil {
			t.Fatal(err)
		}
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
	report := fmt.Sprintf(`{
  "schema_version": "2",
  "timestamp": %q,
  "sprint_number": 1,
  "total_score": 100,
  "verdict": "PASS",
  "dimensions": {},
  "duration_seconds": 1.1
}`, time.Now().UTC().Format(time.RFC3339Nano))
	if err := os.WriteFile(filepath.Join(harnessDir, "reports", "sprint-001.json"), []byte(report), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(harnessDir, "reports", "latest.json"), []byte(report), 0o644); err != nil {
		t.Fatal(err)
	}

	m := newModel(harnessDir, true, "dev")
	m.width = 120
	m.height = 32
	view := m.View()
	for _, expected := range []string{"BLOCKED", "stale", "Quality gate"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("expected stale QA to render %q\n%s", expected, view)
		}
	}
}

func TestHarnessCommandShortcuts(t *testing.T) {
	cases := map[string][]string{
		"qa":             {"sprint", "qa"},
		"accept":         {"sprint", "qa", "--accept-screenshots"},
		"repair":         {"sprint", "repair"},
		"score":          {"sprint", "score"},
		"status":         {"sprint", "status"},
		"propose":        {"contract", "propose"},
		"doctor --fix":   {"doctor", "--fix"},
		"approve tester": {"contract", "approve", "--role", "tester"},
		"new demo goal":  {"sprint", "new", "demo goal"},
	}
	for input, expected := range cases {
		got, err := harnessCommandArgs(input)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", input, err)
		}
		if strings.Join(got, "\x00") != strings.Join(expected, "\x00") {
			t.Fatalf("%s: expected %#v, got %#v", input, expected, got)
		}
	}
}

func TestFooterDoesNotAdvertiseMissingActions(t *testing.T) {
	for _, tc := range []struct {
		view      viewID
		forbidden []string
		required  []string
	}{
		{viewRuns, []string{"search"}, []string{"details", "select"}},
		{viewLogs, []string{"search"}, []string{"pause/resume", "run command", "scroll"}},
		{viewSkills, []string{"toggle"}, []string{"details", "scroll"}},
		{viewDoctor, []string{"verbose"}, []string{"doctor --fix"}},
	} {
		keys := footerKeys(tc.view)
		var labels []string
		for _, key := range keys {
			labels = append(labels, key[0], key[1])
		}
		joined := strings.Join(labels, " ")
		for _, forbidden := range tc.forbidden {
			if strings.Contains(joined, forbidden) {
				t.Fatalf("view %v advertises unsupported action %q in %q", tc.view, forbidden, joined)
			}
		}
		for _, required := range tc.required {
			if !strings.Contains(joined, required) {
				t.Fatalf("view %v should advertise %q in %q", tc.view, required, joined)
			}
		}
	}
}

func TestLogsPauseAndDoctorFixShortcutsAreFunctional(t *testing.T) {
	harnessDir := writeHarnessFixture(t)
	m := newModel(harnessDir, true, "dev")
	m.width = 110
	m.height = 26
	m.activeView = viewLogs

	if !strings.Contains(stripANSI(m.View()), "stream live") {
		t.Fatalf("expected logs to start live\n%s", m.View())
	}
	updated, cmd := m.updateKey(tea.KeyMsg{Type: tea.KeySpace})
	if cmd != nil {
		t.Fatalf("space in logs should only toggle state")
	}
	m = updated.(*model)
	if m.logsFollow {
		t.Fatalf("expected logs follow to be paused")
	}
	if !strings.Contains(stripANSI(m.View()), "stream paused") {
		t.Fatalf("expected logs view to show paused stream\n%s", m.View())
	}

	m.activeView = viewDoctor
	updated, cmd = m.updateKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	if cmd == nil {
		t.Fatalf("doctor fix shortcut should return a command")
	}
	m = updated.(*model)
	if !m.commandBusy || m.commandRun != "doctor --fix" {
		t.Fatalf("expected doctor --fix command to be queued, busy=%v run=%q", m.commandBusy, m.commandRun)
	}
}

func TestOverviewPrefersActiveRunOverStaleCurrentRunFile(t *testing.T) {
	harnessDir := writeHarnessFixture(t)
	appendRun(t, harnessDir, "2026-05-20_22-42-00_sprint-005",
		"Agent-First Menu Design Workspace", "running", 0)

	data := loadDashboardData(harnessDir)
	if data.Current.Number != 5 {
		t.Fatalf("expected active sprint 005 to be current, got number=%d run=%q status=%q",
			data.Current.Number, data.Current.RunID, data.Current.Status)
	}
	if normalizeStatus(data.Current.Status) != "running" {
		t.Fatalf("expected running current run, got %+v", data.Current)
	}

	m := newModel(harnessDir, true, "dev")
	m.width = 132
	m.height = 34
	view := stripANSI(m.View())
	if !strings.Contains(view, "Sprint 005") || !strings.Contains(view, "Agent-First Menu Design Workspace") {
		t.Fatalf("overview should render active sprint 005\n%s", view)
	}
}

func writeHarnessFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	harnessDir := filepath.Join(root, ".harness")
	for _, dir := range []string{"runs", "reports", "skills"} {
		if err := os.MkdirAll(filepath.Join(harnessDir, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(harnessDir, "setup.json"), []byte(`{
  "project": "harness-demo",
  "stack": "typescript",
  "coding_cli": "codex",
  "planning_mode": "spec-driven"
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(harnessDir, "current-run.json"), []byte(`{
  "runId": "2026-05-20_22-38-24_sprint-004",
  "feature": "Exportar helpers formatados de invoice",
  "agent": "codex",
  "status": "pass",
  "score": 98,
  "startedAt": "2026-05-20T22:38:24Z",
  "finishedAt": null,
  "runtime": "2.7s",
  "updatedAt": "2026-05-20T22:41:08Z",
  "branch": "main",
  "reportPath": ".harness/reports/latest.md",
  "validations": {
    "contract": "agreed",
    "build": "done",
    "qa": "pass",
    "report": "done",
    "accept": "done"
  },
  "quality": [
    {"dimension": "correctness", "score": 100, "threshold": 80, "status": "pass", "findings": 0, "sensors": "eslint,vitest"},
    {"dimension": "coverage", "score": 91, "threshold": 70, "status": "pass", "findings": 0, "sensors": "vitest-coverage"},
    {"dimension": "security", "score": 100, "threshold": 85, "status": "pass", "findings": 0, "sensors": "npm-audit"}
  ],
  "findings": 0
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	appendRun(t, harnessDir, "2026-05-20_22-38-24_sprint-004", "Exportar helpers formatados de invoice", "pass", 98)
	if err := os.WriteFile(filepath.Join(harnessDir, "reports", "latest.md"), []byte(`# Verdict: PASS

## Scores
| Dimension | Score |
|---|---:|
| correctness | 100 |
`), 0o644); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(harnessDir, "runs", "2026-05-20_22-38-24_sprint-004")
	if err := os.WriteFile(filepath.Join(runDir, "events.jsonl"), []byte(`{"timestamp":"2026-05-20T22:41:08Z","type":"qa.report.updated","message":"Score: 98/100","agent":"codex","metadata":{}}
{"timestamp":"2026-05-20T22:41:07Z","type":"validation.passed","message":"vitest","agent":"codex","metadata":{}}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	return harnessDir
}

func appendRun(t *testing.T, harnessDir, id, feature, status string, score int) {
	t.Helper()
	runDir := filepath.Join(harnessDir, "runs", id)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	run := fmt.Sprintf(`{
  "runId": %q,
  "feature": %q,
  "agent": "codex",
  "status": %q,
  "score": %d,
  "startedAt": "2026-05-20T22:38:24Z",
  "runtime": "2.7s",
  "updatedAt": "2026-05-20T22:41:08Z",
  "branch": "main",
  "reportPath": ".harness/reports/latest.md",
  "validations": {"contract":"agreed","build":"done","qa":%q,"report":"done","accept":"done"},
  "quality": [{"dimension":"correctness","score":%d,"threshold":80,"status":%q,"findings":0}],
  "findings": 0
}`, id, feature, status, score, status, score, status)
	if err := os.WriteFile(filepath.Join(runDir, "run.json"), []byte(run), 0o644); err != nil {
		t.Fatal(err)
	}
}
