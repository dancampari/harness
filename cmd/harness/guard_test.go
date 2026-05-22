package harness

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dancampari/harness/internal/agreement"
	"github.com/dancampari/harness/internal/events"
)

func TestGuardBlocksProductPatchBeforeAgreement(t *testing.T) {
	root := t.TempDir()
	writeGuardContract(t, root)

	input := strings.NewReader(`{
  "hook_event_name": "PreToolUse",
  "cwd": ` + quoteJSON(root) + `,
  "tool_name": "apply_patch",
  "tool_input": {
    "cmd": "*** Begin Patch\n*** Update File: src/App.tsx\n@@\n-old\n+new\n*** End Patch\n"
  }
}`)
	var output bytes.Buffer
	if err := runGuardPreTool(input, &output); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if !strings.Contains(text, `"permissionDecision":"deny"`) ||
		!strings.Contains(text, "contract is DRAFT") {
		t.Fatalf("expected guard denial, got %s", text)
	}
}

func TestGuardAllowsContractPatchBeforeAgreement(t *testing.T) {
	root := t.TempDir()
	writeGuardContract(t, root)

	input := strings.NewReader(`{
  "cwd": ` + quoteJSON(root) + `,
  "tool_name": "apply_patch",
  "tool_input": {
    "cmd": "*** Begin Patch\n*** Update File: .harness/contracts/sprint-001.md\n@@\n-old\n+new\n*** End Patch\n"
  }
}`)
	var output bytes.Buffer
	if err := runGuardPreTool(input, &output); err != nil {
		t.Fatal(err)
	}
	if output.Len() != 0 {
		t.Fatalf("expected contract edit to be allowed, got %s", output.String())
	}
}

func TestGuardAllowsProductPatchAfterAgreement(t *testing.T) {
	root := t.TempDir()
	writeGuardContract(t, root)
	mgr := agreement.NewManager(filepath.Join(root, ".harness"))
	if _, err := mgr.Propose(1); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Approve(1, "planner"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Approve(1, "tester"); err != nil {
		t.Fatal(err)
	}

	input := strings.NewReader(`{
  "cwd": ` + quoteJSON(root) + `,
  "tool_name": "apply_patch",
  "tool_input": {
    "cmd": "*** Begin Patch\n*** Update File: src/App.tsx\n@@\n-old\n+new\n*** End Patch\n"
  }
}`)
	var output bytes.Buffer
	if err := runGuardPreTool(input, &output); err != nil {
		t.Fatal(err)
	}
	if output.Len() != 0 {
		t.Fatalf("expected product edit to be allowed after agreement, got %s", output.String())
	}
}

func TestGuardPreToolRecordsBlockedEdit(t *testing.T) {
	root := t.TempDir()
	writeGuardContract(t, root)

	input := strings.NewReader(`{
  "cwd": ` + quoteJSON(root) + `,
  "tool_name": "Write",
  "tool_input": {"file_path": "src/App.tsx"}
}`)
	var output bytes.Buffer
	if err := runGuardPreTool(input, &output); err != nil {
		t.Fatal(err)
	}
	recent := events.Recent(filepath.Join(root, ".harness"), 10)
	if len(recent) == 0 {
		t.Fatal("expected the guard to record an activity event")
	}
	if recent[0].Type != "agent.edit.blocked" {
		t.Fatalf("expected agent.edit.blocked event, got %q", recent[0].Type)
	}
	if recent[0].Phase != events.PhaseContract {
		t.Fatalf("expected blocked edit recorded under the contract phase, got %q", recent[0].Phase)
	}
}

func TestGuardPreToolRecordsBuildEditAfterAgreement(t *testing.T) {
	root := t.TempDir()
	writeGuardContract(t, root)
	mgr := agreement.NewManager(filepath.Join(root, ".harness"))
	if _, err := mgr.Propose(1); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Approve(1, "planner"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Approve(1, "tester"); err != nil {
		t.Fatal(err)
	}

	input := strings.NewReader(`{
  "cwd": ` + quoteJSON(root) + `,
  "tool_name": "Write",
  "tool_input": {"file_path": "src/App.tsx"}
}`)
	var output bytes.Buffer
	if err := runGuardPreTool(input, &output); err != nil {
		t.Fatal(err)
	}
	recent := events.Recent(filepath.Join(root, ".harness"), 10)
	if len(recent) == 0 || recent[0].Type != "agent.edit" {
		t.Fatalf("expected agent.edit event after agreement, got %+v", recent)
	}
	if recent[0].Phase != events.PhaseBuild {
		t.Fatalf("expected edit recorded under the build phase, got %q", recent[0].Phase)
	}
	if recent[0].Message != "src/App.tsx" {
		t.Fatalf("expected the edited path in the event, got %q", recent[0].Message)
	}
}

func TestGuardPostToolRecordsBashCommand(t *testing.T) {
	root := t.TempDir()
	writeGuardContract(t, root)

	input := strings.NewReader(`{
  "cwd": ` + quoteJSON(root) + `,
  "tool_name": "Bash",
  "tool_input": {"command": "npm test"}
}`)
	if err := runGuardPostTool(input); err != nil {
		t.Fatal(err)
	}
	recent := events.Recent(filepath.Join(root, ".harness"), 10)
	if len(recent) == 0 || recent[0].Type != "agent.bash" {
		t.Fatalf("expected agent.bash event, got %+v", recent)
	}
	if recent[0].Message != "npm test" {
		t.Fatalf("expected the command text in the event, got %q", recent[0].Message)
	}
}

func TestGuardPreToolWarnsOnUndecodablePayload(t *testing.T) {
	root := t.TempDir()
	writeGuardContract(t, root)
	chdir(t, root)

	var output bytes.Buffer
	// Malformed JSON: the guard must fail open (no decision written) but
	// must not fail silent.
	if err := runGuardPreTool(strings.NewReader("{ this is not json"), &output); err != nil {
		t.Fatal(err)
	}
	if output.Len() != 0 {
		t.Fatalf("guard must not block on a bad payload, got %s", output.String())
	}
	recent := events.Recent(filepath.Join(root, ".harness"), 10)
	if len(recent) == 0 || recent[0].Type != "guard.warn" {
		t.Fatalf("expected a guard.warn event for an undecodable payload, got %+v", recent)
	}
}

func TestGuardResolvesHarnessFromProcessCwdWhenHookCwdIsUnusable(t *testing.T) {
	root := t.TempDir()
	writeGuardContract(t, root)
	chdir(t, root)

	// hook cwd is a path Go cannot resolve to this project; the guard
	// must fall back to the process working directory rather than going
	// silently inert.
	input := strings.NewReader(`{
  "cwd": "/nonexistent/unix/style/path",
  "tool_name": "Write",
  "tool_input": {"file_path": "src/App.tsx"}
}`)
	var output bytes.Buffer
	if err := runGuardPreTool(input, &output); err != nil {
		t.Fatal(err)
	}
	recent := events.Recent(filepath.Join(root, ".harness"), 10)
	if len(recent) == 0 {
		t.Fatal("expected the guard to still record activity via the process cwd fallback")
	}
	if recent[0].Type != "agent.edit.blocked" {
		t.Fatalf("expected agent.edit.blocked recorded via cwd fallback, got %q", recent[0].Type)
	}
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(original) })
}

func writeGuardContract(t *testing.T, root string) {
	t.Helper()
	path := filepath.Join(root, ".harness", "contracts", "sprint-001.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := `# Sprint 001 - guard

## Goal
guard

## Deliverables
- ` + "`src/App.tsx`" + `

## Acceptance Criteria
| # | Criterion | Threshold |
|---|-----------|-----------|
| 1 | Works | 8/10 |
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func quoteJSON(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}
