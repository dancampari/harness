package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHashIsDeterministicForUnchangedTree(t *testing.T) {
	root := t.TempDir()
	mkfile(t, filepath.Join(root, "src", "a.ts"), "export const a = 1;\n")
	mkfile(t, filepath.Join(root, "src", "b.ts"), "export const b = 2;\n")
	mkfile(t, filepath.Join(root, "package.json"), `{"name":"demo"}`)

	first, err := Hash(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Hash(root)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("expected stable hash, got %s vs %s", first, second)
	}
	if len(first) != 64 {
		t.Fatalf("expected 64-char hex, got %d chars", len(first))
	}
}

func TestHashChangesWhenFileChanges(t *testing.T) {
	root := t.TempDir()
	mkfile(t, filepath.Join(root, "src", "a.ts"), "export const a = 1;\n")

	before, err := Hash(root)
	if err != nil {
		t.Fatal(err)
	}
	mkfile(t, filepath.Join(root, "src", "a.ts"), "export const a = 2;\n")
	after, err := Hash(root)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatalf("expected hash to change after edit, both %s", before)
	}
}

func TestHashIgnoresSkippedDirectories(t *testing.T) {
	root := t.TempDir()
	mkfile(t, filepath.Join(root, "src", "a.ts"), "export const a = 1;\n")

	baseline, err := Hash(root)
	if err != nil {
		t.Fatal(err)
	}

	// Anything inside skipped directories should not move the hash.
	for _, dir := range []string{".harness", ".git", "node_modules", "dist", "coverage"} {
		mkfile(t, filepath.Join(root, dir, "garbage"), "noise\n")
	}
	updated, err := Hash(root)
	if err != nil {
		t.Fatal(err)
	}
	if baseline != updated {
		t.Fatalf("expected skipped directories to be ignored, got %s vs %s", baseline, updated)
	}
}

func mkfile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
