package harness

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func newSkillsCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "skills",
		Short: "Manage Harness agent skill documents",
	}
	installCmd := &cobra.Command{
		Use:   "install",
		Short: "Install automated contract-authoring skills into .harness/",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInstallSkillsWithOptions(".harness", force)
		},
	}
	installCmd.Flags().BoolVar(&force, "force", false, "refresh generated Harness skill documents if they already exist")
	cmd.AddCommand(installCmd)
	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show whether automated contract-authoring skills are installed",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if skillsInstalled(".harness") {
				fmt.Println("Contract automation skills: installed")
			} else {
				fmt.Println("Contract automation skills: not installed")
			}
			return nil
		},
	})
	return cmd
}

func runInstallSkills(root string) error {
	return runInstallSkillsWithOptions(root, false)
}

func refreshInstallSkills(root string) error {
	return runInstallSkillsWithOptions(root, true)
}

func runInstallSkillsWithOptions(root string, force bool) error {
	if root == "" {
		root = ".harness"
	}
	dirs := []string{
		filepath.Join(root, "skills", "contract-authoring", "references"),
		filepath.Join(root, "skills", "contract-review"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	files := map[string]string{
		filepath.Join(root, "skills", "contract-authoring", "SKILL.md"):                             contractAuthoringSkill,
		filepath.Join(root, "skills", "contract-authoring", "references", "sprint-planning.md"):     sprintPlanningReference,
		filepath.Join(root, "skills", "contract-authoring", "references", "contract-quality.md"):    contractQualityReference,
		filepath.Join(root, "skills", "contract-authoring", "references", "acceptance-examples.md"): acceptanceExamplesReference,
		filepath.Join(root, "skills", "contract-review", "SKILL.md"):                                contractReviewSkill,
	}
	for path, content := range files {
		if force {
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				return err
			}
		} else {
			if err := writeTemplate(path, content); err != nil {
				return err
			}
		}
	}
	if err := ensureAgentProtocolMode(root, true); err != nil {
		return err
	}
	fmt.Println("  OK contract automation skills installed: .harness/skills/contract-authoring, .harness/skills/contract-review")
	return nil
}

func skillsInstalled(root string) bool {
	_, err := os.Stat(filepath.Join(root, "skills", "contract-authoring", "SKILL.md"))
	return err == nil
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

func ensureAgentProtocolMode(root string, skillsEnabled bool) error {
	path := filepath.Join(root, "agent-protocol.md")
	content := agentProtocolTemplate(harnessInvocation(), skillsEnabled)
	existing, err := os.ReadFile(path)
	if err != nil {
		return os.WriteFile(path, []byte(content), 0o644)
	}
	text := string(existing)
	hasSkillRef := strings.Contains(text, ".harness/skills/contract-authoring/SKILL.md")
	isCurrent := agentProtocolIsCurrent(text)
	if skillsEnabled && hasSkillRef && isCurrent {
		return nil
	}
	if !skillsEnabled && !hasSkillRef && isCurrent {
		return nil
	}
	if strings.Contains(text, "## Harness Agent Protocol") {
		return os.WriteFile(path, []byte(content), 0o644)
	}
	if strings.Contains(text, "# Harness Agent Protocol") {
		return os.WriteFile(path, []byte(content), 0o644)
	}
	return os.WriteFile(path, []byte(strings.TrimSpace(text)+"\n\n"+contractAutomationProtocol(skillsEnabled)+"\n"), 0o644)
}

func agentProtocolIsCurrent(text string) bool {
	return strings.Contains(text, "harness.repair") &&
		strings.Contains(text, "sprint repair") &&
		strings.Contains(text, ".harness/repairs/latest.md") &&
		strings.Contains(text, "sprint score` only after QA")
}

const contractAuthoringSkill = `---
name: harness-contract-authoring
description: Use when an agent receives a user request in a Harness repository and must decompose it into small sprints, create or update .harness/contracts/sprint-NNN.md, or repair a weak contract before implementation.
---

# Harness Contract Authoring

Use this skill before implementing a user request.

## Workflow

1. Read .harness/spec.md, .harness/progress.md, and .harness/agent-protocol.md.
2. Run: harness sprint status.
3. Decide the smallest sprint that can produce visible, testable progress.
4. Run: harness sprint new "<small goal>" when a new contract is needed.
5. Fill the generated contract completely before implementation.
6. Keep the contract honest: do not remove important acceptance criteria to make QA pass.
7. Run: harness contract propose.
8. Approve the planner role only when the contract is complete: harness contract approve --role planner.
9. Ask the independent tester/reviewer role to review the exact hash. In Codex, use harness_contract_reviewer when available. In Claude Code, use harness-contract-reviewer when available.
10. If tester rejects the contract, repair only the contract, propose the new hash, and approve planner again.
11. Do not implement until harness contract status returns AGREED.
12. Never run harness sprint qa --allow-unagreed unless the user explicitly asks for an emergency override.
13. Implement only the current sprint after agreement.
14. Run: harness sprint qa --format=json after meaningful changes.
15. Read .harness/reports/latest.json. If verdict is FAIL, run: harness sprint repair.
16. Read .harness/repairs/latest.md, fix findings, and rerun QA.
17. Repeat repair -> QA until verdict is PASS.
18. Run: harness sprint score only after QA is PASS.

## Required Contract Properties

- Goal is one small outcome, not a whole product.
- Deliverables name concrete files, routes, commands, schemas, or exported symbols.
- Acceptance criteria are observable and testable by sensors or direct inspection.
- Constraints include architecture boundaries, forbidden imports, complexity limits, security rules, or visual requirements when relevant.
- Ambiguities are recorded as assumptions only when they do not change product intent.

## References

- Sprint sizing: references/sprint-planning.md
- Contract quality checklist: references/contract-quality.md
- Examples of weak and strong criteria: references/acceptance-examples.md
`

const contractReviewSkill = `---
name: harness-contract-review
description: Use when an agent must independently review a Harness sprint contract before implementation and either approve or reject the exact contract hash.
---

# Harness Contract Review

Use this skill before implementation, from a tester/reviewer role.

## Workflow

1. Read .harness/spec.md, .harness/progress.md, .harness/agent-protocol.md, and the current sprint contract.
2. Run: harness contract status.
3. If the contract is draft or changed, ask the author/planner to run: harness contract propose.
4. Review only the contract. Do not implement code.
5. Reject weak contracts with: harness contract reject --role tester --reason "<specific issue>".
6. Approve only if the contract is small, objective, testable, and aligned with .harness/spec.md:
   harness contract approve --role tester.
7. Confirm: harness contract status.

## Review Criteria

- The sprint is small enough for one implementation pass.
- Deliverables name concrete files, routes, commands, schemas, or symbols.
- Acceptance criteria are observable and include negative cases where relevant.
- Constraints cover architecture, security, visual, complexity, and coverage risks that matter for the sprint.
- The contract does not lower thresholds or remove risk to make QA easier.
- No implementation starts until the contract status is AGREED.
- If the contract is weak, reject it and require the author/planner to repair
  the contract before any product files are changed.
`

const sprintPlanningReference = `# Sprint Planning

Convert the user's prompt into the smallest useful sprint.

Prefer one sprint when the request is narrow. Split into multiple sprints only
when a single pass would mix unrelated concerns such as data model, UI, E2E,
and migration work.

Good sprint goals:

- "Add appointment conflict validation in the server action and database RPC"
- "Create the public service-selection booking step"
- "Add QA coverage for barber availability edge cases"

Bad sprint goals:

- "Build the whole SaaS"
- "Improve the app"
- "Make everything production ready"

When multiple sprints are needed, create the first sprint contract and leave a
short plan for the next ones in .harness/progress.md after scoring.
`

const contractQualityReference = `# Contract Quality Checklist

A Harness contract is viable only when another agent can implement and verify it
without guessing product intent.

Check before implementation:

- Goal fits in one sprint.
- Deliverables include concrete paths or symbols.
- Acceptance criteria use objective language.
- Criteria mention expected behavior, negative cases, and edge cases.
- UI work includes responsive and visual expectations.
- Backend work includes validation, persistence, permissions, and failure modes.
- Security-sensitive work includes server-side enforcement.
- Architecture constraints mention forbidden shortcuts when relevant.
- Thresholds are not lowered to make the sprint easy.

Reject and rewrite criteria like:

- "Works correctly"
- "Looks good"
- "Refactor code"
- "Handle errors"

Replace them with observable criteria that Harness, tests, or a reviewer can
verify.
`

const acceptanceExamplesReference = `# Acceptance Criteria Examples

Weak:

| # | Criterion | Threshold |
|---|-----------|-----------|
| 1 | Booking works | 8/10 |

Strong:

| # | Criterion | Threshold |
|---|-----------|-----------|
| 1 | Creating an appointment rejects overlapping intervals for the same barber on the server side | 10/10 |
| 2 | The public booking flow shows only available time slots for the selected service duration and date | 8/10 |
| 3 | Unit tests cover overlap, adjacent appointments, cancelled appointments, and timezone conversion | 8/10 |

Weak:

| # | Criterion | Threshold |
|---|-----------|-----------|
| 1 | UI looks premium | 8/10 |

Strong:

| # | Criterion | Threshold |
|---|-----------|-----------|
| 1 | Admin appointment list uses the product dark/copper palette, compact desktop spacing, and clear empty/loading/error states | 8/10 |
| 2 | Mobile public booking uses touch-sized time chips and avoids table layouts | 8/10 |
| 3 | Playwright covers service selection, date selection, slot selection, login gate, and success state | 8/10 |
`
