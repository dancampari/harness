package watch

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const watchTestConfig = `version: "2"
stack: unknown
adapters: {}
thresholds:
  contract: 80
weights:
  contract: 100
e2e:
  required: true
  runner: playwright
  screenshot_dir: .harness/screenshots
  baseline_dir: .harness/screenshots/baseline
memory:
  retention_days: 365
  trend_window: 10
`

func setupWatchTestRepo(t *testing.T) (repoRoot, harnessDir string) {
	t.Helper()
	repoRoot = t.TempDir()
	harnessDir = filepath.Join(repoRoot, ".harness")
	if err := os.MkdirAll(harnessDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(harnessDir, "config.yaml"), []byte(watchTestConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	return
}

func TestRunOnceWritesReportAndLatestPointer(t *testing.T) {
	repoRoot, harnessDir := setupWatchTestRepo(t)

	result, err := RunOnce(context.Background(), repoRoot, harnessDir)
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if result.ReportPath == "" {
		t.Fatal("expected ReportPath to be set")
	}
	if _, err := os.Stat(result.ReportPath); err != nil {
		t.Fatalf("expected report file at %s: %v", result.ReportPath, err)
	}
	latestPath := filepath.Join(harnessDir, "watch", "latest.json")
	if _, err := os.Stat(latestPath); err != nil {
		t.Fatalf("expected latest pointer at %s: %v", latestPath, err)
	}
	if result.LatestPath != latestPath {
		t.Fatalf("expected LatestPath %s, got %s", latestPath, result.LatestPath)
	}
}

func TestRunOnceDoesNotTouchSprintReports(t *testing.T) {
	repoRoot, harnessDir := setupWatchTestRepo(t)
	// Pre-existing sprint report. Watch must not overwrite it.
	sprintReportsDir := filepath.Join(harnessDir, "reports")
	if err := os.MkdirAll(sprintReportsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	original := []byte(`{"verdict":"PASS","total_score":100}`)
	if err := os.WriteFile(filepath.Join(sprintReportsDir, "sprint-001.json"), original, 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := RunOnce(context.Background(), repoRoot, harnessDir); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(sprintReportsDir, "sprint-001.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Fatalf("watch must not modify sprint reports, got %s", got)
	}
}

func TestRunOnceComputesDeltaAgainstPreviousLatest(t *testing.T) {
	repoRoot, harnessDir := setupWatchTestRepo(t)

	// Seed a previous "latest" with 0 findings.
	prev := Report{
		SchemaVersion: schemaVersion,
		Findings:      0,
		Dimensions:    map[string]DimSummary{"contract": {Score: 100, Findings: 0, Passed: true}},
	}
	watchDir := filepath.Join(harnessDir, "watch")
	if err := os.MkdirAll(watchDir, 0o755); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(prev)
	if err := os.WriteFile(filepath.Join(watchDir, "latest.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := RunOnce(context.Background(), repoRoot, harnessDir)
	if err != nil {
		t.Fatal(err)
	}
	if result.Report.Delta == nil {
		t.Fatal("expected Delta to be populated when a previous latest exists")
	}
	if result.Report.Delta.FindingsBefore != 0 {
		t.Fatalf("expected FindingsBefore=0, got %d", result.Report.Delta.FindingsBefore)
	}
}

func TestRunOnceRequiresValidConfig(t *testing.T) {
	root := t.TempDir()
	harnessDir := filepath.Join(root, ".harness")
	if err := os.MkdirAll(harnessDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// No config.yaml on purpose.
	if _, err := RunOnce(context.Background(), root, harnessDir); err == nil {
		t.Fatal("expected RunOnce to fail without config.yaml")
	}
}

func TestListReturnsTimestampedReportsExcludingLatest(t *testing.T) {
	_, harnessDir := setupWatchTestRepo(t)
	watchDir := filepath.Join(harnessDir, "watch")
	if err := os.MkdirAll(watchDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"20260101T000000Z.json", "20260102T000000Z.json", "latest.json"} {
		if err := os.WriteFile(filepath.Join(watchDir, name), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	files, err := List(harnessDir, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 timestamped reports (latest.json excluded), got %d", len(files))
	}
	// Descending sort: 20260102 before 20260101
	if filepath.Base(files[0]) != "20260102T000000Z.json" {
		t.Fatalf("expected newest first, got %s", filepath.Base(files[0]))
	}
}
