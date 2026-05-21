package sprint

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dancampari/harness/internal/agreement"
	"github.com/dancampari/harness/internal/evaluator"
	"github.com/dancampari/harness/internal/sensors"
	"github.com/dancampari/harness/internal/workspace"
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

func TestConsolidateRejectsWorkspaceDrift(t *testing.T) {
	repoRoot := t.TempDir()
	harnessDir := filepath.Join(repoRoot, ".harness")
	mustMkdirAll(t, filepath.Join(harnessDir, "contracts"))
	mustMkdirAll(t, filepath.Join(harnessDir, "reports"))
	mustMkdirAll(t, filepath.Join(harnessDir, "evaluations"))

	writeTestContract(t, harnessDir, 1, `# Sprint 001 — workspace pin

## Goal
Ensure consolidation refuses to score a stale report.

## Deliverables
- `+"`src/index.ts`"+` exports: `+"`run`"+`

## Acceptance Criteria
| # | Criterion              | Threshold |
|---|------------------------|-----------|
| 1 | Implementation present | 8/10      |
`)
	writeTestFile(t, filepath.Join(repoRoot, "src", "index.ts"),
		"export function run() { return 1; }\n")
	writeTestConfig(t, harnessDir)

	ag := agreement.NewManager(harnessDir)
	if _, err := ag.Propose(1); err != nil {
		t.Fatal(err)
	}
	if _, err := ag.Approve(1, "planner"); err != nil {
		t.Fatal(err)
	}
	if _, err := ag.Approve(1, "tester"); err != nil {
		t.Fatal(err)
	}

	currentSHA, err := workspace.Hash(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	report := evaluator.EvaluationResult{
		SchemaVersion: "2",
		Timestamp:     time.Now().UTC(),
		SprintNumber:  1,
		TotalScore:    100,
		Verdict:       "PASS",
		Dimensions:    map[string]evaluator.DimensionScore{},
		Process:       evaluator.ProcessInfo{Isolated: true, WorkspaceSHA: currentSHA},
		ContractCheck: evaluator.ContractCheckResult{Status: "satisfied", Score: 100},
	}
	writeJSONReport(t, harnessDir, 1, report)

	mgr, err := NewManager(harnessDir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	if _, err := mgr.Consolidate(false); err != nil {
		t.Fatalf("expected first consolidate to succeed, got %v", err)
	}

	// Drift the workspace and call Consolidate again with the now-stale
	// stored WorkspaceSHA.
	writeTestFile(t, filepath.Join(repoRoot, "src", "index.ts"),
		"export function run() { return 2; }\n")
	if _, err := mgr.Consolidate(false); err == nil {
		t.Fatal("expected Consolidate to reject stale report after workspace drift")
	} else if !strings.Contains(err.Error(), "workspace changed after QA") {
		t.Fatalf("expected workspace drift error, got %v", err)
	}
}

func TestRepairBriefIncludesSuggestedFixes(t *testing.T) {
	repoRoot := t.TempDir()
	harnessDir := filepath.Join(repoRoot, ".harness")
	mustMkdirAll(t, filepath.Join(harnessDir, "contracts"))
	mustMkdirAll(t, filepath.Join(harnessDir, "reports"))
	mustMkdirAll(t, filepath.Join(harnessDir, "evaluations"))
	mustMkdirAll(t, filepath.Join(harnessDir, "repairs"))

	writeTestContract(t, harnessDir, 1, `# Sprint 001 — repair brief hint test

## Goal
Verify the repair brief surfaces LLM-optimized hints.

## Deliverables
- `+"`src/index.ts`"+` exports: `+"`run`"+`

## Acceptance Criteria
| # | Criterion | Threshold |
|---|-----------|-----------|
| 1 | Works     | 8/10      |
`)
	writeTestConfig(t, harnessDir)

	report := evaluator.EvaluationResult{
		SchemaVersion: "2",
		Timestamp:     time.Now().UTC(),
		SprintNumber:  1,
		TotalScore:    60,
		Verdict:       "FAIL",
		Dimensions: map[string]evaluator.DimensionScore{
			"correctness": {
				Dimension: "correctness",
				Score:     50,
				Threshold: 80,
				Passed:    false,
				Findings: []sensors.Finding{
					{
						Dimension: sensors.DimCorrectness,
						Severity:  sensors.SeverityHigh,
						Rule:      "no-unused-vars",
						Message:   "'foo' is defined but never used",
						File:      "src/index.ts",
						Line:      3,
						Hint:      sensors.LLMHint("no-unused-vars"),
					},
				},
			},
		},
		Process:       evaluator.ProcessInfo{Isolated: true},
		ContractCheck: evaluator.ContractCheckResult{Status: "satisfied", Score: 100},
	}
	writeJSONReport(t, harnessDir, 1, report)

	mgr, err := NewManager(harnessDir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	brief, err := mgr.WriteRepairBrief()
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(brief.LatestPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{
		"## Findings",
		"no-unused-vars",
		"## Suggested Fixes (LLM-optimized)",
		"Do NOT",
	} {
		if !strings.Contains(string(body), needle) {
			t.Fatalf("expected repair brief to contain %q, got:\n%s", needle, body)
		}
	}
}

func writeTestContract(t *testing.T, harnessDir string, n int, body string) {
	t.Helper()
	path := filepath.Join(harnessDir, "contracts", fmt.Sprintf("sprint-%03d.md", n))
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeTestConfig(t *testing.T, harnessDir string) {
	t.Helper()
	cfg := `version: "2"
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
	if err := os.WriteFile(filepath.Join(harnessDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeJSONReport(t *testing.T, harnessDir string, n int, report evaluator.EvaluationResult) {
	t.Helper()
	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(harnessDir, "reports", fmt.Sprintf("sprint-%03d.json", n))
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}
