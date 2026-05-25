package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestE2ESetupInstallsSpecDrivenAutomationForAllCLIs(t *testing.T) {
	exe := buildHarness(t)
	root := t.TempDir()
	writeIntegrationFile(t, filepath.Join(root, "package.json"), `{"name":"agent-automation","type":"module"}`)

	out := runHarness(t, exe, root, "setup", "--yes", "--cli", "all", "--planning", "spec-driven", "--scope", "project")
	assertContains(t, string(out), "Ready.")
	assertFileExists(t, filepath.Join(root, ".harness", "bin", executableNameForTest("harness")))

	for _, check := range []struct {
		path    string
		needles []string
	}{
		{".harness/agent-protocol.md", []string{"harness.doctor_fix", "doctor --fix", "sprint repair", "Spec-driven automation"}},
		{".harness/skills/tlc-spec-driven/SKILL.md", []string{"Specify", "Design", "Tasks", "Execute"}},
		{".harness/skills/harness-gate/SKILL.md", []string{"Agreement gate", "QA dimensions", "Events log"}},
		{"AGENTS.md", []string{"harness.doctor_fix", "harness_contract_reviewer", "harness_task_worker"}},
		{filepath.Join(".codex", "agents", "harness-task-worker.toml"), []string{"harness_task_worker", "harness doctor --fix", "AGREED"}},
		{"CLAUDE.md", []string{"harness.doctor_fix", "harness-contract-reviewer", "harness-task-worker"}},
		{filepath.Join(".claude", "agents", "harness-task-worker.md"), []string{"harness-task-worker", "harness doctor --fix", "AGREED"}},
		{filepath.Join(".cursor", "rules", "harness.mdc"), []string{"Harness Engineering", "doctor --fix", "sprint repair"}},
	} {
		for _, needle := range check.needles {
			assertFileContains(t, filepath.Join(root, check.path), needle)
		}
	}

	doctor := runHarness(t, exe, root, "doctor", "--fix")
	assertContains(t, string(doctor), "Auto-fix:")
}

func TestE2ESetupLocalBinaryRunsAgreedFeatureThroughScore(t *testing.T) {
	exe := buildHarness(t)
	root := t.TempDir()

	runHarness(t, exe, root, "setup", "--yes", "--cli", "none", "--planning", "spec-driven", "--scope", "project")
	localExe := filepath.Join(root, ".harness", "bin", executableNameForTest("harness"))
	assertFileExists(t, localExe)

	runHarness(t, localExe, root, "feature", "new", "active local binary")
	writeIntegrationFile(t, filepath.Join(root, ".specs", "features", "sprint-001", "spec.md"), `# Sprint 001 - Active Local Binary

## Goal
Validate that the project-local Harness binary can drive an agreed feature through QA and score. This is the installed-binary path agents use after setup.

## Size
small

## Requirements
- REQ-001: The local Harness binary can complete the full agreement and QA lifecycle.

## Deliverables
- `+"`README.md`"+` (REQ-001)

## Acceptance Criteria
| # | REQ     | Criterion                                                                        | Evidence      | Threshold |
|---|---------|----------------------------------------------------------------------------------|---------------|-----------|
| 1 | REQ-001 | WHEN README.md exists THEN the system SHALL satisfy the agreed feature contract   | e2e:README.md | 8/10      |

## Edge Cases
- Fresh project with no previous feature reports.

## Out of Scope
- Exercising stack-specific tool adapters.

## Constraints
- coverage_min: 0
`)
	runHarness(t, localExe, root, "feature", "propose")
	runHarness(t, localExe, root, "feature", "approve", "--role", "planner")
	runHarness(t, localExe, root, "feature", "approve", "--role", "tester")
	writeIntegrationFile(t, filepath.Join(root, "README.md"), "# Active Local Binary\n")

	qa := runHarness(t, localExe, root, "feature", "qa", "--format", "json")
	var result struct {
		Verdict    string `json:"verdict"`
		TotalScore int    `json:"total_score"`
		Dimensions map[string]struct {
			Score       int      `json:"score"`
			Passed      bool     `json:"passed"`
			SensorsUsed []string `json:"sensors_used"`
		} `json:"dimensions"`
	}
	if err := json.Unmarshal(qa, &result); err != nil {
		t.Fatalf("parse local-binary QA result: %v\n%s", err, qa)
	}
	if result.Verdict != "PASS" || result.TotalScore != 100 {
		t.Fatalf("expected PASS 100 from local binary, got %+v\n%s", result, qa)
	}
	contract := result.Dimensions["contract"]
	if !contract.Passed || contract.Score != 100 {
		t.Fatalf("expected passing contract dimension, got %+v", contract)
	}
	for _, sensor := range []string{"spec-deviation-scanner", "test-count-tracker", "contract-validator"} {
		if !containsString(contract.SensorsUsed, sensor) {
			t.Fatalf("expected contract sensor %s in %+v", sensor, contract.SensorsUsed)
		}
	}

	score := runHarness(t, localExe, root, "feature", "score")
	assertContains(t, string(score), "Sprint 001 scored")
	assertFileContains(t, filepath.Join(root, ".harness", "reports", "sprint-001.json"), `"verdict": "PASS"`)
	assertFileContains(t, filepath.Join(root, ".specs", "project", "STATE.md"), "Sprint 001")
}

func TestE2EGuardBlocksProductWritesUntilContractAgreement(t *testing.T) {
	exe := buildHarness(t)
	root := t.TempDir()
	writeIntegrationFile(t, filepath.Join(root, "src", "app.ts"), `export const app = "draft";`)

	runHarness(t, exe, root, "init", "--cli", "none", "--skills", "off")
	runHarness(t, exe, root, "sprint", "new", "guard product edits")
	writeIntegrationFile(t, filepath.Join(root, ".harness", "contracts", "sprint-001.md"), validContract("guard product edits", "src/app.ts", "app"))

	deny := runGuard(t, exe, root, map[string]any{
		"hook_event_name": "PreToolUse",
		"cwd":             root,
		"tool_name":       "Edit",
		"tool_input": map[string]any{
			"file_path": "src/app.ts",
		},
	})
	assertContains(t, deny, `"permissionDecision":"deny"`)
	assertContains(t, deny, "Do not edit product files before planner/tester agreement")

	allowedContractEdit := runGuard(t, exe, root, map[string]any{
		"hook_event_name": "PreToolUse",
		"cwd":             root,
		"tool_name":       "Edit",
		"tool_input": map[string]any{
			"file_path": ".harness/contracts/sprint-001.md",
		},
	})
	if strings.TrimSpace(allowedContractEdit) != "" {
		t.Fatalf("expected contract edits to be allowed before agreement, got %s", allowedContractEdit)
	}

	runHarness(t, exe, root, "contract", "propose")
	runHarness(t, exe, root, "contract", "approve", "--role", "planner")
	stillDenied := runGuard(t, exe, root, map[string]any{
		"hook_event_name": "PreToolUse",
		"cwd":             root,
		"tool_name":       "Write",
		"tool_input": map[string]any{
			"path": "src/app.ts",
		},
	})
	assertContains(t, stillDenied, `"permissionDecision":"deny"`)

	runHarness(t, exe, root, "contract", "approve", "--role", "tester")
	allowedProductEdit := runGuard(t, exe, root, map[string]any{
		"hook_event_name": "PreToolUse",
		"cwd":             root,
		"tool_name":       "Edit",
		"tool_input": map[string]any{
			"file_path": "src/app.ts",
		},
	})
	if strings.TrimSpace(allowedProductEdit) != "" {
		t.Fatalf("expected product edits to be allowed after agreement, got %s", allowedProductEdit)
	}
}

func TestE2EContractLifecycleProducesCurrentReportsAndProgress(t *testing.T) {
	exe := buildHarness(t)
	root := t.TempDir()
	writeIntegrationFile(t, filepath.Join(root, "delivered.txt"), "ok\n")

	runHarness(t, exe, root, "init", "--cli", "none", "--skills", "off")
	runHarness(t, exe, root, "sprint", "new", "contract only lifecycle")
	writeIntegrationFile(t, filepath.Join(root, ".harness", "contracts", "sprint-001.md"), validContract("contract only lifecycle", "delivered.txt", ""))
	runHarness(t, exe, root, "contract", "propose")
	runHarness(t, exe, root, "contract", "approve", "--role", "planner")
	runHarness(t, exe, root, "contract", "approve", "--role", "tester")

	qa := runHarness(t, exe, root, "sprint", "qa", "--format", "json")
	var result struct {
		SchemaVersion string `json:"schema_version"`
		Verdict       string `json:"verdict"`
		TotalScore    int    `json:"total_score"`
		Process       struct {
			Isolated           bool `json:"isolated"`
			ContextEnvStripped bool `json:"context_env_stripped"`
		} `json:"process"`
		Dimensions map[string]struct {
			Passed bool `json:"passed"`
		} `json:"dimensions"`
	}
	if err := json.Unmarshal(qa, &result); err != nil {
		t.Fatalf("parse QA result: %v\n%s", err, qa)
	}
	if result.SchemaVersion != "2" || result.Verdict != "PASS" || result.TotalScore != 100 {
		t.Fatalf("expected schema v2 PASS 100, got %+v\n%s", result, qa)
	}
	if !result.Process.Isolated || !result.Process.ContextEnvStripped {
		t.Fatalf("expected isolated subprocess with stripped agent env, got %+v", result.Process)
	}
	if dim, ok := result.Dimensions["contract"]; !ok || !dim.Passed {
		t.Fatalf("expected passing contract dimension, got %+v", result.Dimensions)
	}

	assertFileContains(t, filepath.Join(root, ".harness", "reports", "latest.json"), `"verdict": "PASS"`)
	assertFileContains(t, filepath.Join(root, ".harness", "evaluations", "sprint-001.md"), "## Verdict: PASS")

	score := runHarness(t, exe, root, "sprint", "score")
	assertContains(t, string(score), "Sprint 001 scored")
	assertFileContains(t, filepath.Join(root, ".specs", "project", "STATE.md"), "Sprint 001")
	assertContains(t, string(runHarness(t, exe, root, "sprint", "status")), "QA=pass")
	assertContains(t, string(runHarness(t, exe, root, "sprint", "list")), "PASS")
	assertContains(t, string(runHarness(t, exe, root, "progress")), "Sprint 001")
	assertContains(t, string(runHarness(t, exe, root, "spec")), "Product Specification")
	assertContains(t, string(runHarness(t, exe, root, "trend")), "Score trend")
	assertContains(t, string(runHarness(t, exe, root, "sprint", "repair")), "No repair required")
}

func TestE2ESpecDeviationWithoutReasonBlocksQA(t *testing.T) {
	exe := buildHarness(t)
	root := t.TempDir()

	runHarness(t, exe, root, "init", "--cli", "none", "--planning", "manual")
	runHarness(t, exe, root, "feature", "new", "blocking spec deviation")
	writeIntegrationFile(t, filepath.Join(root, ".specs", "features", "sprint-001", "spec.md"), `# Sprint 001 - Blocking Spec Deviation

## Goal
Prove orphan SPEC_DEVIATION markers cannot pass the contract gate.

## Deliverables
- `+"`README.md`"+`

## Acceptance Criteria
| # | Criterion | Threshold |
|---|-----------|-----------|
| 1 | README exists | 8/10 |
`)
	runHarness(t, exe, root, "feature", "propose")
	runHarness(t, exe, root, "feature", "approve", "--role", "planner")
	runHarness(t, exe, root, "feature", "approve", "--role", "tester")
	writeIntegrationFile(t, filepath.Join(root, "README.md"), "# Deviation\n")
	writeIntegrationFile(t, filepath.Join(root, "main.go"), "package main\n\n// SPEC_DEVIATION\nfunc main() {}\n")

	qa := runHarness(t, exe, root, "feature", "qa", "--format", "json")
	var result struct {
		Verdict    string `json:"verdict"`
		Dimensions map[string]struct {
			Score    int `json:"score"`
			Findings []struct {
				Rule string `json:"rule"`
			} `json:"findings"`
		} `json:"dimensions"`
	}
	if err := json.Unmarshal(qa, &result); err != nil {
		t.Fatalf("parse QA result: %v\n%s", err, qa)
	}
	if result.Verdict != "FAIL" {
		t.Fatalf("expected blocking SPEC_DEVIATION to fail QA, got %s\n%s", result.Verdict, qa)
	}
	contract := result.Dimensions["contract"]
	if contract.Score != 0 {
		t.Fatalf("expected contract score 0, got %+v", contract)
	}
	if !hasFindingRule(contract.Findings, "spec-deviation-without-reason") {
		t.Fatalf("expected spec-deviation finding, got %+v", contract.Findings)
	}
}

func TestE2EPreCommitHookBlocksFastFail(t *testing.T) {
	exe := buildHarness(t)
	root := t.TempDir()

	// Pretend this is a git repo so install-hooks --pre-commit writes the
	// hook. We do not need actual git to run the hook script directly.
	if err := os.MkdirAll(filepath.Join(root, ".git", "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}

	runHarness(t, exe, root, "init", "--cli", "none", "--skills", "off")
	runHarness(t, exe, root, "install-hooks", "--cli", "none", "--pre-commit")

	hookPath := filepath.Join(root, ".git", "hooks", "pre-commit")
	if _, err := os.Stat(hookPath); err != nil {
		t.Fatalf("expected pre-commit hook to be installed at %s: %v", hookPath, err)
	}
	b, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{"sprint qa --fast", "--no-verify"} {
		if !strings.Contains(string(b), needle) {
			t.Fatalf("expected pre-commit hook to mention %q, got:\n%s", needle, b)
		}
	}

	// Doctor must surface the new hook.
	doctor := runHarness(t, exe, root, "doctor")
	if !strings.Contains(string(doctor), "git pre-commit runs fast QA") {
		t.Fatalf("expected doctor to report pre-commit hook, got:\n%s", doctor)
	}
}

func TestE2EFastQASkipsAgreementGateAndSlowDimensions(t *testing.T) {
	exe := buildHarness(t)
	root := t.TempDir()
	writeIntegrationFile(t, filepath.Join(root, "delivered.txt"), "ok\n")

	runHarness(t, exe, root, "init", "--cli", "none", "--skills", "off")
	runHarness(t, exe, root, "sprint", "new", "fast qa shift-left")
	writeIntegrationFile(t, filepath.Join(root, ".harness", "contracts", "sprint-001.md"),
		validContract("fast qa shift-left", "delivered.txt", ""))
	// Note: no contract propose/approve. Fast mode must still run.

	out := runHarness(t, exe, root, "sprint", "qa", "--fast", "--format", "json")
	var result struct {
		Verdict    string `json:"verdict"`
		Dimensions map[string]struct {
			Skipped bool `json:"skipped"`
			Passed  bool `json:"passed"`
		} `json:"dimensions"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("parse fast qa: %v\n%s", err, out)
	}
	if result.Verdict != "PASS" {
		t.Fatalf("expected PASS for fast mode with structural contract check, got %s\n%s", result.Verdict, out)
	}
	contract, ok := result.Dimensions["contract"]
	if !ok {
		t.Fatalf("expected contract dimension in fast mode, got %+v", result.Dimensions)
	}
	if contract.Skipped {
		t.Fatal("contract dimension must run in fast mode; it is the structural check the hook protects")
	}

	// Fast mode must NOT overwrite the canonical report — there should
	// not be a report file at all yet because we never ran a full QA.
	reportPath := filepath.Join(root, ".harness", "reports", "sprint-001.json")
	if _, err := os.Stat(reportPath); err == nil {
		t.Fatalf("fast mode should not write %s; that is the slot for full QA only", reportPath)
	}
}

func TestE2EContextSizeReportsHarnessBundle(t *testing.T) {
	exe := buildHarness(t)
	root := t.TempDir()
	runHarness(t, exe, root, "init", "--cli", "none", "--skills", "off")

	out := runHarness(t, exe, root, "context", "size", "--format", "json")
	var report struct {
		TotalBytes    int64 `json:"total_bytes"`
		TokenEstimate int64 `json:"token_estimate"`
		SoftLimit     int64 `json:"soft_limit_tokens"`
	}
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatalf("parse context size: %v\n%s", err, out)
	}
	if report.SoftLimit == 0 {
		t.Fatal("expected non-zero soft limit")
	}
	if report.TotalBytes <= 0 {
		t.Fatalf("expected harness init to produce some context, got %d bytes", report.TotalBytes)
	}
}

func TestE2EWatchOnceWritesReportWithoutSprint(t *testing.T) {
	exe := buildHarness(t)
	root := t.TempDir()
	runHarness(t, exe, root, "init", "--cli", "none", "--skills", "off")

	// No sprint contract, no agreed gate; watch must still produce a
	// report because drift monitoring is independent of the sprint cycle.
	out := runHarness(t, exe, root, "watch", "once", "--format", "json")
	var report struct {
		SchemaVersion string `json:"schema_version"`
		Findings      int    `json:"findings"`
	}
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatalf("parse watch report: %v\n%s", err, out)
	}
	if report.SchemaVersion == "" {
		t.Fatalf("expected schema_version in watch report, got %s", out)
	}
	if _, err := os.Stat(filepath.Join(root, ".harness", "watch", "latest.json")); err != nil {
		t.Fatalf("expected .harness/watch/latest.json to exist: %v", err)
	}

	// Doctor must mention the watch report after the first run.
	doctor := runHarness(t, exe, root, "doctor")
	if !strings.Contains(string(doctor), "drift watch report present") {
		t.Fatalf("expected doctor to recognise watch report, got:\n%s", doctor)
	}
}

func TestE2ENodeProjectRunsFullQualityGateWithDeterministicLocalToolchain(t *testing.T) {
	exe := buildHarness(t)
	root := t.TempDir()
	fakeBin := filepath.Join(root, ".fake-bin")
	writeNodeFixture(t, root, fakeBin)

	env := withPrependedPath(fakeBin)
	runHarnessEnv(t, exe, root, env, "init", "--cli", "none", "--skills", "off")
	runHarnessEnv(t, exe, root, env, "sprint", "new", "node full quality gate")
	writeIntegrationFile(t, filepath.Join(root, ".harness", "contracts", "sprint-001.md"), validContract("node full quality gate", "src/index.ts", "sum"))
	runHarnessEnv(t, exe, root, env, "contract", "propose")
	runHarnessEnv(t, exe, root, env, "contract", "approve", "--role", "planner")
	runHarnessEnv(t, exe, root, env, "contract", "approve", "--role", "tester")

	accepted := runHarnessEnv(t, exe, root, env, "sprint", "qa", "--format", "json", "--accept-screenshots")
	var first nodeQAE2EResult
	if err := json.Unmarshal(accepted, &first); err != nil {
		t.Fatalf("parse first QA result: %v\n%s", err, accepted)
	}
	if first.Verdict != "PASS" {
		t.Fatalf("expected first QA with accepted screenshot baseline to pass, got %s\n%s", first.Verdict, accepted)
	}
	if !first.Process.AcceptingScreenshots {
		t.Fatalf("expected accepting_screenshots marker in subprocess: %+v", first.Process)
	}
	assertFileExists(t, filepath.Join(root, ".harness", "screenshots", "baseline", "home.png"))

	qa := runHarnessEnv(t, exe, root, env, "sprint", "qa", "--format", "json")
	var result nodeQAE2EResult
	if err := json.Unmarshal(qa, &result); err != nil {
		t.Fatalf("parse second QA result: %v\n%s", err, qa)
	}
	if result.Verdict != "PASS" || result.TotalScore < 95 {
		t.Fatalf("expected full Node gate to pass with high score, got %+v\n%s", result, qa)
	}
	for _, dim := range []string{"correctness", "coverage", "complexity", "security", "architecture", "contract", "e2e"} {
		score, ok := result.Dimensions[dim]
		if !ok || !score.Passed {
			t.Fatalf("expected dimension %s to pass, got %+v", dim, score)
		}
	}
	for _, sensor := range []string{"eslint", "vitest", "vitest-coverage", "npm-audit", "js-complexity", "js-architecture", "playwright"} {
		if !sensorExecuted(result.Sensors, sensor) {
			t.Fatalf("expected sensor %s to execute, got %+v", sensor, result.Sensors)
		}
	}
}

func TestE2EDoctorFixRepairsStackDefaultsForBroaderStacks(t *testing.T) {
	exe := buildHarness(t)
	cases := []struct {
		name    string
		files   map[string]string
		needles []string
	}{
		{
			name: "python",
			files: map[string]string{
				"pyproject.toml": `[project]` + "\n" + `name = "py-e2e"`,
			},
			needles: []string{"stack: python", "- ruff", "- pytest-cov", "- pip-audit"},
		},
		{
			name: "go",
			files: map[string]string{
				"go.mod": "module go-e2e\n\ngo 1.24\n",
			},
			needles: []string{"stack: go", "- go-vet", "- go-test-coverage", "- govulncheck"},
		},
		{
			name: "rust",
			files: map[string]string{
				"Cargo.toml": "[package]\nname = \"rust-e2e\"\nversion = \"0.1.0\"\nedition = \"2021\"\n",
			},
			needles: []string{"stack: rust", "- clippy", "- cargo-audit"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for path, content := range tc.files {
				writeIntegrationFile(t, filepath.Join(root, path), content)
			}
			runHarness(t, exe, root, "init", "--cli", "none", "--skills", "off")
			writeIntegrationFile(t, filepath.Join(root, ".harness", "config.yaml"), contractOnlyConfig())

			out := runHarness(t, exe, root, "doctor", "--fix")
			assertContains(t, string(out), "Auto-fix:")
			cfg, err := os.ReadFile(filepath.Join(root, ".harness", "config.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			for _, needle := range tc.needles {
				if !strings.Contains(string(cfg), needle) {
					t.Fatalf("expected fixed %s config to contain %q\n%s", tc.name, needle, cfg)
				}
			}
		})
	}
}

func TestE2ENpmPackageExposesHarnessBinary(t *testing.T) {
	packDir := t.TempDir()
	cmd := exec.Command("npm", "pack", "--pack-destination", packDir)
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("npm pack failed: %v\n%s", err, out)
	}
	tarball := filepath.Join(packDir, strings.TrimSpace(lastNonEmptyLine(string(out))))
	if _, err := os.Stat(tarball); err != nil {
		t.Fatalf("expected npm tarball %s: %v\nnpm output:\n%s", tarball, err, out)
	}

	tmp := t.TempDir()
	version := exec.Command("npm", "exec", "--yes", "--package", tarball, "--", "harness", "--version")
	version.Dir = tmp
	versionOut, err := version.CombinedOutput()
	if err != nil {
		t.Fatalf("npm exec harness --version failed: %v\n%s", err, versionOut)
	}
	assertContains(t, string(versionOut), "harness version")

	help := exec.Command("npm", "exec", "--yes", "--package", tarball, "--", "harness", "doctor", "--help")
	help.Dir = tmp
	helpOut, err := help.CombinedOutput()
	if err != nil {
		t.Fatalf("npm exec harness doctor --help failed: %v\n%s", err, helpOut)
	}
	assertContains(t, string(helpOut), "--fix")
}

type nodeQAE2EResult struct {
	Verdict    string `json:"verdict"`
	TotalScore int    `json:"total_score"`
	Process    struct {
		AcceptingScreenshots bool `json:"accepting_screenshots"`
	} `json:"process"`
	Dimensions map[string]struct {
		Passed bool `json:"passed"`
	} `json:"dimensions"`
	Sensors []struct {
		Name     string `json:"name"`
		Executed bool   `json:"executed"`
	} `json:"sensors"`
}

func validContract(goal, deliverable, export string) string {
	exportText := ""
	if export != "" {
		exportText = " exports: `" + export + "`"
	}
	return fmt.Sprintf(`# Sprint 001 - %s

## Goal
Deliver %s with deterministic Harness validation.

## Deliverables
- `+"`%s`"+`%s

## Acceptance Criteria
| # | Criterion | Threshold |
|---|-----------|-----------|
| 1 | The declared deliverable exists and satisfies the current sprint contract | 8/10 |

## Constraints
- max_function_complexity: 10
`, goal, goal, deliverable, exportText)
}

func contractOnlyConfig() string {
	return `version: "2"
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
}

func writeNodeFixture(t *testing.T, root, fakeBin string) {
	t.Helper()
	writeIntegrationFile(t, filepath.Join(root, "package.json"), `{
  "name": "node-e2e",
  "type": "module",
  "dependencies": {
    "eslint": "1.0.0",
    "vitest": "1.0.0",
    "playwright": "1.0.0"
  }
}`)
	writeIntegrationFile(t, filepath.Join(root, "package-lock.json"), `{
  "name": "node-e2e",
  "lockfileVersion": 3,
  "requires": true,
  "packages": {
    "": {
      "name": "node-e2e",
      "dependencies": {
        "eslint": "1.0.0",
        "vitest": "1.0.0",
        "playwright": "1.0.0"
      }
    }
  }
}`)
	writeIntegrationFile(t, filepath.Join(root, "eslint.config.js"), "export default [];\n")
	writeIntegrationFile(t, filepath.Join(root, "playwright.config.js"), "export default {};\n")
	writeIntegrationFile(t, filepath.Join(root, "src", "index.ts"), `export function sum(a: number, b: number): number {
  return a + b;
}
`)
	writeIntegrationFile(t, filepath.Join(root, "src", "index.test.ts"), `import { sum } from "./index";
if (sum(1, 2) !== 3) throw new Error("sum failed");
`)
	writeFakeNodePackage(t, root, "eslint", `console.log("[]");`)
	writeFakeNodePackage(t, root, "vitest", fakeVitestScript())
	writeFakeNodePackage(t, root, "playwright", fakePlaywrightScript())
	writeFakeNpm(t, fakeBin)
}

func writeFakeNodePackage(t *testing.T, root, name, script string) {
	t.Helper()
	pkgDir := filepath.Join(root, "node_modules", name)
	binDir := filepath.Join(root, "node_modules", ".bin")
	writeIntegrationFile(t, filepath.Join(pkgDir, "package.json"),
		fmt.Sprintf(`{"name":%q,"version":"1.0.0","bin":{"%s":"bin.js"}}`, name, name))
	writeIntegrationFile(t, filepath.Join(pkgDir, "bin.js"), "#!/usr/bin/env node\n"+script+"\n")
	binPath := filepath.Join(binDir, name)
	writeIntegrationFile(t, binPath, "#!/usr/bin/env node\n"+script+"\n")
	if runtime.GOOS != "windows" {
		if err := os.Chmod(binPath, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeIntegrationFile(t, filepath.Join(binDir, name+".cmd"),
		"@echo off\r\nnode \"%~dp0\\"+name+"\" %*\r\n")
}

func writeFakeNpm(t *testing.T, fakeBin string) {
	t.Helper()
	script := `#!/usr/bin/env node
if (process.argv[2] === "audit") {
  console.log(JSON.stringify({ vulnerabilities: {}, metadata: { vulnerabilities: { low: 0, moderate: 0, high: 0, critical: 0 } } }));
  process.exit(0);
}
console.error("unexpected fake npm command: " + process.argv.slice(2).join(" "));
process.exit(1);
`
	path := filepath.Join(fakeBin, "npm")
	writeIntegrationFile(t, path, script)
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeIntegrationFile(t, filepath.Join(fakeBin, "npm.cmd"), "@echo off\r\nnode \"%~dp0\\npm\" %*\r\n")
}

func fakeVitestScript() string {
	return `const fs = require("fs");
const path = require("path");
if (process.argv.includes("--coverage")) {
  const dir = path.join(process.cwd(), "coverage");
  fs.mkdirSync(dir, { recursive: true });
  fs.writeFileSync(path.join(dir, "coverage-summary.json"), JSON.stringify({
    total: {
      lines: { pct: 95 },
      statements: { pct: 95 },
      functions: { pct: 95 },
      branches: { pct: 95 }
    }
  }));
}
console.log(JSON.stringify({
  numFailedTests: 0,
  numPassedTests: 1,
  numTotalTests: 1,
  testResults: []
}));`
}

func fakePlaywrightScript() string {
	return `const fs = require("fs");
const path = require("path");
const png = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAFgwJ/lL6mWQAAAABJRU5ErkJggg==", "base64");
const outDir = path.join(process.cwd(), ".harness", "playwright", "results");
fs.mkdirSync(outDir, { recursive: true });
const screenshot = path.join(outDir, "home.png");
fs.writeFileSync(screenshot, png);
console.log(JSON.stringify({
  stats: { expected: 1, unexpected: 0, flaky: 0, skipped: 0 },
  suites: [{
    title: "e2e",
    file: "tests/app.spec.ts",
    specs: [{
      title: "renders app",
      file: "tests/app.spec.ts",
      line: 1,
      ok: true,
      tests: [{
        status: "expected",
        results: [{
          status: "passed",
          attachments: [{ name: "home", path: screenshot, contentType: "image/png" }]
        }]
      }]
    }]
  }]
}));`
}

func runHarnessEnv(t *testing.T, exe, dir string, env []string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command(exe, args...)
	cmd.Dir = dir
	cmd.Env = env
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.Output()
	if err != nil {
		t.Fatalf("harness %s failed: %v\nstdout:\n%s\nstderr:\n%s",
			strings.Join(args, " "), err, stdout, stderr.String())
	}
	if stderr.Len() > 0 {
		t.Logf("harness %s stderr:\n%s", strings.Join(args, " "), stderr.String())
	}
	return stdout
}

func runGuard(t *testing.T, exe, dir string, payload map[string]any) string {
	t.Helper()
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(exe, "guard", "pre-tool")
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(string(b))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("guard pre-tool failed: %v\n%s", err, out)
	}
	return string(out)
}

func withPrependedPath(dir string) []string {
	env := os.Environ()
	pathKey := "PATH"
	for i, kv := range env {
		if strings.HasPrefix(strings.ToUpper(kv), "PATH=") {
			pathKey = kv[:strings.IndexByte(kv, '=')]
			env[i] = pathKey + "=" + dir + string(os.PathListSeparator) + kv[strings.IndexByte(kv, '=')+1:]
			return env
		}
	}
	return append(env, pathKey+"="+dir)
}

func sensorExecuted(statuses []struct {
	Name     string `json:"name"`
	Executed bool   `json:"executed"`
}, name string) bool {
	for _, status := range statuses {
		if status.Name == name && status.Executed {
			return true
		}
	}
	return false
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func hasFindingRule(findings []struct {
	Rule string `json:"rule"`
}, rule string) bool {
	for _, finding := range findings {
		if finding.Rule == rule {
			return true
		}
	}
	return false
}

func assertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
}

func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("expected output to contain %q\n%s", needle, haystack)
	}
}

func executableNameForTest(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func lastNonEmptyLine(s string) string {
	lines := strings.Split(s, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			return line
		}
	}
	return ""
}
