package harness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dancampari/harness/internal/adapters"
	"github.com/dancampari/harness/internal/config"
	"github.com/dancampari/harness/internal/detect"
	"github.com/spf13/cobra"
)

type doctorOptions struct {
	Strict bool
}

type doctorFinding struct {
	Severity string
	Message  string
}

type doctorAudit struct {
	failures []doctorFinding
	warnings []doctorFinding
}

func newDoctorCmd() *cobra.Command {
	var opts doctorOptions
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check Harness config and required local sensors",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoctorWithOptions(".", opts)
		},
	}
	cmd.Flags().BoolVar(&opts.Strict, "strict", false, "exit non-zero when Harness config, sensors, or generated agent references are incomplete")
	return cmd
}

func runDoctor(root string) error {
	return runDoctorWithOptions(root, doctorOptions{})
}

func runDoctorWithOptions(root string, opts doctorOptions) error {
	var audit doctorAudit
	project := detect.DetectProject(root)
	cfg, err := loadDoctorConfig(root, project.Stack)
	if err != nil {
		return err
	}
	fmt.Println("Harness doctor")
	fmt.Printf("  Project: %s\n", valueOr(project.Name, "unknown"))
	fmt.Printf("  Stack: %s\n", valueOr(project.Stack, "unknown"))
	if project.PackageManager != "" {
		fmt.Printf("  Package manager: %s\n", project.PackageManager)
	}
	if len(project.Frameworks) > 0 {
		fmt.Printf("  Frameworks: %s\n", joinList(project.Frameworks))
	}

	if errs := cfg.Validate(); len(errs) > 0 {
		fmt.Println()
		fmt.Println("Config errors:")
		for _, err := range errs {
			fmt.Println("  FAIL", err)
			audit.fail(err)
		}
		return finishDoctor(opts, audit)
	}

	reg := adapters.BuildRegistry()
	fmt.Println()
	fmt.Println("Active dimensions:")
	for _, dim := range cfg.ActiveDimensions() {
		fmt.Printf("  %s threshold=%d weight=%d\n", dim, cfg.ThresholdFor(dim), cfg.WeightFor(dim))
		if dim == config.DimContract {
			fmt.Println("    OK contract-validator built in")
			continue
		}
		names := cfg.AdapterNamesForDimension(dim)
		if len(names) == 0 {
			fmt.Println("    FAIL no sensors configured")
			audit.fail(fmt.Sprintf("%s has no sensors configured", dim))
			continue
		}
		registered := 0
		available := 0
		for _, name := range names {
			s, ok := reg.ByName(name)
			switch {
			case !ok:
				fmt.Printf("    FAIL %-18s not registered in this binary\n", name)
				audit.fail(fmt.Sprintf("%s sensor %q is configured but not registered", dim, name))
			case s.Available(root):
				fmt.Printf("    OK   %-18s available\n", name)
				registered++
				available++
			default:
				fmt.Printf("    MISS %-18s %s\n", name, installHint(name))
				registered++
				audit.warn(fmt.Sprintf("%s sensor %q is configured but unavailable", dim, name))
			}
		}
		if registered > 0 && available == 0 {
			audit.fail(fmt.Sprintf("%s is active but no registered configured sensor is available", dim))
		}
	}

	inspectHarnessCoverage(root, project, &audit)
	return finishDoctor(opts, audit)
}

func loadDoctorConfig(root, stack string) (config.Config, error) {
	path := filepath.Join(root, ".harness", "config.yaml")
	if _, err := os.Stat(path); err != nil {
		return config.DefaultFor(stack), nil
	}
	return config.Load(path)
}

func installHint(sensor string) string {
	switch sensor {
	case "eslint":
		return "install eslint and add an eslint.config.* file"
	case "jest", "jest-coverage":
		return "install jest and keep tests runnable with npx jest"
	case "vitest", "vitest-coverage":
		return "install vitest and @vitest/coverage-v8 when coverage is active"
	case "npm-audit":
		return "ensure npm is on PATH and package-lock.json exists"
	case "playwright":
		return "install @playwright/test and add playwright.config.*"
	case "js-complexity", "js-architecture":
		return "built-in static sensor should be available for Node/TS projects"
	case "approved-fixtures":
		return "add approved JSON fixtures under .harness/fixtures"
	default:
		return "install or disable this sensor in .harness/config.yaml"
	}
}

func inspectHarnessCoverage(root string, project detect.ProjectInfo, audit *doctorAudit) {
	fmt.Println()
	fmt.Println("Harness coverage:")
	harnessDir := filepath.Join(root, ".harness")
	if _, err := os.Stat(harnessDir); err != nil {
		fmt.Println("  FAIL .harness/ is missing; run harness setup")
		audit.fail(".harness/ is missing")
		return
	}

	if fileContains(filepath.Join(harnessDir, "agent-protocol.md"), "harness.repair", "sprint repair") {
		fmt.Println("  OK   agent protocol includes repair loop")
	} else {
		fmt.Println("  FAIL .harness/agent-protocol.md is missing or stale")
		audit.fail(".harness/agent-protocol.md is missing or does not include the repair loop")
	}

	if hasGeneratedIgnore(filepath.Join(harnessDir, ".gitignore")) {
		fmt.Println("  OK   generated local artifacts are ignored")
	} else {
		fmt.Println("  WARN .harness/.gitignore should ignore memory.db, reports/, repairs/, screenshots/")
		audit.warn(".harness/.gitignore does not ignore all generated local artifacts")
	}

	setup := readSetupState(filepath.Join(harnessDir, "setup.json"))
	if setup.PlanningMode == "" {
		setup.PlanningMode = planningModeFromInstalled(harnessDir)
	}
	if setup.ContractSkillsEnabled && setup.PlanningMode == PlanningManual {
		setup.PlanningMode = PlanningContract
	}
	if setup.PlanningMode == PlanningSpecDriven {
		checkSpecDrivenCoverage(root, harnessDir, audit)
	} else if setup.PlanningMode == PlanningContract {
		if skillsInstalled(harnessDir) {
			fmt.Println("  OK   contract automation skills installed")
		} else {
			fmt.Println("  FAIL contract skills enabled but .harness/skills is missing")
			audit.fail("contract skills are enabled but .harness/skills is missing")
		}
		authorSkill := filepath.Join(harnessDir, "skills", "contract-authoring", "SKILL.md")
		if fileContains(authorSkill, "sprint repair", "repairs/latest.md") {
			fmt.Println("  OK   contract-authoring skill includes repair loop")
		} else {
			fmt.Println("  FAIL contract-authoring skill is stale; run harness skills install --force")
			audit.fail("contract-authoring skill is stale; run harness skills install --force")
		}
	}

	expected := expectedReferences(setup.CodingCLI, project.CodingCLIs)
	if len(expected) == 0 {
		fmt.Println("  WARN no coding CLI reference detected; Harness will rely on manual use")
		audit.warn("no coding CLI reference detected")
		return
	}
	for _, cli := range expected {
		checkAgentReference(root, cli, setup.PlanningMode, audit)
	}
}

type setupState struct {
	CodingCLI             string `json:"coding_cli"`
	PlanningMode          string `json:"planning_mode"`
	ContractSkillsEnabled bool   `json:"contract_skills_enabled"`
}

func readSetupState(path string) setupState {
	var state setupState
	b, err := os.ReadFile(path)
	if err != nil {
		return state
	}
	_ = json.Unmarshal(b, &state)
	state.CodingCLI = normalizeCLI(state.CodingCLI)
	state.PlanningMode = normalizePlanningMode(state.PlanningMode)
	if state.PlanningMode == PlanningAuto {
		state.PlanningMode = ""
	}
	return state
}

func checkSpecDrivenCoverage(root, harnessDir string, audit *doctorAudit) {
	if specDrivenSkillsInstalled(harnessDir) {
		fmt.Println("  OK   spec-driven skill pack installed")
	} else {
		fmt.Println("  FAIL spec-driven planning enabled but .harness/skills/spec-driven is missing")
		audit.fail("spec-driven planning enabled but .harness/skills/spec-driven is missing")
	}
	specSkill := filepath.Join(harnessDir, "skills", "spec-driven", "SKILL.md")
	if fileContains(specSkill, "Specify", "Design", "Tasks", "Execute", "Validate", ".harness/contracts/sprint-NNN.md") {
		fmt.Println("  OK   spec-driven skill includes full planning loop")
	} else {
		fmt.Println("  FAIL spec-driven skill is stale; run harness skills install --planning spec-driven --force")
		audit.fail("spec-driven skill is stale; run harness skills install --planning spec-driven --force")
	}
	if fileContains(filepath.Join(harnessDir, "skills", "contract-review", "SKILL.md"), "No implementation starts until the contract status is AGREED") {
		fmt.Println("  OK   contract-review skill enforces agreement")
	} else {
		fmt.Println("  FAIL contract-review skill is stale; run harness skills install --planning spec-driven --force")
		audit.fail("contract-review skill is stale; run harness skills install --planning spec-driven --force")
	}
	for _, rel := range []string{
		".harness/context/STACK.md",
		".harness/context/ARCHITECTURE.md",
		".harness/context/CONVENTIONS.md",
		".harness/context/TESTING.md",
		".harness/context/INTEGRATIONS.md",
		".harness/context/CONCERNS.md",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			fmt.Printf("  WARN %s is missing\n", rel)
			audit.warn(rel + " is missing")
		}
	}
	if _, err := os.Stat(filepath.Join(harnessDir, "design")); err == nil {
		fmt.Println("  OK   design artifact directory exists")
	} else {
		fmt.Println("  FAIL .harness/design is missing")
		audit.fail(".harness/design is missing")
	}
	if _, err := os.Stat(filepath.Join(harnessDir, "tasks")); err == nil {
		fmt.Println("  OK   task artifact directory exists")
	} else {
		fmt.Println("  FAIL .harness/tasks is missing")
		audit.fail(".harness/tasks is missing")
	}
}

func expectedReferences(setupCLI string, detected []string) []string {
	switch setupCLI {
	case "codex", "claude", "cursor":
		return []string{setupCLI}
	case "all":
		return []string{"claude", "codex", "cursor"}
	case "none":
		return nil
	}
	return detected
}

func checkAgentReference(root, cli, planningMode string, audit *doctorAudit) {
	switch cli {
	case "codex":
		if fileContains(filepath.Join(root, "AGENTS.md"), "## Harness Gate", "harness.repair", "sprint repair") {
			fmt.Println("  OK   Codex AGENTS.md includes repair protocol")
		} else {
			fmt.Println("  FAIL Codex AGENTS.md is missing or stale")
			audit.fail("Codex AGENTS.md is missing or stale")
		}
		if fileContains(filepath.Join(root, ".codex", "hooks.json"), "guard pre-tool") {
			fmt.Println("  OK   Codex edit guard installed")
		} else {
			fmt.Println("  WARN Codex edit guard not found in .codex/hooks.json")
			audit.warn("Codex edit guard not found")
		}
		if planningMode == PlanningSpecDriven {
			if fileContains(filepath.Join(root, ".codex", "agents", "harness-spec-planner.toml"), "harness_spec_planner", "Specify", "Tasks") &&
				fileContains(filepath.Join(root, ".codex", "agents", "harness-task-worker.toml"), "harness_task_worker", "AGREED") {
				fmt.Println("  OK   Codex spec-driven agents installed")
			} else {
				fmt.Println("  FAIL Codex spec-driven agents are missing or stale")
				audit.fail("Codex spec-driven agents are missing or stale")
			}
		}
	case "claude":
		if fileContains(filepath.Join(root, "CLAUDE.md"), "## Harness Gate", "harness.repair", "sprint repair") {
			fmt.Println("  OK   Claude CLAUDE.md includes repair protocol")
		} else {
			fmt.Println("  FAIL Claude CLAUDE.md is missing or stale")
			audit.fail("Claude CLAUDE.md is missing or stale")
		}
		if fileContains(filepath.Join(root, ".claude", "settings.json"), "guard pre-tool") {
			fmt.Println("  OK   Claude edit guard installed")
		} else {
			fmt.Println("  WARN Claude edit guard not found in .claude/settings.json")
			audit.warn("Claude edit guard not found")
		}
		if planningMode == PlanningSpecDriven {
			if fileContains(filepath.Join(root, ".claude", "agents", "harness-spec-planner.md"), "harness-spec-planner", "Specify", "Tasks") &&
				fileContains(filepath.Join(root, ".claude", "agents", "harness-task-worker.md"), "harness-task-worker", "AGREED") {
				fmt.Println("  OK   Claude spec-driven agents installed")
			} else {
				fmt.Println("  FAIL Claude spec-driven agents are missing or stale")
				audit.fail("Claude spec-driven agents are missing or stale")
			}
		}
	case "cursor":
		needles := []string{"Harness Engineering", "sprint repair"}
		if planningMode == PlanningSpecDriven {
			needles = append(needles, "Spec-driven automation")
		}
		if fileContains(filepath.Join(root, ".cursor", "rules", "harness.mdc"), needles...) {
			fmt.Println("  OK   Cursor rule includes repair protocol")
		} else {
			fmt.Println("  FAIL Cursor rule is missing or stale")
			audit.fail("Cursor rule is missing or stale")
		}
	}
}

func fileContains(path string, needles ...string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	text := string(b)
	for _, needle := range needles {
		if !strings.Contains(text, needle) {
			return false
		}
	}
	return true
}

func hasGeneratedIgnore(path string) bool {
	return fileContains(path, "memory.db", "reports/", "repairs/", "screenshots/")
}

func (a *doctorAudit) fail(message string) {
	a.failures = append(a.failures, doctorFinding{Severity: "FAIL", Message: message})
}

func (a *doctorAudit) warn(message string) {
	a.warnings = append(a.warnings, doctorFinding{Severity: "WARN", Message: message})
}

func finishDoctor(opts doctorOptions, audit doctorAudit) error {
	fmt.Println()
	if len(audit.failures) == 0 {
		fmt.Printf("Doctor summary: PASS (%d warning(s))\n", len(audit.warnings))
		return nil
	}
	fmt.Printf("Doctor summary: FAIL (%d issue(s), %d warning(s))\n", len(audit.failures), len(audit.warnings))
	if opts.Strict {
		return fmt.Errorf("doctor strict failed: %d issue(s)", len(audit.failures))
	}
	return nil
}
