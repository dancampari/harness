package harness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendStateEntryRoutesKindToSection(t *testing.T) {
	root := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(wd) }()
	if err := os.MkdirAll(".harness", 0o755); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		kind, message, wantSection string
	}{
		{"decision", "use modernc.org/sqlite", "## Decisions"},
		{"blocker", "ci runner pinned to 4 cores", "## Blockers"},
		{"todo", "wire HANDOFF.md into setup output", "## Todos"},
		{"deferred", "session replay for trend view", "## Deferred"},
		{"lesson", "do not mock the db in integration tests", "## Lessons"},
	}
	for _, tc := range cases {
		path, err := appendStateEntry(tc.kind, tc.message)
		if err != nil {
			t.Fatalf("append %s: %v", tc.kind, err)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		// Section heading should precede the message we just recorded.
		idxHeading := strings.Index(string(body), tc.wantSection)
		idxMessage := strings.Index(string(body), tc.message)
		if idxHeading < 0 || idxMessage < 0 || idxHeading > idxMessage {
			t.Fatalf("expected message %q to land after %q, got body:\n%s", tc.message, tc.wantSection, body)
		}
	}

	final, err := os.ReadFile(filepath.Join(siblingSpecsRoot(".harness"), "project", "STATE.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"use modernc.org/sqlite",
		"ci runner pinned to 4 cores",
		"wire HANDOFF.md into setup output",
		"session replay for trend view",
		"do not mock the db in integration tests",
	} {
		if !strings.Contains(string(final), want) {
			t.Fatalf("expected STATE.md to contain %q, got:\n%s", want, final)
		}
	}
}

func TestNormalizeStateKindRejectsUnknown(t *testing.T) {
	if got := normalizeStateKind("ridiculous"); got != "" {
		t.Fatalf("expected unknown kind to normalise to empty, got %q", got)
	}
}
