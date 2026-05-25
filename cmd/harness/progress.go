package harness

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dancampari/harness/internal/memory"
	"github.com/dancampari/harness/internal/reporter"
	"github.com/spf13/cobra"
)

func newProgressCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "progress",
		Short: "Print .specs/project/STATE.md (the narrative brain)",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := filepath.Join(".specs", "project", "STATE.md")
			if _, err := os.Stat(path); err != nil {
				legacy := filepath.Join(".harness", "progress.md")
				if _, legacyErr := os.Stat(legacy); legacyErr == nil {
					path = legacy
				}
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			fmt.Print(string(b))
			return nil
		},
	}
}

func newTrendCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "trend",
		Short: "Show score trend over time (sparkline + table)",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := memory.Open(filepath.Join(".harness", "memory.db"))
			if err != nil {
				return err
			}
			defer db.Close()
			runs, err := db.RecentRuns(30)
			if err != nil {
				return err
			}
			return reporter.PrintTrend(os.Stdout, runs)
		},
	}
}

func newExplainCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "explain <finding-id>",
		Short: "Explain a specific finding in detail",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := memory.Open(filepath.Join(".harness", "memory.db"))
			if err != nil {
				return err
			}
			defer db.Close()
			f, err := db.FindingByID(args[0])
			if err != nil {
				return err
			}
			return reporter.PrintFinding(os.Stdout, f)
		},
	}
}
