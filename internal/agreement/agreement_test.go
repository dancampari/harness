package agreement

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
