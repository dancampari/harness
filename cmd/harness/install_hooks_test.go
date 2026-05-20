package harness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallHooksSpecDrivenWritesProviderReferences(t *testing.T) {
	root := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	}()
	if err := os.MkdirAll(".harness", 0o755); err != nil {
		t.Fatal(err)
	}

	if err := runInstallHooks(installHookOptions{
		CLI:        "all",
		Planning:   PlanningSpecDriven,
		InstallGit: false,
	}); err != nil {
		t.Fatal(err)
	}

	expectFileContains(t, "AGENTS.md", "harness_spec_planner", "harness_task_worker")
	expectFileContains(t, "CLAUDE.md", "harness-spec-planner", "harness-task-worker")
	expectFileContains(t, filepath.Join(".cursor", "rules", "harness.mdc"), "Spec-driven automation")
	expectFileContains(t, filepath.Join(".codex", "agents", "harness-spec-planner.toml"), "harness_spec_planner", "Specify")
	expectFileContains(t, filepath.Join(".claude", "agents", "harness-task-worker.md"), "harness-task-worker", "AGREED")
}

func TestInstallGitHookSkipsReposWithoutHarnessConfig(t *testing.T) {
	root := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	}()
	if err := os.MkdirAll(".git", 0o755); err != nil {
		t.Fatal(err)
	}

	if err := installGitHook(); err != nil {
		t.Fatal(err)
	}

	expectFileContains(t,
		filepath.Join(".git", "hooks", "pre-push"),
		`if [ ! -f ".harness/config.yaml" ]; then`,
		"sprint qa --format=tty || true",
	)
}

func expectFileContains(t *testing.T, path string, needles ...string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, needle := range needles {
		if !strings.Contains(text, needle) {
			t.Fatalf("expected %s to contain %q\n%s", path, needle, text)
		}
	}
}
