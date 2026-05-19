package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCLIFailsWhenActiveSensorsAreMissing(t *testing.T) {
	exe := buildHarness(t)
	root := t.TempDir()
	writeIntegrationFile(t, filepath.Join(root, "package.json"), `{"name":"missing-sensors"}`)
	writeIntegrationFile(t, filepath.Join(root, "index.js"), `export function hello() { return "hi"; }`)

	runHarness(t, exe, root, "init", "--cli", "none")
	runHarness(t, exe, root, "sprint", "new", "missing sensors")
	writeIntegrationFile(t, filepath.Join(root, ".harness", "contracts", "sprint-001.md"), `# Sprint 001 - missing sensors

## Goal
Prove active dimensions cannot pass without real sensors.

## Deliverables
- `+"`index.js`"+` exports: `+"`hello`"+`

## Acceptance Criteria
| # | Criterion | Threshold |
|---|-----------|-----------|
| 1 | Harness fails without active sensors | 8/10 |

## Constraints
- max_function_complexity: 10
`)

	out := runHarness(t, exe, root, "sprint", "qa", "--format", "json")
	var result struct {
		Verdict    string `json:"verdict"`
		Dimensions map[string]struct {
			Findings []struct {
				Rule string `json:"rule"`
			} `json:"findings"`
		} `json:"dimensions"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatal(err)
	}
	if result.Verdict != "FAIL" {
		t.Fatalf("expected FAIL with missing active sensors, got %s", result.Verdict)
	}
	if !hasRuleInDimensions(result.Dimensions, "missing-sensor") {
		t.Fatalf("expected missing-sensor finding, got %s", string(out))
	}
}

func TestCLIQAUsesIsolatedSubprocess(t *testing.T) {
	exe := buildHarness(t)
	root := t.TempDir()
	writeIntegrationFile(t, filepath.Join(root, "delivered.txt"), "ok")

	runHarness(t, exe, root, "init", "--cli", "none")
	runHarness(t, exe, root, "sprint", "new", "isolation")
	writeIntegrationFile(t, filepath.Join(root, ".harness", "contracts", "sprint-001.md"), `# Sprint 001 - isolation

## Goal
Prove evaluator runs in an isolated subprocess.

## Deliverables
- `+"`delivered.txt`"+`

## Acceptance Criteria
| # | Criterion | Threshold |
|---|-----------|-----------|
| 1 | Isolated evaluator runs | 8/10 |

## Constraints
- max_function_complexity: 10
`)

	cmd := exec.Command(exe, "sprint", "qa", "--format", "json")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "CLAUDE_SESSION_TOKEN=must-not-leak")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("harness sprint qa failed: %v\n%s", err, string(out))
	}
	var result struct {
		Process struct {
			PID                int  `json:"pid"`
			ParentPID          int  `json:"parent_pid"`
			Isolated           bool `json:"isolated"`
			ContextEnvStripped bool `json:"context_env_stripped"`
		} `json:"process"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatal(err)
	}
	if !result.Process.Isolated {
		t.Fatalf("expected isolated subprocess marker, got %+v", result.Process)
	}
	if !result.Process.ContextEnvStripped {
		t.Fatalf("expected builder env vars to be stripped, got %+v", result.Process)
	}
	if result.Process.PID == cmd.Process.Pid {
		t.Fatalf("expected evaluator child pid to differ from parent pid %d", cmd.Process.Pid)
	}
	if result.Process.ParentPID != cmd.Process.Pid {
		t.Fatalf("expected evaluator parent pid %d, got %+v", cmd.Process.Pid, result.Process)
	}
}

func buildHarness(t *testing.T) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "harness")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", exe, ".")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}
	return exe
}

func runHarness(t *testing.T, exe, dir string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command(exe, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("harness %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

func writeIntegrationFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func hasRuleInDimensions(dims map[string]struct {
	Findings []struct {
		Rule string `json:"rule"`
	} `json:"findings"`
}, rule string) bool {
	for _, dim := range dims {
		for _, finding := range dim.Findings {
			if finding.Rule == rule {
				return true
			}
		}
	}
	return false
}
