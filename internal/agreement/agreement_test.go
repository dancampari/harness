package agreement

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAgreementRequiresSameHashFromAllRoles(t *testing.T) {
	root := t.TempDir()
	writeContract(t, root, "first")

	m := NewManager(root)
	st, err := m.Propose(1)
	if err != nil {
		t.Fatal(err)
	}
	if st.State != "proposed" {
		t.Fatalf("expected proposed, got %+v", st)
	}
	st, err = m.Approve(1, "planner")
	if err != nil {
		t.Fatal(err)
	}
	if st.State != "proposed" || strings.Join(st.MissingRoles, ",") != "tester" {
		t.Fatalf("expected tester still missing, got %+v", st)
	}
	st, err = m.Approve(1, "tester")
	if err != nil {
		t.Fatal(err)
	}
	if st.State != "agreed" {
		t.Fatalf("expected agreed, got %+v", st)
	}
	if st.AgreedAt.IsZero() {
		t.Fatal("expected agreed status to include agreement timestamp")
	}
	if st.ReportIsCurrent(st.AgreedAt.Add(-time.Second)) {
		t.Fatal("expected report generated before agreement to be stale")
	}
	if !st.ReportIsCurrent(st.AgreedAt.Add(time.Second)) {
		t.Fatal("expected report generated after agreement to be current")
	}
	if err := m.EnsureAgreed(1); err != nil {
		t.Fatal(err)
	}
}

func TestContractChangeInvalidatesAgreement(t *testing.T) {
	root := t.TempDir()
	writeContract(t, root, "first")

	m := NewManager(root)
	if _, err := m.Propose(1); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Approve(1, "planner"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Approve(1, "tester"); err != nil {
		t.Fatal(err)
	}

	writeContract(t, root, "changed")
	st, err := m.Status(1)
	if err != nil {
		t.Fatal(err)
	}
	if st.State != "changed" {
		t.Fatalf("expected changed, got %+v", st)
	}
	if err := m.EnsureAgreed(1); err == nil {
		t.Fatal("expected changed contract to block agreement")
	}
}

func TestDesignChangeInvalidatesAgreement(t *testing.T) {
	root := t.TempDir()
	writeContract(t, root, "design coverage")
	writeArtifact(t, root, "design", "first design")

	m := NewManager(root)
	if _, err := m.Propose(1); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Approve(1, "planner"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Approve(1, "tester"); err != nil {
		t.Fatal(err)
	}

	st, err := m.Status(1)
	if err != nil {
		t.Fatal(err)
	}
	if st.State != "agreed" {
		t.Fatalf("expected agreed before design change, got %+v", st)
	}
	if !contains(st.Hashed, "contract") || !contains(st.Hashed, "design") {
		t.Fatalf("expected Hashed to include contract + design, got %v", st.Hashed)
	}

	writeArtifact(t, root, "design", "changed design")
	st, err = m.Status(1)
	if err != nil {
		t.Fatal(err)
	}
	if st.State != "changed" {
		t.Fatalf("expected state changed after design edit, got %+v", st)
	}
}

func TestTasksChangeInvalidatesAgreement(t *testing.T) {
	root := t.TempDir()
	writeContract(t, root, "tasks coverage")
	writeArtifact(t, root, "tasks", "first tasks plan")

	m := NewManager(root)
	if _, err := m.Propose(1); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Approve(1, "planner"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Approve(1, "tester"); err != nil {
		t.Fatal(err)
	}

	st, err := m.Status(1)
	if err != nil {
		t.Fatal(err)
	}
	if st.State != "agreed" {
		t.Fatalf("expected agreed before tasks change, got %+v", st)
	}
	if !contains(st.Hashed, "tasks") {
		t.Fatalf("expected Hashed to include tasks, got %v", st.Hashed)
	}

	writeArtifact(t, root, "tasks", "changed tasks plan")
	st, err = m.Status(1)
	if err != nil {
		t.Fatal(err)
	}
	if st.State != "changed" {
		t.Fatalf("expected state changed after tasks edit, got %+v", st)
	}
}

func TestContractOnlySprintHashHasNoDesignTasks(t *testing.T) {
	root := t.TempDir()
	writeContract(t, root, "no extras")

	st, err := NewManager(root).Status(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Hashed) != 1 || st.Hashed[0] != "contract" {
		t.Fatalf("expected Hashed = [contract] for contract-only sprint, got %v", st.Hashed)
	}
}

func writeArtifact(t *testing.T, root, kind, body string) {
	t.Helper()
	path := filepath.Join(root, kind, "sprint-001.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRejectBlocksContract(t *testing.T) {
	root := t.TempDir()
	writeContract(t, root, "first")

	m := NewManager(root)
	if _, err := m.Propose(1); err != nil {
		t.Fatal(err)
	}
	st, err := m.Reject(1, "tester", "criteria too weak")
	if err != nil {
		t.Fatal(err)
	}
	if st.State != "rejected" {
		t.Fatalf("expected rejected, got %+v", st)
	}
	if _, err := m.Approve(1, "tester"); err == nil {
		t.Fatal("expected approval after rejection to fail")
	}
}

func TestProposeRejectsSpecDrivenContractMissingRequirements(t *testing.T) {
	root := t.TempDir()
	writeContract(t, root, "weak spec-driven")
	writeSetup(t, root, `{"planning_mode":"spec-driven"}`)

	m := NewManager(root)
	_, err := m.Propose(1)
	if err == nil {
		t.Fatal("expected Propose to reject contract that violates spec-driven policy")
	}
	if !strings.Contains(err.Error(), "spec-driven policy") {
		t.Fatalf("expected spec-driven policy error, got %v", err)
	}
}

func TestProposeAcceptsCompliantSpecDrivenContract(t *testing.T) {
	root := t.TempDir()
	contractPath := filepath.Join(root, "contracts", "sprint-001.md")
	if err := os.MkdirAll(filepath.Dir(contractPath), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "# Sprint 001 - compliant spec-driven\n\n" +
		"## Goal\nShip a compliant spec-driven sprint.\n\n" +
		"## Requirements\n- REQ-001: Ship the feature.\n\n" +
		"## Deliverables\n- `index.js` (REQ-001)\n\n" +
		"## Acceptance Criteria\n" +
		"| # | REQ     | Criterion                                                              | Evidence    | Threshold |\n" +
		"|---|---------|------------------------------------------------------------------------|-------------|-----------|\n" +
		"| 1 | REQ-001 | WHEN the agent runs build THEN the system SHALL emit index.js          | tests:works | 8/10      |\n\n" +
		"## Edge Cases\n- empty input\n\n" +
		"## Out of Scope\n- documentation site\n"
	if err := os.WriteFile(contractPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	writeSetup(t, root, `{"planning_mode":"spec-driven"}`)

	st, err := NewManager(root).Propose(1)
	if err != nil {
		t.Fatalf("expected compliant contract to be proposed, got %v", err)
	}
	if st.State != "proposed" {
		t.Fatalf("expected proposed state, got %+v", st)
	}
}

func writeSetup(t *testing.T, root, payload string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "setup.json"), []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeContract(t *testing.T, root, goal string) {
	t.Helper()
	path := filepath.Join(root, "contracts", "sprint-001.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := `# Sprint 001 - ` + goal + `

## Goal
` + goal + `

## Deliverables
- ` + "`index.js`" + `

## Acceptance Criteria
| # | Criterion | Threshold |
|---|-----------|-----------|
| 1 | Works | 8/10 |

## Constraints
- max_function_complexity: 10
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
