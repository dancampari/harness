package harness

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/dancampari/harness/internal/traceability"
	"github.com/spf13/cobra"
)

// newTraceabilityCmd surfaces the ledger persisted at
// .harness/traceability.json. Print it as a compact table by default,
// or as JSON for tooling. The ledger itself is owned by the agreement
// manager + sprint Consolidate — this command never mutates it.
func newTraceabilityCmd() *cobra.Command {
	var format string
	var slug string
	cmd := &cobra.Command{
		Use:   "traceability",
		Short: "Show the REQ-ID lifecycle ledger (.harness/traceability.json)",
		Long: `Reads .harness/traceability.json and prints every requirement with
its current TLC status (Pending → In Design → In Tasks → Implementing →
Verified). Pass --slug to filter by feature; pass --format=json for
machine-readable output.

The ledger is populated by ` + "`harness contract propose`" + ` (creates Pending
entries from the spec's ` + "`## Requirements`" + ` section), advanced to
Implementing on AGREED, and to Verified after ` + "`harness sprint score`" + ` /
` + "`harness feature score`" + ` succeeds.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ledger, err := traceability.Load(".harness")
			if err != nil {
				return err
			}
			entries := ledger.Entries
			if slug != "" {
				entries = ledger.ForSlug(slug)
			}
			switch format {
			case "json":
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(struct {
					Entries []traceability.Entry `json:"entries"`
				}{Entries: entries})
			default:
				printTraceabilityTable(entries)
				return nil
			}
		},
	}
	cmd.Flags().StringVar(&format, "format", autoFormat(), "output format: tty|json")
	cmd.Flags().StringVar(&slug, "slug", "", "filter by feature slug, e.g. sprint-001")
	return cmd
}

func printTraceabilityTable(entries []traceability.Entry) {
	if len(entries) == 0 {
		fmt.Println("No traceability entries yet — propose a contract with a `## Requirements` section to seed the ledger.")
		return
	}
	fmt.Printf("%-14s  %-9s  %-13s  %s\n", "Slug", "REQ", "Status", "Updated")
	fmt.Println("--------------  ---------  -------------  --------------------")
	for _, e := range entries {
		fmt.Printf("%-14s  %-9s  %-13s  %s\n", e.Slug, e.RequirementID, e.Status, e.UpdatedAt)
	}
}
