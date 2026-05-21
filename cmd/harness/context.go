package harness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dancampari/harness/internal/budget"
	"github.com/spf13/cobra"
)

func newContextCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Inspect and manage the harness context bundle agents must load",
	}
	cmd.AddCommand(newContextSizeCmd())
	return cmd
}

func newContextSizeCmd() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "size",
		Short: "Estimate the agent context cost of harness memory files",
		Long: `Sums the byte size of the files an agent reads at session start:

  - .harness/spec.md
  - .harness/progress.md
  - .harness/agent-protocol.md
  - .harness/context/*.md
  - .harness/contracts/sprint-NNN.md for the latest sprint
  - .harness/design/sprint-NNN.md and .harness/tasks/sprint-NNN.md when present

Converts the total to an approximate token count using a constant
heuristic of 4 bytes per token. Long-running projects often drift past
the soft limit; pruning progress.md and context/ entries restores
working window for the agent.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			harnessDir := filepath.Join(cwd, ".harness")
			if _, err := os.Stat(harnessDir); err != nil {
				return fmt.Errorf("no .harness directory in %s; run harness init first", cwd)
			}
			sprintNumber := latestSprintNumber(harnessDir)
			snap, err := budget.Inspect(harnessDir, sprintNumber)
			if err != nil {
				return err
			}
			switch format {
			case "json":
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(snap)
			default:
				printContextSize(snap, sprintNumber)
				return nil
			}
		},
	}
	cmd.Flags().StringVar(&format, "format", autoFormat(), "output format: tty|json")
	return cmd
}

func printContextSize(s *budget.Snapshot, sprintNumber int) {
	fmt.Printf("Harness context bundle\n")
	if sprintNumber > 0 {
		fmt.Printf("  Sprint:        %03d\n", sprintNumber)
	}
	fmt.Printf("  Files:         %d\n", len(s.Files))
	fmt.Printf("  Total bytes:   %d\n", s.TotalBytes)
	fmt.Printf("  Token est.:    ~%d (heuristic %.2f tokens/byte)\n", s.TokenEstimate, budget.TokensPerByte)
	fmt.Printf("  Soft limit:    %d tokens\n", s.SoftLimitTokens)
	if s.OverBudget() {
		fmt.Printf("  STATUS:        OVER BUDGET — consider pruning progress.md or context/*.md\n")
	} else {
		fmt.Printf("  STATUS:        within budget\n")
	}
	if len(s.Files) > 0 {
		fmt.Println()
		fmt.Println("  Breakdown:")
		for _, f := range s.Files {
			fmt.Printf("    %8d  %s\n", f.Bytes, f.Path)
		}
	}
}

var sprintFileRe = regexp.MustCompile(`^sprint-(\d+)\.md$`)

// latestSprintNumber finds the highest sprint number with a contract on
// disk. Returns 0 when there is no sprint yet.
func latestSprintNumber(harnessDir string) int {
	dir := filepath.Join(harnessDir, "contracts")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	max := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := sprintFileRe.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		var n int
		fmt.Sscanf(m[1], "%d", &n)
		if n > max {
			max = n
		}
	}
	return max
}

// suppress unused import warning in case we move helpers out later.
var _ = strings.TrimSpace
