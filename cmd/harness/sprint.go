package harness

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dancampari/harness/internal/agreement"
	"github.com/dancampari/harness/internal/events"
	"github.com/dancampari/harness/internal/sprint"
	"github.com/spf13/cobra"
)

// newSprintCmd builds the legacy `harness sprint` surface. As of Phase 4
// of the unification plan, `harness feature` is the canonical
// TLC-aligned vocabulary; sprint stays as a deprecated alias and is
// scheduled for removal in v2.0.
//
// Every invocation prints a one-line deprecation warning on stderr and
// emits a `cli.deprecated` event so the trend tooling can surface the
// last project still leaning on the old surface.
func newSprintCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:        "sprint",
		Short:      "Deprecated alias of `harness feature` (Contract → Build → QA → Score)",
		Deprecated: "use `harness feature` instead; the `sprint` alias will be removed in v2.0",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			emitDeprecatedAlias("sprint", strings.Join(append([]string{cmd.Name()}, args...), " "))
			return nil
		},
	}
	cmd.AddCommand(
		newSprintNewCmd(),
		newSprintStatusCmd(),
		newSprintQACmd(),
		newSprintRepairCmd(),
		newSprintScoreCmd(),
		newSprintListCmd(),
	)
	return cmd
}

// emitDeprecatedAlias prints a one-line warning to stderr and records a
// `cli.deprecated` event so doctor + trend tooling can surface usage of
// the legacy `harness sprint` alias. Safe to call even when `.harness/`
// does not exist — the events package no-ops on a missing directory.
func emitDeprecatedAlias(alias, invocation string) {
	fmt.Fprintf(os.Stderr,
		"harness: `%s` is a deprecated alias — use `harness feature` instead (alias removed in v2.0).\n",
		alias)
	events.Record(".harness", "cli.deprecated", events.PhaseContract,
		fmt.Sprintf("alias=%s invocation=%s", alias, invocation), "")
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
			events.Record(".harness", "contract.created", events.PhaseContract,
				fmt.Sprintf("sprint %03d · %s", n, goal), "")
			fmt.Printf("✓ Created %s (sprint %03d)\n", path, n)
			fmt.Println("  Next: fill the contract, then run harness contract propose.")
			fmt.Println("  Implementation starts only after planner/tester agreement.")
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
	var acceptFixtures bool
	var allowUnagreed bool
	var fast bool
	cmd := &cobra.Command{
		Use:   "qa",
		Short: "Run the Evaluator (isolated subprocess) on the current sprint",
		Long: `Spawns the Evaluator in a separate process with a clean context.
It receives only the contract and the diff. It cannot see how the build
happened. This is the 'verificação real' from problem 5 of the video.

The --fast flag is the shift-left mode used by the pre-commit hook: it
filters out tests, coverage, audit, and browser sensors so feedback
returns in seconds. Dimensions without a fast sensor are reported as
SKIPPED, do not contribute to verdict or score, and do not overwrite
the full QA report on disk. A fast QA result is not consolidable by
harness sprint score; run full harness sprint qa after agreement before
scoring.

The --internal flag is reserved for the spawned subprocess itself. End
users never pass it; the parent process sets it when forking.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := sprint.NewManager(".harness")
			if err != nil {
				return err
			}
			opts := sprint.QAOptions{
				AcceptScreenshots: acceptScreenshots,
				AcceptFixtures:    acceptFixtures,
				Fast:              fast,
			}
			if internal {
				// We are the spawned subprocess. Do the work and emit
				// the EvaluationResult as JSON on stdout. Nothing else
				// may go to stdout (the parent depends on parseable JSON).
				return mgr.RunQAInternalWith(os.Stdout, opts)
			}
			// Fast mode is informational and used by pre-commit. The
			// agreement gate only protects the canonical full QA loop.
			if !allowUnagreed && !fast {
				if err := agreement.NewManager(".harness").EnsureAgreed(0); err != nil {
					return err
				}
			}
			// We are the parent. Spawn the subprocess and render.
			result, err := mgr.RunQAWith(opts)
			if err != nil {
				return err
			}
			events.Record(".harness", "qa.finished", events.PhaseQA, result.Verdict(), "")
			switch format {
			case "json":
				if err := result.WriteJSON(os.Stdout); err != nil {
					return err
				}
			default:
				if err := result.WriteTTY(os.Stdout); err != nil {
					return err
				}
				if !fast {
					openReportIfInteractive(result.EvaluationPath())
				}
			}
			// Fast mode is consumed by pre-commit, so FAIL must propagate
			// as a non-zero exit code. Fast runs intentionally do not write
			// scoreable reports; the hook only needs the signal.
			if fast && result.Verdict() == "FAIL" {
				return fmt.Errorf("fast QA returned FAIL; fix findings or commit with --no-verify to bypass")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", autoFormat(), "output format: tty|json")
	cmd.Flags().BoolVar(&acceptScreenshots, "accept-screenshots", false,
		"accept current Playwright screenshots as the visual baseline")
	cmd.Flags().BoolVar(&acceptFixtures, "accept-fixtures", false,
		"accept current approved-fixture command outputs after human review")
	cmd.Flags().BoolVar(&allowUnagreed, "allow-unagreed", false,
		"explicitly run QA before multi-agent contract agreement")
	cmd.Flags().BoolVar(&fast, "fast", false,
		"shift-left mode: run only fast static-analysis sensors; safe for pre-commit")
	cmd.Flags().BoolVar(&internal, "internal", false,
		"internal use only: act as the isolated evaluator subprocess")
	_ = cmd.Flags().MarkHidden("internal")
	return cmd
}

func newSprintScoreCmd() *cobra.Command {
	var allowFail bool
	cmd := &cobra.Command{
		Use:   "score",
		Short: "Consolidate a passing verdict and update STATE.md + memory.db",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := sprint.NewManager(".harness")
			if err != nil {
				return err
			}
			report, err := mgr.Consolidate(allowFail)
			if err != nil {
				return err
			}
			events.Record(".harness", "sprint.scored", events.PhaseReport,
				fmt.Sprintf("sprint %03d · %s · %d/100", report.SprintNumber, report.Verdict, report.Score.Total), "")
			fmt.Printf("✓ Sprint %03d scored: %d/100 (%s)\n",
				report.SprintNumber, report.Score.Total, report.Verdict)
			fmt.Printf("  Report: %s\n", report.Path)
			fmt.Printf("  Evaluation: %s\n", report.EvaluationPath)
			fmt.Printf("  State updated: %s\n", filepath.Join(".specs", "project", "STATE.md"))
			openReportIfInteractive(report.EvaluationPath)
			return nil
		},
	}
	cmd.Flags().BoolVar(&allowFail, "allow-fail", false,
		"record a failing sprint anyway; use only for explicit abandonment/audit")
	return cmd
}

func newSprintRepairCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "repair",
		Short: "Create and print the repair brief for the latest failing QA report",
		RunE: func(cmd *cobra.Command, args []string) error {
			mgr, err := sprint.NewManager(".harness")
			if err != nil {
				return err
			}
			brief, err := mgr.WriteRepairBrief()
			if err != nil {
				return err
			}
			if brief.LatestPath == "" {
				fmt.Printf("Sprint %03d has QA %s (%d/100). No repair required.\n",
					brief.SprintNumber, brief.Verdict, brief.TotalScore)
				return nil
			}
			b, err := os.ReadFile(brief.LatestPath)
			if err != nil {
				return err
			}
			fmt.Print(string(b))
			if len(b) == 0 || b[len(b)-1] != '\n' {
				fmt.Println()
			}
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
