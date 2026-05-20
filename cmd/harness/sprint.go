package harness

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dancampari/harness/internal/sprint"
	"github.com/spf13/cobra"
)

func newSprintCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sprint",
		Short: "Manage sprints (Contract → Build → QA → Score)",
	}
	cmd.AddCommand(
		newSprintNewCmd(),
		newSprintStatusCmd(),
		newSprintQACmd(),
		newSprintScoreCmd(),
		newSprintListCmd(),
	)
	return cmd
}

func newSprintNewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "new <goal>",
		Short: "Create a new sprint contract template",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			goal := args[0]
			for _, a := range args[1:] {
				goal += " " + a
			}
			mgr, err := sprint.NewManager(".harness")
			if err != nil {
				return err
			}
			path, n, err := mgr.NewContract(goal)
			if err != nil {
				return err
			}
			fmt.Printf("✓ Created %s (sprint %03d)\n", path, n)
			fmt.Println("  Next: edit the contract, then implement the feature.")
			fmt.Println("  When done: harness sprint qa")
			return nil
		},
	}
}

func newSprintStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show current sprint state (Contract/Build/QA/Score)",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := sprint.NewManager(".harness")
			if err != nil {
				return err
			}
			st, err := mgr.Status()
			if err != nil {
				return err
			}
			fmt.Printf("Sprint %03d  Contract=%s  Build=%s  QA=%s  Score=%s\n",
				st.Number, st.Contract, st.Build, st.QA, st.Score)
			return nil
		},
	}
}

func newSprintQACmd() *cobra.Command {
	var format string
	var internal bool
	var acceptScreenshots bool
	cmd := &cobra.Command{
		Use:   "qa",
		Short: "Run the Evaluator (isolated subprocess) on the current sprint",
		Long: `Spawns the Evaluator in a separate process with a clean context.
It receives only the contract and the diff. It cannot see how the build
happened. This is the 'verificação real' from problem 5 of the video.

The --internal flag is reserved for the spawned subprocess itself. End
users never pass it; the parent process sets it when forking.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := sprint.NewManager(".harness")
			if err != nil {
				return err
			}
			if internal {
				// We are the spawned subprocess. Do the work and emit
				// the EvaluationResult as JSON on stdout. Nothing else
				// may go to stdout (the parent depends on parseable JSON).
				return mgr.RunQAInternal(os.Stdout, acceptScreenshots)
			}
			// We are the parent. Spawn the subprocess and render.
			result, err := mgr.RunQA(acceptScreenshots)
			if err != nil {
				return err
			}
			switch format {
			case "json":
				return result.WriteJSON(os.Stdout)
			default:
				if err := result.WriteTTY(os.Stdout); err != nil {
					return err
				}
				openReportIfInteractive(result.EvaluationPath())
				return nil
			}
		},
	}
	cmd.Flags().StringVar(&format, "format", autoFormat(), "output format: tty|json")
	cmd.Flags().BoolVar(&acceptScreenshots, "accept-screenshots", false,
		"accept current Playwright screenshots as the visual baseline")
	cmd.Flags().BoolVar(&internal, "internal", false,
		"internal use only: act as the isolated evaluator subprocess")
	_ = cmd.Flags().MarkHidden("internal")
	return cmd
}

func newSprintScoreCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "score",
		Short: "Consolidate verdict and update progress.md + memory.db",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := sprint.NewManager(".harness")
			if err != nil {
				return err
			}
			report, err := mgr.Consolidate()
			if err != nil {
				return err
			}
			fmt.Printf("✓ Sprint %03d scored: %d/100 (%s)\n",
				report.SprintNumber, report.Score.Total, report.Verdict)
			fmt.Printf("  Report: %s\n", report.Path)
			fmt.Printf("  Evaluation: %s\n", report.EvaluationPath)
			fmt.Printf("  Progress updated: %s\n", filepath.Join(".harness", "progress.md"))
			openReportIfInteractive(report.EvaluationPath)
			return nil
		},
	}
}

func newSprintListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all sprints with their scores",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := sprint.NewManager(".harness")
			if err != nil {
				return err
			}
			items, err := mgr.List()
			if err != nil {
				return err
			}
			for _, it := range items {
				fmt.Printf("  %03d  %-40s  %3d/100  %s\n",
					it.Number, truncate(it.Goal, 40), it.Score, it.Verdict)
			}
			return nil
		},
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func autoFormat() string {
	// If stdout is a terminal, default to tty; else json.
	fi, err := os.Stdout.Stat()
	if err != nil {
		return "json"
	}
	if (fi.Mode() & os.ModeCharDevice) != 0 {
		return "tty"
	}
	return "json"
}
