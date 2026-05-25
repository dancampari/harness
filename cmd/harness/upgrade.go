package harness

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/dancampari/harness/internal/config"
	"github.com/dancampari/harness/internal/detect"
	"github.com/dancampari/harness/internal/memory"
	"github.com/spf13/cobra"
)

type upgradeOptions struct {
	CLI      string
	Planning string
	Scope    string
	Yes      bool
	StartTUI bool
}

var lookPath = exec.LookPath

func newUpgradeCmd(version string) *cobra.Command {
	var opts upgradeOptions
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Refresh Harness generated files while preserving project memory",
		Long: `Refreshes the project Harness installation in one command.

Upgrade preserves project-owned state:
  - .harness/memory.db
  - .harness/runs/, reports/, evaluations/
  - .specs/ (project memory + feature trees, after migration)

It migrates legacy projects to the canonical TLC layout:
  - .harness/spec.md         → .specs/project/PROJECT.md
  - .harness/progress.md     → .specs/project/STATE.md
  - .harness/context/*.md    → .specs/codebase/*.md
  - .harness/contracts/<s>.md → .specs/features/<s>/spec.md
  - .harness/design/<s>.md   → .specs/features/<s>/design.md
  - .harness/tasks/<s>.md    → .specs/features/<s>/tasks.md

It refreshes generated Harness files:
  - .harness/bin/harness
  - .harness/skills/
  - .harness/agent-protocol.md
  - Codex/Claude/Cursor references and hooks
  - safe config defaults via doctor --fix`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpgrade(opts, version)
		},
	}
	cmd.Flags().StringVar(&opts.CLI, "cli", "auto", "coding CLI references to refresh: auto|codex|claude|cursor|all|none")
	cmd.Flags().StringVar(&opts.Planning, "planning", "auto", "planning automation: auto|spec-driven|contract|manual")
	cmd.Flags().StringVar(&opts.Scope, "scope", "auto", "install scope: auto|project|global")
	cmd.Flags().BoolVarP(&opts.Yes, "yes", "y", false, "run upgrade with no prompts")
	cmd.Flags().BoolVar(&opts.StartTUI, "start", false, "launch the live TUI after upgrade")
	return cmd
}

func runUpgrade(opts upgradeOptions, version string) error {
	if _, err := os.Stat(".harness"); err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		fmt.Println("Harness upgrade")
		fmt.Println("  .harness/ not found; running first-time setup.")
		return runSetup(setupOptions{
			CLI:      valueOr(opts.CLI, "auto"),
			Planning: valueOr(opts.Planning, "auto"),
			Scope:    valueOr(opts.Scope, "project"),
			Yes:      true,
			StartTUI: opts.StartTUI,
		}, version)
	}

	project := detect.DetectProject(".")
	state := readSetupState(filepath.Join(".harness", "setup.json"))
	choices, err := resolveUpgradeChoices(opts, project, state)
	if err != nil {
		return err
	}

	fmt.Println("Harness upgrade")
	fmt.Println("  Mode: refresh generated Harness files; preserve memory, contracts, reports, and progress.")
	fmt.Println()

	if err := ensureHarnessSkeleton(".harness", choices.Planning); err != nil {
		return err
	}
	migrated, err := migrateLegacyArtifactsToSpecs(".harness")
	if err != nil {
		return fmt.Errorf("migrate legacy artifacts to .specs/: %w", err)
	}
	for _, line := range migrated {
		fmt.Printf("  %s\n", line)
	}
	if err := ensurePersistentHarnessFiles(".harness", project, choices.Planning); err != nil {
		return err
	}
	if err := installProjectCommand(); err != nil {
		return err
	}
	fixes, err := applyDoctorFixes(".", project, doctorOptions{Fix: true})
	if err != nil {
		return err
	}
	for _, fix := range fixes {
		fmt.Printf("  OK %s\n", fix)
	}
	if err := runInstallHooks(installHookOptions{
		CLI:         choices.CLI,
		Skills:      boolSkillsModeForPlanning(choices.Planning),
		Planning:    choices.Planning,
		Interactive: false,
		InstallGit:  true,
	}); err != nil {
		return err
	}
	if err := writeSetupState(choices, project); err != nil {
		return err
	}
	if choices.Scope == "global" {
		if err := installGlobalCommand(); err != nil {
			return err
		}
	}
	if err := runDoctor("."); err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("Upgrade complete.")
	fmt.Printf("  Preserved:       .harness/memory.db, runs/, reports/, evaluations/, .specs/\n")
	fmt.Printf("  Migrated:        legacy .harness/{contracts,design,tasks,context,spec.md,progress.md} → .specs/\n")
	fmt.Printf("  Refreshed:       project command, skills, agent protocol, hooks, safe config defaults\n")
	fmt.Printf("  CLI references:  %s\n", choices.CLI)
	fmt.Printf("  Planning mode:   %s\n", planningModeLabel(choices.Planning))
	fmt.Printf("  Install scope:   %s\n", choices.Scope)
	fmt.Printf("  Run Harness:     %s run --resume\n", harnessInvocation())

	if opts.StartTUI {
		return runTUI(true, version, true)
	}
	return nil
}

func resolveUpgradeChoices(opts upgradeOptions, project detect.ProjectInfo, state setupState) (setupChoices, error) {
	planning, err := resolveUpgradePlanning(opts.Planning, state)
	if err != nil {
		return setupChoices{}, err
	}
	cli, err := resolveUpgradeCLI(opts.CLI, state, project)
	if err != nil {
		return setupChoices{}, err
	}
	scope, err := resolveUpgradeScope(opts.Scope, state)
	if err != nil {
		return setupChoices{}, err
	}
	return setupChoices{
		CLI:      cli,
		Planning: planning,
		Skills:   planningUsesSkills(planning),
		Scope:    scope,
	}, nil
}

func resolveUpgradePlanning(value string, state setupState) (string, error) {
	mode := normalizePlanningMode(value)
	switch mode {
	case PlanningSpecDriven, PlanningContract, PlanningManual:
		return mode, nil
	case PlanningAuto:
		if state.PlanningMode != "" {
			return state.PlanningMode, nil
		}
		if installed := planningModeFromInstalled(".harness"); installed != PlanningManual {
			return installed, nil
		}
		return PlanningSpecDriven, nil
	default:
		return "", fmt.Errorf("unknown planning mode %q; use auto|spec-driven|contract|manual", value)
	}
}

func resolveUpgradeCLI(value string, state setupState, project detect.ProjectInfo) (string, error) {
	cli := normalizeCLI(value)
	switch cli {
	case "codex", "claude", "cursor", "all", "none":
		return cli, nil
	case "auto":
		if state.CodingCLI != "" && state.CodingCLI != "auto" {
			return state.CodingCLI, nil
		}
		if len(project.CodingCLIs) > 0 {
			return "auto", nil
		}
		return "all", nil
	default:
		return "", fmt.Errorf("unknown coding CLI %q; use auto|codex|claude|cursor|all|none", value)
	}
}

func resolveUpgradeScope(value string, state setupState) (string, error) {
	scope := normalizeScope(value)
	switch scope {
	case "project", "global":
		return scope, nil
	case "auto":
		if state.InstallScope == "global" {
			return "global", nil
		}
		if globalHarnessOnPath() {
			return "global", nil
		}
		if state.InstallScope == "project" {
			return "project", nil
		}
		return "project", nil
	default:
		return "", fmt.Errorf("unknown install scope %q; use auto|project|global", value)
	}
}

func globalHarnessOnPath() bool {
	path, err := lookPath("harness")
	if err != nil {
		return false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return true
	}
	projectHarness, projectErr := filepath.Abs(filepath.Join(".harness", "bin", executableName("harness")))
	if projectErr == nil && samePath(abs, projectHarness) {
		return false
	}
	return true
}

func ensurePersistentHarnessFiles(root string, project detect.ProjectInfo, planningMode string) error {
	cfgPath := filepath.Join(root, "config.yaml")
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		if err := config.Save(cfgPath, config.DefaultFor(project.Stack)); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
	}
	if err := ensureProjectMemoryFiles(root); err != nil {
		return err
	}
	if err := ensureAgentProtocolMode(root, planningMode); err != nil {
		return err
	}
	db, err := memory.Open(filepath.Join(root, "memory.db"))
	if err != nil {
		return fmt.Errorf("open memory: %w", err)
	}
	defer db.Close()
	if err := db.Migrate(); err != nil {
		return fmt.Errorf("migrate memory: %w", err)
	}
	return ensureHarnessGitignore(root)
}
