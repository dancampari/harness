package adapters

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"

	"github.com/dancampari/harness/internal/planner"
)

func TestFilesOutsideAllowedReportsOffScopeChanges(t *testing.T) {
	changed := []string{
		"src/auth/user.ts",
		"src/auth/user.test.ts",
		"src/billing/invoice.ts",
		".harness/setup.json",
		".specs/features/sprint-001/spec.md",
	}
	allowed := []string{"src/auth/user.ts", "src/auth/user.test.ts"}

	got := filesOutsideAllowed(changed, allowed)
	want := []string{"src/billing/invoice.ts"}
	sort.Strings(got)
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("expected %v, got %v", want, got)
	}
}

func TestFilesOutsideAllowedHonoursTrailingSlashPrefix(t *testing.T) {
	changed := []string{"src/auth/user.ts", "src/auth/session.ts", "src/billing/x.ts"}
	allowed := []string{"src/auth/"}

	got := filesOutsideAllowed(changed, allowed)
	if len(got) != 1 || got[0] != "src/billing/x.ts" {
		t.Fatalf("expected only src/billing/x.ts off-scope, got %v", got)
	}
}

func TestUnionWhereDeduplicates(t *testing.T) {
	plan := &planner.TaskPlan{Tasks: []planner.Task{
		{Number: 1, Where: []string{"a.ts", "b.ts"}},
		{Number: 2, Where: []string{"b.ts", "c.ts"}},
	}}
	got := unionWhere(plan)
	want := []string{"a.ts", "b.ts", "c.ts"}
	if len(got) != len(want) {
		t.Fatalf("expected %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("at %d: expected %s, got %s", i, want[i], got[i])
		}
	}
}

func TestFindLatestTasksPathPrefersSpecsLayout(t *testing.T) {
	root := t.TempDir()
	specs1 := filepath.Join(root, ".specs", "features", "sprint-001")
	specs2 := filepath.Join(root, ".specs", "features", "sprint-002")
	legacy := filepath.Join(root, ".harness", "tasks")
	for _, d := range []string{specs1, specs2, legacy} {
		if err := mkdirAllForTest(d); err != nil {
			t.Fatal(err)
		}
	}
	for path, body := range map[string]string{
		filepath.Join(specs1, "tasks.md"):      "# 1",
		filepath.Join(specs2, "tasks.md"):      "# 2",
		filepath.Join(legacy, "sprint-001.md"): "# legacy",
		filepath.Join(legacy, "sprint-005.md"): "# legacy 5",
	} {
		if err := writeFileForTest(path, body); err != nil {
			t.Fatal(err)
		}
	}
	got := findLatestTasksPath(root)
	if filepath.Base(filepath.Dir(got)) != "sprint-002" {
		t.Fatalf("expected .specs/features/sprint-002/tasks.md, got %s", got)
	}
}

func TestScopeCreepHardFailsOffScopeChanges(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	root := t.TempDir()
	writeFileForTest(filepath.Join(root, ".specs", "features", "sprint-001", "tasks.md"), `# Tasks

## Task 001 - Auth
- Where: src/auth/user.ts
- Tests: src/auth/user.test.ts
`)
	writeFileForTest(filepath.Join(root, "src", "auth", "user.ts"), "export const user = 1\n")
	writeFileForTest(filepath.Join(root, "src", "billing", "invoice.ts"), "export const invoice = 1\n")
	runGitForTest(t, root, "init")
	runGitForTest(t, root, "config", "user.email", "harness-test@example.local")
	runGitForTest(t, root, "config", "user.name", "Harness Test")
	runGitForTest(t, root, "add", ".")
	runGitForTest(t, root, "commit", "-m", "baseline")

	writeFileForTest(filepath.Join(root, "src", "billing", "invoice.ts"), "export const invoice = 2\n")

	res := ScopeCreep{}.Run(context.Background(), root)
	if len(res.Findings) != 1 {
		t.Fatalf("expected scope-creep finding, got %#v", res.Findings)
	}
	if res.Findings[0].Rule != "scope-creep" {
		t.Fatalf("unexpected rule: %s", res.Findings[0].Rule)
	}
	if res.RawScore != 0 {
		t.Fatalf("expected hard-fail RawScore 0, got %d", res.RawScore)
	}
}

func runGitForTest(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func mkdirAllForTest(dir string) error { return os.MkdirAll(dir, 0o755) }
func writeFileForTest(path, body string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(body), 0o644)
}
