package adapters

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestTestCountTrackerFirstRunNoFinding(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "feature.test.ts"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := TestCountTracker{}.Run(context.Background(), root)
	if len(res.Findings) != 0 {
		t.Fatalf("first run should not report regression, got %#v", res.Findings)
	}
	if _, err := os.Stat(filepath.Join(root, ".harness", "test-count.json")); err != nil {
		t.Fatalf("expected snapshot persisted after first run: %v", err)
	}
}

func TestTestCountTrackerDetectsDrop(t *testing.T) {
	root := t.TempDir()
	for i, name := range []string{"a.test.ts", "b.test.ts", "c.test.ts"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("ok"), 0o644); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}
	if res := (TestCountTracker{}).Run(context.Background(), root); len(res.Findings) != 0 {
		t.Fatalf("first run should not report regression, got %#v", res.Findings)
	}
	// Drop two tests, leaving only one.
	if err := os.Remove(filepath.Join(root, "a.test.ts")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "b.test.ts")); err != nil {
		t.Fatal(err)
	}
	res := TestCountTracker{}.Run(context.Background(), root)
	if len(res.Findings) != 1 {
		t.Fatalf("expected one regression finding after drop, got %#v", res.Findings)
	}
	if res.Findings[0].Rule != "test-count-regression" {
		t.Fatalf("unexpected rule: %s", res.Findings[0].Rule)
	}
	if res.RawScore != 0 {
		t.Fatalf("expected hard-fail RawScore 0 on regression, got %d", res.RawScore)
	}
}

func TestCountTestFilesIgnoresHarnessSkills(t *testing.T) {
	root := t.TempDir()
	skillsDir := filepath.Join(root, ".harness", "skills", "tlc-spec-driven")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "validate.test.md"), []byte("docs"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "feature.test.ts"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := countTestFiles(context.Background(), root); got != 1 {
		t.Fatalf("expected count 1 (skills doc ignored), got %d", got)
	}
}
