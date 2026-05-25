package harness

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/dancampari/harness/internal/events"
	"github.com/spf13/cobra"
)

// newQuickCmd implements TLC's Quick mode: ad-hoc tasks (bug fixes, config
// tweaks, ≤3 files) that bypass the planner/tester agreement gate and run
// the fast QA path. Quick records the work as a TASK.md under
// .specs/quick/NNN-slug/ so the audit trail survives even when the gate
// is skipped.
//
// Quick is intentionally a one-liner UX:
//
//	harness quick "fix navbar overflow on mobile"
//
// It writes the TASK.md, emits a `quick.created` event, and prints the
// next-step pointer. It does NOT spawn QA — the caller decides whether to
// follow up with `harness sprint qa --fast --allow-unagreed`.
func newQuickCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "quick \"<one-line description>\"",
		Short: "TLC Quick mode: ad-hoc task, bypasses agreement, ≤3 files",
		Long: `Records an ad-hoc task at .specs/quick/NNN-slug/TASK.md.

Use Quick for bug fixes, config changes, doc tweaks, and other work that
is too small to deserve a full Specify -> Design -> Tasks -> Execute pass.
Quick:
  - assigns the next quick-task number,
  - generates a slug from the description,
  - writes a TASK.md with the description, timestamp, and a SUMMARY.md
    placeholder for the agent to fill after the work is done,
  - emits a quick.created event so the live panel and trend tooling see it.

Quick bypasses the planner/tester agreement gate by design. Audit comes
from the TASK.md + SUMMARY.md pair, not from the deterministic contract.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			description := strings.TrimSpace(strings.Join(args, " "))
			if description == "" {
				return fmt.Errorf("quick task requires a description")
			}
			specsRoot := siblingSpecsRoot(".harness")
			number, err := nextQuickTaskNumber(specsRoot)
			if err != nil {
				return err
			}
			slug := slugify(description)
			dir := filepath.Join(specsRoot, "quick", fmt.Sprintf("%03d-%s", number, slug))
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
			taskPath := filepath.Join(dir, "TASK.md")
			if err := os.WriteFile(taskPath, []byte(renderQuickTask(number, description)), 0o644); err != nil {
				return err
			}
			summaryPath := filepath.Join(dir, "SUMMARY.md")
			if err := os.WriteFile(summaryPath, []byte(renderQuickSummary(number, description)), 0o644); err != nil {
				return err
			}
			events.Record(".harness", "quick.created", events.PhaseContract,
				fmt.Sprintf("quick %03d · %s", number, description), "")
			fmt.Printf("✓ Quick task %03d created at %s\n", number, taskPath)
			fmt.Println("  Implement the change, fill SUMMARY.md, and run `harness sprint qa --fast --allow-unagreed` if QA is appropriate.")
			return nil
		},
	}
}

var quickDirRe = regexp.MustCompile(`^(\d+)-`)

func nextQuickTaskNumber(specsRoot string) (int, error) {
	dir := filepath.Join(specsRoot, "quick")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 1, nil
	}
	max := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		m := quickDirRe.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		var n int
		for _, r := range m[1] {
			n = n*10 + int(r-'0')
		}
		if n > max {
			max = n
		}
	}
	return max + 1, nil
}

var slugInvalid = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(input string) string {
	s := strings.ToLower(input)
	s = slugInvalid.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "task"
	}
	if len(s) > 48 {
		s = strings.Trim(s[:48], "-")
	}
	return s
}

func renderQuickTask(number int, description string) string {
	return fmt.Sprintf(`# Quick Task %03d

%s

- Created: %s
- Constraint: ≤3 files touched. If scope grows, escalate to a feature spec.
- Audit: fill SUMMARY.md after the change so the trend tooling has a record.
`, number, description, time.Now().UTC().Format(time.RFC3339))
}

func renderQuickSummary(number int, description string) string {
	return fmt.Sprintf(`# Quick Task %03d Summary

> %s

- Files touched:
  - <path/to/file>
- Outcome: <what changed, in 1-2 sentences>
- Risk: <what could break, in 1 sentence>
- Follow-up: <none | escalate to feature spec | doc update>
`, number, description)
}
