package harness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureAgentProtocolModeRefreshesStaleGeneratedProtocol(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".harness")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := `# Harness Agent Protocol

Contract automation skills are enabled.

- .harness/skills/contract-authoring/SKILL.md

Run harness sprint score before declaring the work complete.
`
	path := filepath.Join(root, "agent-protocol.md")
	if err := os.WriteFile(path, []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ensureAgentProtocolMode(root, true); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, expected := range []string{"harness.repair", "sprint repair", ".harness/repairs/latest.md", "sprint score` only after QA"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected refreshed protocol to contain %q\n%s", expected, text)
		}
	}
}
