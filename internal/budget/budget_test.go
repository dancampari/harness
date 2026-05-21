package budget

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mkfile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("x", size)), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestInspectSumsKnownFiles(t *testing.T) {
	harnessDir := t.TempDir()
	mkfile(t, filepath.Join(harnessDir, "spec.md"), 1000)
	mkfile(t, filepath.Join(harnessDir, "progress.md"), 2000)
	mkfile(t, filepath.Join(harnessDir, "agent-protocol.md"), 500)
	mkfile(t, filepath.Join(harnessDir, "context", "STACK.md"), 300)
	mkfile(t, filepath.Join(harnessDir, "context", "ARCHITECTURE.md"), 700)
	mkfile(t, filepath.Join(harnessDir, "contracts", "sprint-001.md"), 1500)

	snap, err := Inspect(harnessDir, 1)
	if err != nil {
		t.Fatal(err)
	}
	wantBytes := int64(1000 + 2000 + 500 + 300 + 700 + 1500)
	if snap.TotalBytes != wantBytes {
		t.Fatalf("expected total bytes %d, got %d", wantBytes, snap.TotalBytes)
	}
	if len(snap.Files) != 6 {
		t.Fatalf("expected 6 files counted, got %d", len(snap.Files))
	}
	if snap.TokenEstimate != int64(float64(wantBytes)*TokensPerByte) {
		t.Fatalf("token estimate mismatch: %d", snap.TokenEstimate)
	}
}

func TestInspectIgnoresMissingSprintArtifacts(t *testing.T) {
	harnessDir := t.TempDir()
	mkfile(t, filepath.Join(harnessDir, "spec.md"), 500)
	// sprint 1 exists but no design/tasks files.
	mkfile(t, filepath.Join(harnessDir, "contracts", "sprint-001.md"), 800)

	snap, err := Inspect(harnessDir, 1)
	if err != nil {
		t.Fatal(err)
	}
	if snap.TotalBytes != 1300 {
		t.Fatalf("expected 1300 bytes (spec + contract only), got %d", snap.TotalBytes)
	}
	for _, f := range snap.Files {
		if strings.Contains(f.Path, "design") || strings.Contains(f.Path, "tasks") {
			t.Fatalf("expected design/tasks to be skipped when absent, got %s", f.Path)
		}
	}
}

func TestOverBudgetFlagsLargeBundles(t *testing.T) {
	harnessDir := t.TempDir()
	// 200_000 bytes ≈ 50_000 tokens, over the 40k soft limit.
	mkfile(t, filepath.Join(harnessDir, "progress.md"), 200_000)

	snap, err := Inspect(harnessDir, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !snap.OverBudget() {
		t.Fatalf("expected OverBudget=true for %d tokens vs %d soft limit",
			snap.TokenEstimate, snap.SoftLimitTokens)
	}
}

func TestInspectWithoutSprintSkipsSprintFiles(t *testing.T) {
	harnessDir := t.TempDir()
	mkfile(t, filepath.Join(harnessDir, "spec.md"), 100)
	mkfile(t, filepath.Join(harnessDir, "contracts", "sprint-001.md"), 800)

	snap, err := Inspect(harnessDir, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range snap.Files {
		if strings.Contains(f.Path, "sprint-001.md") {
			t.Fatalf("expected sprint-001.md to be excluded when sprintNumber=0, got %s", f.Path)
		}
	}
}
