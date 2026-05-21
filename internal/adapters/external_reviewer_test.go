package adapters

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestFakeReviewerProcess is a test-binary trampoline: when invoked with
// HARNESS_FAKE_REVIEWER=1 it acts as the configured reviewer command,
// writing whatever HARNESS_FAKE_REVIEWER_OUTPUT contains to stdout and
// exiting with HARNESS_FAKE_REVIEWER_EXIT. Otherwise it short-circuits
// so the testing framework treats it as a no-op test case.
func TestFakeReviewerProcess(t *testing.T) {
	if os.Getenv("HARNESS_FAKE_REVIEWER") != "1" {
		return
	}
	_, _ = os.Stdout.Write([]byte(os.Getenv("HARNESS_FAKE_REVIEWER_OUTPUT")))
	exit := os.Getenv("HARNESS_FAKE_REVIEWER_EXIT")
	if exit == "" {
		os.Exit(0)
	}
	switch exit {
	case "0":
		os.Exit(0)
	default:
		os.Exit(1)
	}
}

func writeReviewerConfig(t *testing.T, root string, args ...string) {
	t.Helper()
	harnessDir := filepath.Join(root, ".harness")
	if err := os.MkdirAll(harnessDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// YAML hand-rolled to keep adapters tests free of yaml lib imports.
	cmdYAML := "[" + strings.Join(quotedArgs(args), ", ") + "]"
	body := `version: "2"
stack: unknown
adapters:
  review: [external-reviewer]
thresholds:
  contract: 80
  review: 70
weights:
  contract: 50
  review: 50
review:
  command: ` + cmdYAML + `
  timeout_seconds: 30
e2e:
  required: false
  runner: playwright
  screenshot_dir: .harness/screenshots
  baseline_dir: .harness/screenshots/baseline
memory:
  retention_days: 365
  trend_window: 10
`
	if err := os.WriteFile(filepath.Join(harnessDir, "config.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func quotedArgs(args []string) []string {
	// YAML single-quoted form keeps backslashes literal (no escape
	// interpretation), which matters on Windows where exe paths look
	// like `C:\Users\...` — double-quoted YAML would treat `\U` as a
	// unicode escape sequence.
	out := make([]string, len(args))
	for i, a := range args {
		out[i] = `'` + strings.ReplaceAll(a, `'`, `''`) + `'`
	}
	return out
}

func reviewerEnv(t *testing.T, output string, exit string) []string {
	t.Helper()
	env := append(os.Environ(),
		"HARNESS_FAKE_REVIEWER=1",
		"HARNESS_FAKE_REVIEWER_OUTPUT="+output,
	)
	if exit != "" {
		env = append(env, "HARNESS_FAKE_REVIEWER_EXIT="+exit)
	}
	return env
}

func TestExternalReviewerNotAvailableWithoutConfig(t *testing.T) {
	root := t.TempDir()
	if (ExternalReviewer{}).Available(root) {
		t.Fatal("expected Available=false when .harness/config.yaml is missing")
	}
}

func TestExternalReviewerNotAvailableWithoutCommand(t *testing.T) {
	root := t.TempDir()
	harnessDir := filepath.Join(root, ".harness")
	if err := os.MkdirAll(harnessDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `version: "2"
adapters:
  review: [external-reviewer]
thresholds:
  contract: 80
weights:
  contract: 100
`
	if err := os.WriteFile(filepath.Join(harnessDir, "config.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if (ExternalReviewer{}).Available(root) {
		t.Fatal("expected Available=false when review.command is missing")
	}
}

func TestExternalReviewerAvailableWithCommand(t *testing.T) {
	root := t.TempDir()
	testBin := testBinaryPath(t)
	writeReviewerConfig(t, root, testBin, "-test.run=TestFakeReviewerProcess")
	if !(ExternalReviewer{}).Available(root) {
		t.Fatal("expected Available=true when review.command is set")
	}
}

func TestExternalReviewerEmitsFindingsFromJSON(t *testing.T) {
	root := t.TempDir()
	testBin := testBinaryPath(t)
	writeReviewerConfig(t, root, testBin, "-test.run=TestFakeReviewerProcess")
	t.Setenv("HARNESS_FAKE_REVIEWER", "1")
	t.Setenv("HARNESS_FAKE_REVIEWER_OUTPUT", `{
  "schema_version": "1",
  "findings": [
    {"requirement_id": "REQ-001", "severity": "high", "rule": "missing-guard", "file": "src/auth.ts", "line": 12, "message": "auth check missing on admin route", "suggestion": "wrap with requireRole(admin) middleware"}
  ]
}`)

	result := (ExternalReviewer{}).Run(context.Background(), root)
	if result.Error != "" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	if len(result.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d (%+v)", len(result.Findings), result.Findings)
	}
	f := result.Findings[0]
	if !strings.Contains(f.Message, "REQ-001") {
		t.Fatalf("expected finding message to include REQ-001 prefix, got %q", f.Message)
	}
	if f.Hint != "wrap with requireRole(admin) middleware" {
		t.Fatalf("expected Hint to be populated from suggestion, got %q", f.Hint)
	}
	if f.Rule != "missing-guard" {
		t.Fatalf("expected rule, got %q", f.Rule)
	}
	if result.RawScore != 90 {
		t.Fatalf("expected RawScore=90 (100 - 1 finding * 10), got %d", result.RawScore)
	}
}

func TestExternalReviewerHandlesReviewerError(t *testing.T) {
	root := t.TempDir()
	testBin := testBinaryPath(t)
	writeReviewerConfig(t, root, testBin, "-test.run=TestFakeReviewerProcess")
	t.Setenv("HARNESS_FAKE_REVIEWER", "1")
	t.Setenv("HARNESS_FAKE_REVIEWER_OUTPUT", "boom")
	t.Setenv("HARNESS_FAKE_REVIEWER_EXIT", "1")

	result := (ExternalReviewer{}).Run(context.Background(), root)
	if result.Error == "" {
		t.Fatal("expected non-empty Error when reviewer exits non-zero")
	}
	if result.RawScore != 0 {
		t.Fatalf("expected RawScore=0 on error, got %d", result.RawScore)
	}
}

func TestExternalReviewerHandlesMalformedOutput(t *testing.T) {
	root := t.TempDir()
	testBin := testBinaryPath(t)
	writeReviewerConfig(t, root, testBin, "-test.run=TestFakeReviewerProcess")
	t.Setenv("HARNESS_FAKE_REVIEWER", "1")
	t.Setenv("HARNESS_FAKE_REVIEWER_OUTPUT", "not json at all")

	result := (ExternalReviewer{}).Run(context.Background(), root)
	if result.Error == "" {
		t.Fatal("expected parse error on non-JSON stdout")
	}
	if !strings.Contains(result.Error, "parse reviewer output") {
		t.Fatalf("expected 'parse reviewer output' error, got %q", result.Error)
	}
}

func testBinaryPath(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// On Windows the test binary may carry a .exe suffix already; the
	// path is whatever Go gave us.
	if runtime.GOOS == "windows" && !strings.HasSuffix(strings.ToLower(exe), ".exe") {
		exe += ".exe"
	}
	return exe
}
