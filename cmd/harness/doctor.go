package harness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dancampari/harness/internal/adapters"
	"github.com/dancampari/harness/internal/budget"
	"github.com/dancampari/harness/internal/config"
	"github.com/dancampari/harness/internal/detect"
	"github.com/spf13/cobra"
)

type doctorOptions struct {
	Strict bool
	Fix    bool
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
	cmd.Flags().BoolVar(&opts.Fix, "fix", false, "repair safe Harness config drift such as missing stack defaults and adapter lists")
	return cmd
}

func runDoctor(root string) error {
	return runDoctorWithOptions(root, doctorOptions{})
}

func runDoctorWithOptions(root string, opts doctorOptions) error {
	var audit doctorAudit
	project := detect.DetectProject(root)
	fixes, err := applyDoctorFixes(root, project, opts)
	if err != nil {
		return err
	}
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
	if opts.Fix {
		fmt.Println()
		fmt.Println("Auto-fix:")
		if len(fixes) == 0 {
			fmt.Println("  OK no safe fixes needed")
		} else {
			for _, fix := range fixes {
				fmt.Printf("  OK %s\n", fix)
			}
		}
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
	if stackSupportsDefaultAdapters(project.Stack) && !hasStackAdapterConfig(cfg) {
		fmt.Println()
		fmt.Println("Config warnings:")
		fmt.Printf("  WARN detected %s project has no stack adapters configured; run harness doctor --fix\n", project.Stack)
		audit.warn(fmt.Sprintf("detected %s project has no stack adapters configured", project.Stack))
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

func applyDoctorFixes(root string, project detect.ProjectInfo, opts doctorOptions) ([]string, error) {
	if !opts.Fix {
		return nil, nil
	}
	harnessDir := filepath.Join(root, ".harness")
	if _, err := os.Stat(harnessDir); err != nil {
		return nil, fmt.Errorf("cannot apply doctor --fix: .harness/ is missing; run harness setup")
	}

	stack := valueOr(project.Stack, "unknown")
	defaults := config.DefaultFor(stack)
	cfgPath := filepath.Join(harnessDir, "config.yaml")
	cfg, err := config.Load(cfgPath)
	var fixes []string
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		cfg = defaults
		fixes = append(fixes, "created .harness/config.yaml from detected stack defaults")
	} else {
		fixes = append(fixes, applyConfigDefaultFixes(&cfg, defaults, stack)...)
	}

	if len(fixes) > 0 {
		if err := config.Save(cfgPath, cfg); err != nil {
			return nil, err
		}
	}
	if !hasGeneratedIgnore(filepath.Join(harnessDir, ".gitignore")) {
		if err := ensureHarnessGitignore(harnessDir); err != nil {
			return nil, err
		}
		fixes = append(fixes, "updated .harness/.gitignore for generated local artifacts")
	}
	return fixes, nil
}

func applyConfigDefaultFixes(cfg *config.Config, defaults config.Config, detectedStack string) []string {
	var fixes []string
	if cfg.Version == "" {
		cfg.Version = defaults.Version
		fixes = append(fixes, "set config schema version")
	}
	if detectedStack != "" && detectedStack != "unknown" && (cfg.Stack == "" || cfg.Stack == "unknown") {
		cfg.Stack = detectedStack
		fixes = append(fixes, fmt.Sprintf("set config stack to %s", detectedStack))
	}
	if isContractOnlyConfig(*cfg) && len(defaults.AllAdapterNames()) > 0 {
		cfg.Thresholds = defaults.Thresholds
		cfg.Weights = defaults.Weights
		cfg.E2E = defaults.E2E
		fixes = append(fixes, fmt.Sprintf("activated %s quality gate defaults", detectedStack))
	}
	if len(cfg.AllAdapterNames()) == 0 && len(defaults.AllAdapterNames()) > 0 {
		cfg.Adapters = defaults.Adapters
		fixes = append(fixes, fmt.Sprintf("configured %s default adapters", detectedStack))
		return fixes
	}
	for _, dim := range cfg.ActiveDimensions() {
		if dim == config.DimContract {
			continue
		}
		if len(cfg.AdapterNamesForDimension(dim)) > 0 {
			continue
		}
		names := defaults.AdapterNamesForDimension(dim)
		if len(names) == 0 {
			continue
		}
		setAdapterNamesForDimension(&cfg.Adapters, dim, names)
		fixes = append(fixes, fmt.Sprintf("configured default %s adapters", dim))
	}
	return fixes
}

func isContractOnlyConfig(cfg config.Config) bool {
	active := cfg.ActiveDimensions()
	return len(active) == 1 && active[0] == config.DimContract
}

func stackSupportsDefaultAdapters(stack string) bool {
	switch stack {
	case "node", "typescript", "python", "go", "rust":
		return true
	default:
		return false
	}
}

func hasStackAdapterConfig(cfg config.Config) bool {
	return len(cfg.Adapters.Lint)+
		len(cfg.Adapters.Test)+
		len(cfg.Adapters.Coverage)+
		len(cfg.Adapters.Security)+
		len(cfg.Adapters.Complexity)+
		len(cfg.Adapters.Architecture)+
		len(cfg.Adapters.E2E) > 0
}

func setAdapterNamesForDimension(adapters *config.AdaptersConfig, dim string, names []string) {
	switch dim {
	case config.DimCorrectness:
		for _, name := range names {
			if isLintAdapter(name) {
				adapters.Lint = append(adapters.Lint, name)
			} else {
				adapters.Test = append(adapters.Test, name)
			}
		}
	case config.DimCoverage:
		adapters.Coverage = names
	case config.DimComplexity:
		adapters.Complexity = names
	case config.DimSecurity:
		adapters.Security = names
	case config.DimArchitecture:
		adapters.Architecture = names
	case config.DimBehavior:
		adapters.Behavior = names
	case config.DimE2E:
		adapters.E2E = names
	}
}

func isLintAdapter(name string) bool {
	switch name {
	case "eslint", "ruff", "mypy", "go-vet", "staticcheck", "clippy":
		return true
	default:
		return false
	}
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
	case "ruff":
		return "install ruff in the project environment"
	case "mypy":
		return "install mypy in the project environment"
	case "pytest", "pytest-cov":
		return "install pytest and pytest-cov in the project environment"
	case "pip-audit":
		return "install pip-audit in the project environment"
	case "go-vet", "go-test", "go-test-coverage":
		return "ensure go is on PATH"
	case "staticcheck":
		return "install staticcheck"
	case "govulncheck":
		return "install govulncheck"
	case "clippy", "cargo-test":
		return "ensure rustup/cargo and clippy are installed"
	case "cargo-audit":
		return "install cargo-audit"
	case "semgrep":
		return "install semgrep and add .semgrep.yml, .semgrep.yaml, or .semgrep/"
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

	checkGitHooks(root)
	checkDriftWatch(root)
	checkContextBudget(harnessDir)
	if cfg, err := config.Load(filepath.Join(harnessDir, "config.yaml")); err == nil {
		checkInferentialReviewer(cfg, audit)
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

// checkGitHooks surfaces which Harness-managed git hooks are installed.
// The hooks are opt-in safety nets; their absence is never an error,
// only a warning, because Harness must work in repos without git or in
// environments where hook installation is blocked by policy.
func checkGitHooks(root string) {
	hooksDir := filepath.Join(root, ".git", "hooks")
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		// No git repo in this directory; nothing to surface.
		return
	}
	prePush := filepath.Join(hooksDir, "pre-push")
	if fileContains(prePush, "harness", "sprint qa") {
		fmt.Println("  OK   git pre-push reports QA")
	} else {
		fmt.Println("  WARN git pre-push not installed; run harness install-hooks")
	}
	preCommit := filepath.Join(hooksDir, "pre-commit")
	if fileContains(preCommit, "harness", "sprint qa --fast") {
		fmt.Println("  OK   git pre-commit runs fast QA")
	} else {
		fmt.Println("  WARN git pre-commit not installed; run harness install-hooks --pre-commit to enable shift-left")
	}
}

// checkInferentialReviewer surfaces whether the optional LLM-backed
// reviewer is configured. It is the only Harness adapter that may
// invoke a model, so making its state explicit prevents the user from
// being surprised by an unexpected subprocess.
func checkInferentialReviewer(cfg config.Config, audit *doctorAudit) {
	active := cfg.ThresholdFor(config.DimReview) > 0 && cfg.WeightFor(config.DimReview) > 0
	hasCommand := len(cfg.Review.Command) > 0
	if !active && !hasCommand {
		return // intentionally disabled; nothing to report
	}
	if active && !hasCommand {
		fmt.Println("  FAIL review dimension is active but review.command is empty in .harness/config.yaml")
		audit.fail("review dimension is active but review.command is empty in .harness/config.yaml")
		return
	}
	if !active && hasCommand {
		fmt.Println("  WARN review.command is configured but thresholds.review and weights.review are zero (dimension disabled)")
		return
	}
	fmt.Printf("  OK   inferential reviewer configured (%s)\n", cfg.Review.Command[0])
}

// checkContextBudget surfaces how much agent context the harness memory
// is consuming. The framework's value depends on the agent actually
// reading spec.md, progress.md, and context/*.md every session, so
// uncontrolled growth there directly hurts the working window. Doctor
// warns rather than fails because pruning context is a judgement call.
func checkContextBudget(harnessDir string) {
	snap, err := budget.Inspect(harnessDir, latestSprintNumber(harnessDir))
	if err != nil {
		return
	}
	if snap.OverBudget() {
		fmt.Printf("  WARN context bundle is ~%d tokens (soft limit %d); prune progress.md or context/*.md\n",
			snap.TokenEstimate, snap.SoftLimitTokens)
		return
	}
	fmt.Printf("  OK   context bundle ~%d tokens (limit %d)\n", snap.TokenEstimate, snap.SoftLimitTokens)
}

// checkDriftWatch reports whether `harness watch` has run recently.
// The framework's mission is long-running autonomous development
// ("apps inteiras"), and drift between sprints is what kills quality
// in that mode. Doctor nudges the user toward running watch even when
// the rest of the sprint cycle is healthy.
func checkDriftWatch(root string) {
	latest := filepath.Join(root, ".harness", "watch", "latest.json")
	if _, err := os.Stat(latest); err != nil {
		fmt.Println("  WARN drift watch never ran; run harness watch once or schedule docs/templates/harness-watch.yml.example")
		return
	}
	fmt.Println("  OK   drift watch report present at .harness/watch/latest.json")
}

type setupState struct {
	CodingCLI             string `json:"coding_cli"`
	PlanningMode          string `json:"planning_mode"`
	ContractSkillsEnabled bool   `json:"contract_skills_enabled"`
	InstallScope          string `json:"install_scope"`
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
	return fileContains(path, "memory.db", "reports/", "repairs/", "screenshots/", "tmp/")
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
