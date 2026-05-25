// Package harness contains the CLI command tree for the harness binary.
package harness

import "github.com/spf13/cobra"

// Execute runs the root command. Called from main.
func Execute(version string) error {
	var setup setupOptions
	root := &cobra.Command{
		Use:           "harness",
		Short:         "Harness Engineering agent - stack-agnostic, deterministic, offline",
		Long:          longDescription,
		Version:       version,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSetup(setup, version)
		},
	}

	root.Flags().StringVar(&setup.CLI, "cli", "auto", "coding CLI to configure: auto|codex|claude|cursor|all|none")
	root.Flags().StringVar(&setup.Planning, "planning", "auto", "planning automation: auto|spec-driven|contract|manual")
	root.Flags().StringVar(&setup.Skills, "skills", "auto", "legacy alias for planning: auto|on|off")
	root.Flags().StringVar(&setup.Scope, "scope", "auto", "install scope: auto|project|global")
	root.Flags().BoolVar(&setup.Force, "force", false, "overwrite existing .harness/")
	root.Flags().BoolVarP(&setup.Yes, "yes", "y", false, "run setup with no prompts; installs all agent references if none are detected")
	root.Flags().BoolVar(&setup.StartTUI, "start", false, "launch the live TUI after setup")

	root.AddCommand(
		newSetupCmd(version),
		newUpgradeCmd(version),
		newInitCmd(),
		newSpecCmd(),
		newSprintCmd(),
		newFeatureCmd(),
		newContractCmd(),
		newQuickCmd(),
		newRoadmapCmd(),
		newStateCmd(),
		newSessionCmd(),
		newRunCmd(version),
		newUICmd(version),
		newProgressCmd(),
		newTrendCmd(),
		newExplainCmd(),
		newInstallHooksCmd(),
		newSkillsCmd(),
		newGuardCmd(),
		newDoctorCmd(),
		newWatchCmd(),
		newContextCmd(),
		newTraceabilityCmd(),
	)

	return root.Execute()
}

const longDescription = `Harness Engineering agent.

A stack-agnostic, deterministic, offline auditor for AI-assisted development.
Sits next to your coding CLI (Claude Code, Codex, Cursor) and validates each
sprint's contract against the actual diff using independent sensors:
linters, type checkers, tests, coverage, complexity, architecture, E2E.

Run ` + "`harness`" + ` with no subcommand for the one-command bootstrap. It detects
the project, initializes .harness/, asks which coding CLI and contract mode to
configure, installs references, prints sensor status, and tells you how to open
the live terminal dashboard.

Workflow:
  harness                         # one-command setup
  harness upgrade                 # refresh generated files, preserve memory/history
  harness --planning spec-driven  # setup with full spec-driven automation
  harness feature new "<goal>"     # creates .specs/features/sprint-NNN/spec.md
  harness contract propose        # proposes the contract hash for agreement
  harness contract approve --role planner
  harness contract approve --role tester
  # ... CLI implements only after the contract is AGREED ...
  harness feature qa               # runs Evaluator (isolated subprocess)
  harness feature repair           # prints the latest repair brief after FAIL
  harness feature score            # consolidates PASS + updates .specs/project/STATE.md
  harness doctor --fix            # repairs safe config drift such as missing adapter defaults
  harness doctor --strict         # checks active dimensions, sensors, and generated agent references
  harness run --resume            # live TUI of the whole pipeline

Use 'harness <command> --help' for details.`
