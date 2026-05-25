package adapters

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestClassifyChangesSeparatesImplAndTest(t *testing.T) {
	files := []string{
		"src/auth/user.ts",
		"src/auth/user.test.ts",
		"src/billing/invoice.ts",
		"README.md",
		"go.sum",
	}
	impl, test := classifyChanges(files)
	if len(impl) != 2 || impl[0] != "src/auth/user.ts" || impl[1] != "src/billing/invoice.ts" {
		t.Fatalf("unexpected impl set: %v", impl)
	}
	if len(test) != 1 || test[0] != "src/auth/user.test.ts" {
		t.Fatalf("unexpected test set: %v", test)
	}
}

func TestClassifyChangesHonorsTestPathSegments(t *testing.T) {
	files := []string{
		"internal/feature/service.go",
		"internal/feature/service_test.go",
		"tests/integration/checkout.spec.ts",
		"src/api/handler.py",
		"src/api/test_handler.py",
	}
	impl, test := classifyChanges(files)
	if len(impl) != 2 {
		t.Fatalf("expected 2 impl files, got %v", impl)
	}
	if len(test) != 3 {
		t.Fatalf("expected 3 test files, got %v", test)
	}
}

func TestShortFilesRefCapsAtThree(t *testing.T) {
	got := shortFilesRef([]string{"a.go", "b.go", "c.go", "d.go", "e.go"})
	want := "a.go, b.go, c.go (+ more)"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestTDDViolationHardFailsImplementationCommitWithoutTests(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "service.go"), []byte("package src\n\nfunc Service() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitForTDDTest(t, root, "init")
	runGitForTDDTest(t, root, "config", "user.email", "harness-test@example.local")
	runGitForTDDTest(t, root, "config", "user.name", "Harness Test")
	runGitForTDDTest(t, root, "add", ".")
	runGitForTDDTest(t, root, "commit", "-m", "implementation without test")

	res := TDDViolation{}.Run(context.Background(), root)
	if len(res.Findings) != 1 {
		t.Fatalf("expected tdd-violation finding, got %#v", res.Findings)
	}
	if res.Findings[0].Rule != "tdd-violation" {
		t.Fatalf("unexpected rule: %s", res.Findings[0].Rule)
	}
	if res.RawScore != 0 {
		t.Fatalf("expected hard-fail RawScore 0, got %d", res.RawScore)
	}
}

func runGitForTDDTest(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}
