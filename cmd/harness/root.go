// Package harness contains the CLI command tree for the harness binary.
package harness

import (
	"github.com/spf13/cobra"
)

// Execute runs the root command. Called from main.
func Execute(version string) error {
	root := &cobra.Command{
		Use:     "harness",
		Short:   "Harness Engineering agent — stack-agnostic, deterministic, offline",
		Long:    longDescription,
		Version: version,
	}

	root.AddCommand(
		newInitCmd(),
		newSpecCmd(),
		newSprintCmd(),
		newRunCmd(),
		newProgressCmd(),
		newTrendCmd(),
		newExplainCmd(),
		newInstallHooksCmd(),
		newDoctorCmd(),
	)

	return root.Execute()
}

const longDescription = `Harness Engineering agent.

A stack-agnostic, deterministic, offline auditor for AI-assisted development.
Sits next to your coding CLI (Claude Code, Codex, Cursor) and validates each
sprint's contract against the actual diff using independent sensors:
linters, type checkers, tests, coverage, complexity, architecture, E2E.

Reports only — never blocks. The CLI or human decides what to do with the
score. Maintains per-project memory in .harness/progress.md (narrative,
versionable) and .harness/memory.db (indexed history for trend analysis).

Workflow:
  harness init                  # one-time setup
  harness sprint new "<goal>"   # creates contracts/sprint-NNN.md template
  # ... CLI writes contract, then implements the feature ...
  harness sprint qa             # runs Evaluator (isolated subprocess)
  harness sprint score          # consolidates verdict + updates progress.md
  harness doctor                # checks active dimensions and sensor tooling
  harness run --resume          # live TUI of the whole pipeline

Use 'harness <command> --help' for details.`
