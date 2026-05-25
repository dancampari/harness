package harness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteHandoffAppendsAcrossInvocations(t *testing.T) {
	root := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(wd) }()

	if _, err := writeHandoff("first stop"); err != nil {
		t.Fatal(err)
	}
	if _, err := writeHandoff("second stop"); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(siblingSpecsRoot(".harness"), "project", "HANDOFF.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, "# Session Handoff") {
		t.Fatalf("expected canonical header, got:\n%s", text)
	}
	if !strings.Contains(text, "first stop") || !strings.Contains(text, "second stop") {
		t.Fatalf("expected both notes preserved, got:\n%s", text)
	}
	if strings.Count(text, "## ") < 2 {
		t.Fatalf("expected two timestamped entries, got:\n%s", text)
	}
}
