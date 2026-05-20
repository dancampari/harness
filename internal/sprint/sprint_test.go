package sprint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsolatedEvaluatorPathDropsUnsafeRelativeEntries(t *testing.T) {
	root := t.TempDir()
	nodeBin := filepath.Join(root, "node_modules", ".bin")
	if err := os.MkdirAll(nodeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	relativeBin := filepath.Join("node_modules", ".bin")
	input := strings.Join([]string{relativeBin, ".", filepath.Join(root, "custom-bin")}, string(os.PathListSeparator))

	got := isolatedEvaluatorPath(root, input)
	entries := filepath.SplitList(got)
	if len(entries) == 0 {
		t.Fatal("expected PATH entries")
	}
	if entries[0] != filepath.Clean(nodeBin) {
		t.Fatalf("expected node_modules/.bin to be prepended as an absolute path, got %q in %q", entries[0], got)
	}
	for _, entry := range entries {
		if entry == "." || entry == relativeBin || !filepath.IsAbs(entry) {
			t.Fatalf("expected only safe absolute PATH entries, got %q in %q", entry, got)
		}
	}
}

func TestIsolatedEvaluatorEnvDoesNotDisableExecErrDot(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PATH", strings.Join([]string{filepath.Join("node_modules", ".bin"), "."}, string(os.PathListSeparator)))

	env := isolatedEvaluatorEnv(root, false, false)
	for _, kv := range env {
		if strings.HasPrefix(kv, "GODEBUG=") && strings.Contains(kv, "execerrdot=0") {
			t.Fatalf("isolated env must not disable Go exec ErrDot protection: %q", kv)
		}
	}
}
