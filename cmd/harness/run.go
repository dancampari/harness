package harness

import (
	"github.com/dancampari/harness/internal/tui"
	"github.com/spf13/cobra"
)

func newRunCmd(version string) *cobra.Command {
	var resume bool
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Launch the live TUI pipeline (Sprints / Activity / Status bar)",
		Long: `Opens the live TUI showing the pipeline state:
  - Sprints table (Contract/Build/QA/Score per row)
  - Activity log (current step, recent findings)
  - Status bar (active sprint, average score, elapsed time)

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
