package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dancampari/harness/internal/watch"
	"github.com/spf13/cobra"
)

func newWatchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Run a drift-watch pass outside the sprint lifecycle",
		Long: `Runs a periodic drift monitor that complements harness sprint qa.

Watch is intentionally narrow:
  - it does not require a sprint contract;
  - it does not modify .harness/reports/ or memory.db (those are
    sprint-owned);
  - it stores reports under .harness/watch/<timestamp>.json and a
    .harness/watch/latest.json pointer;
  - it surfaces regressions versus the previous run so a cron / GitHub
    Actions schedule can fail when new drift appears between sprints.

The sensor set is fast static analysis plus configured audit adapters
(npm-audit, pip-audit, govulncheck, cargo-audit). Tests and e2e are
intentionally excluded.`,
	}
	cmd.AddCommand(newWatchOnceCmd(), newWatchListCmd())
	return cmd
}

func newWatchOnceCmd() *cobra.Command {
	var failOnRegression bool
	var format string
	cmd := &cobra.Command{
		Use:   "once",
		Short: "Run one drift-watch pass and write a report under .harness/watch/",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			harnessDir := filepath.Join(cwd, ".harness")
			if _, err := os.Stat(harnessDir); err != nil {
				return fmt.Errorf("no .harness directory in %s; run harness init first", cwd)
			}
			result, err := watch.RunOnce(context.Background(), cwd, harnessDir)
			if err != nil {
				return err
			}
			switch format {
			case "json":
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				if err := enc.Encode(result.Report); err != nil {
					return err
				}
			default:
				printWatchReport(result)
			}
			if failOnRegression && result.IsRegression {
				return fmt.Errorf("drift watch regression: %d new finding(s) versus previous run", result.Report.Delta.Regressed)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&failOnRegression, "fail-on-regression", false,
		"exit non-zero when finding count grew versus the previous watch run (useful in CI)")
	cmd.Flags().StringVar(&format, "format", autoFormat(), "output format: tty|json")
	return cmd
}

func newWatchListCmd() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List previous drift-watch reports",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			files, err := watch.List(filepath.Join(cwd, ".harness"), limit)
			if err != nil {
				return err
			}
			if len(files) == 0 {
				fmt.Println("No watch reports yet. Run: harness watch once")
				return nil
			}
			for _, f := range files {
				fmt.Println(f)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 10, "maximum number of reports to list (0 = all)")
	return cmd
}

func printWatchReport(r *watch.Result) {
	fmt.Printf("Watch report  %s\n", r.Report.Timestamp.Format("2006-01-02 15:04:05 UTC"))
	fmt.Printf("  Findings:   %d\n", r.Report.Findings)
	if r.Report.Delta != nil {
		fmt.Printf("  Previous:   %d  (delta %+d)\n",
			r.Report.Delta.FindingsBefore,
			r.Report.Delta.FindingsAfter-r.Report.Delta.FindingsBefore)
		if r.Report.Delta.Regressed > 0 {
			fmt.Printf("  REGRESSION: %d new finding(s) since last run\n", r.Report.Delta.Regressed)
		}
		for dim, change := range r.Report.Delta.DimensionDeltas {
			fmt.Printf("    %s: %+d\n", dim, change)
		}
	}
	fmt.Printf("  Report:     %s\n", r.ReportPath)
	fmt.Printf("  Latest:     %s\n", r.LatestPath)
}
