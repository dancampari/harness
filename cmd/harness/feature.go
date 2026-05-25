package harness

import "github.com/spf13/cobra"

// newFeatureCmd builds the TLC-aligned `harness feature` command tree.
// In TLC's vocabulary the work-unit is a "feature"; the harness has
// historically called it a "sprint". Phase 4 of the unification plan
// makes "feature" the primary surface while keeping "sprint" + "contract"
// as backwards-compatible aliases for the existing CLI surface.
//
// Each subcommand here is a thin re-export of the existing sprint or
// contract command so behavior stays identical — only the spelling
// changes. When `harness sprint` is removed in v2.0, this file becomes
// the canonical wiring.
func newFeatureCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "feature",
		Aliases: []string{"feat"},
		Short:   "Manage features (TLC vocabulary; aliases harness sprint + harness contract)",
		Long: `TLC's spec-driven workflow speaks of features, not sprints.
` + "`harness feature`" + ` exposes that vocabulary while delegating to the same
implementation as ` + "`harness sprint`" + ` / ` + "`harness contract`" + `:

  harness feature new "<goal>"           # alias of harness sprint new
  harness feature status                 # alias of harness sprint status
  harness feature qa                     # alias of harness sprint qa
  harness feature repair                 # alias of harness sprint repair
  harness feature score                  # alias of harness sprint score
  harness feature list                   # alias of harness sprint list
  harness feature propose                # alias of harness contract propose
  harness feature approve --role <role>  # alias of harness contract approve
  harness feature reject  --role <role>  # alias of harness contract reject
`,
	}
	cmd.AddCommand(
		renameForFeature(newSprintNewCmd(), "Create a new feature spec at .specs/features/sprint-NNN/spec.md"),
		renameForFeature(newSprintStatusCmd(), "Show the current feature pipeline state"),
		renameForFeature(newSprintQACmd(), "Run QA on the current feature (isolated Evaluator subprocess)"),
		renameForFeature(newSprintRepairCmd(), "Print the latest repair brief for the current feature"),
		renameForFeature(newSprintScoreCmd(), "Consolidate the current feature after QA PASS"),
		renameForFeature(newSprintListCmd(), "List every completed feature with scores"),
		renameForFeature(newContractProposeCmd(), "Propose the feature spec hash for agreement"),
		renameForFeature(newContractApproveCmd(), "Approve the feature spec as planner or tester"),
		renameForFeature(newContractRejectCmd(), "Reject the feature spec with a specific reason"),
		newFeatureImplementCmd(),
	)
	return cmd
}

// renameForFeature retags a sprint/contract subcommand with the supplied
// short description so `harness feature --help` reads in TLC vocabulary
// even though the underlying RunE is shared.
func renameForFeature(cmd *cobra.Command, short string) *cobra.Command {
	cmd.Short = short
	return cmd
}
