// Package harness contains the CLI command tree for the harness binary.
package harness

import "github.com/spf13/cobra"

// Execute runs the root command. Called from main.
func Execute(version string) error {
	var setup setupOptions
	root := &cobra.Command{
		Use:     "harness",
		Short:   "Harness Engineering agent - stack-agnostic, deterministic, offline",
		Long:    longDescription,
		Version: version,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSetup(setup)
		},
	}

	root.Flags().StringVar(&setup.CLI, "cli", "auto", "coding CLI to configure: auto|codex|claude|cursor|all|none")
	root.Flags().StringVar(&setup.Skills, "skills", "auto", "contract automation skills: auto|on|off")
	root.Flags().StringVar(&setup.Scope, "scope", "auto", "install scope: auto|project|global")
	root.Flags().BoolVar(&setup.Force, "force", false, "overwrite existing .harness/")
	root.Flags().BoolVarP(&setup.Yes, "yes", "y", false, "run setup with no prompts; installs all agent references if none are detected")
	root.Flags().BoolVar(&setup.StartTUI, "start", false, "launch the live TUI after setup")

	root.AddCommand(
		newSetupCmd(),
		newInitCmd(),
		newSpecCmd(),
		newSprintCmd(),
		newRunCmd(),
		newProgressCmd(),
		newTrendCmd(),
		newExplainCmd(),
		newInstallHooksCmd(),
		newSkillsCmd(),
		newDoctorCmd(),
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
  harness --skills on             # setup with automated contract-authoring skills
  harness sprint new "<goal>"     # creates contracts/sprint-NNN.md template
  # ... CLI writes contract, then implements the feature ...
  harness sprint qa               # runs Evaluator (isolated subprocess)
  harness sprint score            # consolidates verdict + updates progress.md
  harness doctor                  # checks active dimensions and sensor tooling
  harness run --resume            # live TUI of the whole pipeline

Use 'harness <command> --help' for details.`
