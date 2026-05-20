package adapters

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNodeToolCommandUsesAbsoluteProjectBin(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "node_modules", ".bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tool := filepath.Join(binDir, "vitest.cmd")
	if err := os.WriteFile(tool, []byte("@echo off\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := nodeToolCommand(context.Background(), root, "vitest", "run")
	if cmd.Path != filepath.Clean(tool) {
		t.Fatalf("expected absolute project bin %q, got %q", filepath.Clean(tool), cmd.Path)
	}
	if !filepath.IsAbs(cmd.Path) {
		t.Fatalf("expected absolute command path, got %q", cmd.Path)
	}
}
