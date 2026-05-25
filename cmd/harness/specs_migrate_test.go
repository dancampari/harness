package harness

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateLegacyArtifactsToSpecs(t *testing.T) {
	root := t.TempDir()
	harnessDir := filepath.Join(root, ".harness")

	files := map[string]string{
		filepath.Join(harnessDir, "spec.md"):                           "# product spec body\n",
		filepath.Join(harnessDir, "progress.md"):                       "# state body\n",
		filepath.Join(harnessDir, "context", "STACK.md"):               "node,\n",
		filepath.Join(harnessDir, "context", "ARCHITECTURE.md"):        "layers,\n",
		filepath.Join(harnessDir, "contracts", "sprint-001.md"):        "# contract body\n",
		filepath.Join(harnessDir, "design", "sprint-001.md"):           "# design body\n",
		filepath.Join(harnessDir, "tasks", "sprint-001.md"):            "# tasks body\n",
		filepath.Join(harnessDir, "contracts", "sprint-001.lock.json"): `{"sprint_number":1}`,
	}
	for path, body := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := migrateLegacyArtifactsToSpecs(harnessDir); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	specsRoot := filepath.Join(root, ".specs")
	expectations := map[string]string{
		filepath.Join(specsRoot, "project", "PROJECT.md"):               "# product spec body\n",
		filepath.Join(specsRoot, "project", "STATE.md"):                 "# state body\n",
		filepath.Join(specsRoot, "codebase", "STACK.md"):                "node,\n",
		filepath.Join(specsRoot, "codebase", "ARCHITECTURE.md"):         "layers,\n",
		filepath.Join(specsRoot, "features", "sprint-001", "spec.md"):   "# contract body\n",
		filepath.Join(specsRoot, "features", "sprint-001", "design.md"): "# design body\n",
		filepath.Join(specsRoot, "features", "sprint-001", "tasks.md"):  "# tasks body\n",
	}
	for path, want := range expectations {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("expected migrated file %s: %v", path, err)
		}
		if string(got) != want {
			t.Fatalf("%s content drift: got %q want %q", path, got, want)
		}
	}

	for _, legacy := range []string{
		filepath.Join(harnessDir, "spec.md"),
		filepath.Join(harnessDir, "progress.md"),
		filepath.Join(harnessDir, "context", "STACK.md"),
		filepath.Join(harnessDir, "contracts", "sprint-001.md"),
		filepath.Join(harnessDir, "design", "sprint-001.md"),
		filepath.Join(harnessDir, "tasks", "sprint-001.md"),
	} {
		if _, err := os.Stat(legacy); !os.IsNotExist(err) {
			t.Fatalf("expected legacy file %s to be removed, got err=%v", legacy, err)
		}
	}

	// Lock file remains: it is runtime state, not migrated.
	if _, err := os.Stat(filepath.Join(harnessDir, "contracts", "sprint-001.lock.json")); err != nil {
		t.Fatalf("lock file should remain in .harness/contracts/: %v", err)
	}

	// Second run is a no-op (idempotent).
	summary, err := migrateLegacyArtifactsToSpecs(harnessDir)
	if err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	if len(summary) != 0 {
		t.Fatalf("expected idempotent second run, got moves: %v", summary)
	}
}
