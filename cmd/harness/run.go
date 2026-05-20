package harness

import (
	"github.com/dancampari/harness/internal/tui"
	"github.com/spf13/cobra"
)

func newRunCmd(version string) *cobra.Command {
	var resume bool
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Launch the live TUI pipeline dashboard",
		Long: `Opens the live TUI showing the Harness pipeline:
  - Overview dashboard
  - Runs history
  - Latest report
  - Logs
  - Skills
  - Doctor

Use --resume to load existing state from .harness/.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTUI(resume, version)
		},
	}
	cmd.Flags().BoolVar(&resume, "resume", false, "resume from existing state")
	return cmd
}

func runTUI(resume bool, version string) error {
	return tui.Run(".harness", resume, version)
}

func newUICmd(version string) *cobra.Command {
	var resume bool
	cmd := &cobra.Command{
		Use:   "ui",
		Short: "Launch the Harness terminal UI",
		Long:  "Opens the Harness terminal UI. This is an alias for the live pipeline dashboard.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTUI(resume, version)
		},
	}
	cmd.Flags().BoolVar(&resume, "resume", true, "resume from existing state")
	return cmd
}
