package harness

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dancampari/harness/internal/skills"
	"github.com/spf13/cobra"
)

func newSkillsCmd() *cobra.Command {
	var force bool
	var planning string
	cmd := &cobra.Command{
		Use:   "skills",
		Short: "Manage Harness agent skill packs",
	}
	installCmd := &cobra.Command{
		Use:   "install",
		Short: "Install the vendored TLC spec-driven skill plus harness-gate into .harness/",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			mode := normalizePlanningMode(planning)
			if mode == PlanningAuto {
				mode = planningModeFromInstalled(".harness")
				if mode == PlanningManual {
					mode = PlanningSpecDriven
				}
			}
			return runInstallSkillsWithOptions(".harness", force, mode)
		},
	}
	installCmd.Flags().BoolVar(&force, "force", false, "refresh generated Harness skill documents if they already exist")
	installCmd.Flags().StringVar(&planning, "planning", "auto", "planning automation: auto|spec-driven|contract|manual")
	cmd.AddCommand(installCmd)
	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show whether the TLC + harness-gate skill packs are installed",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if skillsInstalled(".harness") {
				fmt.Println("Skill packs: installed (tlc-spec-driven, harness-gate)")
			} else {
				fmt.Println("Skill packs: not installed")
			}
			fmt.Println("Planning mode:", planningModeLabel(planningModeFromInstalled(".harness")))
			return nil
		},
	})
	return cmd
}

func runInstallSkills(root string) error {
	return runInstallSkillsWithOptions(root, false, PlanningSpecDriven)
}

func runInstallSkillsWithMode(root, planningMode string) error {
	return runInstallSkillsWithOptions(root, false, planningMode)
}

func refreshInstallSkills(root string) error {
	mode := planningModeFromInstalled(root)
	if mode == PlanningManual {
		mode = PlanningSpecDriven
	}
	return runInstallSkillsWithOptions(root, true, mode)
}

// runInstallSkillsWithOptions installs the canonical skill packs into
// .harness/skills/. The vendored TLC spec-driven content is extracted
// verbatim, plus the small harness-gate skill that documents the
// deterministic enforcement protocol layered on top of TLC.
//
// Legacy directories from earlier harness versions (spec-driven,
// contract-authoring, contract-review) are removed by skills.Install
// so a project never carries parallel methodology hierarchies.
//
// PlanningManual skips skill installation entirely and only refreshes
// the agent protocol to its manual-mode wording.
func runInstallSkillsWithOptions(root string, force bool, planningMode string) error {
	if root == "" {
		root = ".harness"
	}
	planningMode = normalizePlanningMode(planningMode)
	if planningMode == PlanningAuto {
		planningMode = PlanningSpecDriven
	}
	if planningMode == PlanningManual {
		if err := ensureAgentProtocolMode(root, PlanningManual); err != nil {
			return err
		}
		fmt.Println("  OK planning skills disabled")
		return nil
	}
	// force flag is honoured by Install — it always overwrites — so the
	// non-force path is only meaningful for ensuring the skill pack
	// exists. Either way we end up with the canonical layout.
	_ = force
	if err := skills.Install(root); err != nil {
		return err
	}
	// Phase 2 of the unification plan migrates these to .specs/. For
	// now keep the legacy spec-driven artifact directories so existing
	// projects continue to work; new commands will start writing under
	// .specs/ when Phase 2 lands.
	for _, dir := range []string{
		filepath.Join(root, "context"),
		filepath.Join(root, "design"),
		filepath.Join(root, "tasks"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	if err := ensureAgentProtocolMode(root, planningMode); err != nil {
		return err
	}
	fmt.Println("  OK skill packs installed: .harness/skills/tlc-spec-driven, .harness/skills/harness-gate")
	return nil
}

// skillsInstalled reports whether both canonical packs are present.
func skillsInstalled(root string) bool {
	return skills.Installed(root)
}

func normalizeSkillsMode(value string) string {
	v := strings.ToLower(strings.TrimSpace(value))
	switch v {
	case "", "detect":
		return "auto"
	case "true", "yes", "y", "sim", "s", "enable", "enabled", "1":
		return "on"
	case "false", "no", "n", "nao", "disable", "disabled", "0", "none":
		return "off"
	}
	return v
}

// ensureAgentProtocolMode writes (or refreshes) .harness/agent-protocol.md
// so the generated text references the canonical skill paths and matches
// the active planning mode.
func ensureAgentProtocolMode(root string, planningMode string) error {
	path := filepath.Join(root, "agent-protocol.md")
	planningMode = normalizePlanningMode(planningMode)
	if planningMode == PlanningAuto {
		planningMode = PlanningSpecDriven
	}
	content := agentProtocolTemplate(harnessInvocation(), planningMode)
	existing, err := os.ReadFile(path)
	if err != nil {
		return os.WriteFile(path, []byte(content), 0o644)
	}
	text := string(existing)
	hasSkillRef := strings.Contains(text, ".harness/skills/tlc-spec-driven/SKILL.md") ||
		strings.Contains(text, ".harness/skills/harness-gate/SKILL.md")
	isCurrent := agentProtocolIsCurrent(text)
	if planningUsesSkills(planningMode) && hasSkillRef && isCurrent && protocolHasPlanningMode(text, planningMode) {
		return nil
	}
	if !planningUsesSkills(planningMode) && !hasSkillRef && isCurrent {
		return nil
	}
	if strings.Contains(text, "## Harness Agent Protocol") {
		return os.WriteFile(path, []byte(content), 0o644)
	}
	if strings.Contains(text, "# Harness Agent Protocol") {
		return os.WriteFile(path, []byte(content), 0o644)
	}
	return os.WriteFile(path, []byte(strings.TrimSpace(text)+"\n\n"+planningAutomationProtocol(planningMode)+"\n"), 0o644)
}

func agentProtocolIsCurrent(text string) bool {
	return strings.Contains(text, "harness.repair") &&
		strings.Contains(text, "sprint repair") &&
		strings.Contains(text, ".harness/repairs/latest.md") &&
		strings.Contains(text, "sprint score` only after QA")
}

func protocolHasPlanningMode(text, planningMode string) bool {
	switch planningMode {
	case PlanningSpecDriven, PlanningContract:
		return strings.Contains(text, ".harness/skills/tlc-spec-driven/SKILL.md") ||
			strings.Contains(text, ".harness/skills/harness-gate/SKILL.md")
	case PlanningManual:
		return strings.Contains(text, "Planning automation is disabled") ||
			strings.Contains(text, "skills disabled")
	default:
		return true
	}
}
