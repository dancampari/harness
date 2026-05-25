package harness

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dancampari/harness/internal/config"
	"github.com/dancampari/harness/internal/detect"
	"github.com/dancampari/harness/internal/memory"
	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	var force bool
	var cli string
	var installHooks bool
	var skills string
	var planning string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize .harness/ in the current repo",
		Long: `Creates the .harness/ runtime directory and .specs/ project memory with:
  - .harness/config.yaml (auto-detected for your stack)
  - .specs/project/PROJECT.md (template you should fill in)
  - .specs/project/STATE.md (the narrative brain of the project)
  - contracts/, evaluations/, repairs/, screenshots/, reports/ (empty runtime dirs)
  - memory.db (SQLite index, initialized)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			planningMode := normalizePlanningMode(planning)
			if planningMode == PlanningAuto {
				fromSkills, err := planningModeFromSkills(skills)
				if err != nil {
					return err
				}
				planningMode = fromSkills
			}
			if planningMode == PlanningAuto {
				planningMode = PlanningManual
			}
			if planningMode != PlanningSpecDriven && planningMode != PlanningContract && planningMode != PlanningManual {
				return fmt.Errorf("unknown planning mode %q; use auto|spec-driven|contract|manual", planning)
			}
			return runInit(initOptions{
				Force:         force,
				CLI:           cli,
				InstallHooks:  installHooks,
				InstallSkills: planningUsesSkills(planningMode),
				PlanningMode:  planningMode,
			})
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing .harness/")
	cmd.Flags().StringVar(&cli, "cli", "auto", "coding CLI to configure: auto|codex|claude|cursor|all|none")
	cmd.Flags().StringVar(&planning, "planning", "auto", "planning automation: auto|spec-driven|contract|manual")
	cmd.Flags().StringVar(&skills, "skills", "off", "legacy alias for planning: on|off")
	cmd.Flags().BoolVar(&installHooks, "install-hooks", false, "install coding CLI references during init")
	return cmd
}

type initOptions struct {
	Force         bool
	CLI           string
	InstallHooks  bool
	InstallSkills bool
	PlanningMode  string
	Quiet         bool
}

func runInit(opts initOptions) error {
	root := ".harness"
	if _, err := os.Stat(root); err == nil && !opts.Force {
		return errors.New(".harness/ already exists - use --force to overwrite")
	}

	if err := ensureHarnessSkeleton(root, opts.PlanningMode); err != nil {
		return err
	}

	project := detect.DetectProject(".")
	cfg := config.DefaultFor(project.Stack)
	if err := config.Save(filepath.Join(root, "config.yaml"), cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	planningMode := opts.PlanningMode
	if planningMode == "" {
		if opts.InstallSkills {
			planningMode = PlanningSpecDriven
		} else {
			planningMode = PlanningManual
		}
	}
	if err := writeSetupState(setupChoices{
		CLI:      normalizeCLI(opts.CLI),
		Planning: planningMode,
		Skills:   planningUsesSkills(planningMode),
		Scope:    "project",
	}, project); err != nil {
		return err
	}
	if err := ensureProjectMemoryFiles(root); err != nil {
		return err
	}
	if err := writeTemplate(filepath.Join(root, "agent-protocol.md"), agentProtocolTemplate(harnessInvocation(), planningMode)); err != nil {
		return err
	}
	if opts.InstallSkills {
		if err := runInstallSkillsWithMode(root, planningMode); err != nil {
			return err
		}
	}

	db, err := memory.Open(filepath.Join(root, "memory.db"))
	if err != nil {
		return fmt.Errorf("init memory: %w", err)
	}
	defer db.Close()
	if err := db.Migrate(); err != nil {
		return fmt.Errorf("migrate memory: %w", err)
	}

	_ = ensureHarnessGitignore(root)

	fmt.Println("OK .harness/ initialized")
	fmt.Printf("  Project: %s\n", valueOr(project.Name, "unknown"))
	fmt.Printf("  Stack: %s\n", valueOr(project.Stack, "unknown"))
	if project.PackageManager != "" {
		fmt.Printf("  Package manager: %s\n", project.PackageManager)
	}
	if len(project.Frameworks) > 0 {
		fmt.Printf("  Frameworks: %s\n", joinList(project.Frameworks))
	}
	if len(project.CodingCLIs) > 0 {
		fmt.Printf("  Coding CLI markers: %s\n", joinList(project.CodingCLIs))
	}

	shouldInstallHooks := opts.InstallHooks || opts.CLI != "auto"
	if shouldInstallHooks {
		if err := runInstallHooks(installHookOptions{
			CLI:         opts.CLI,
			Skills:      boolSkillsModeForPlanning(planningMode),
			Planning:    planningMode,
			Interactive: isTerminal(os.Stdin) && opts.CLI == "auto",
			InstallGit:  true,
		}); err != nil {
			return err
		}
	}

	if !opts.Quiet {
		invoke := harnessInvocation()
		fmt.Println("  Next steps:")
		fmt.Println("    1. Edit .specs/project/PROJECT.md with your product spec")
		fmt.Printf("    2. %s install-hooks --interactive    # choose Codex, Claude Code, or Cursor\n", invoke)
		fmt.Printf("    3. %s feature new \"first goal\"\n", invoke)
	}
	return nil
}

func ensureHarnessSkeleton(root, planningMode string) error {
	dirs := []string{
		root,
		filepath.Join(root, "contracts"),
		filepath.Join(root, "approvals"),
		filepath.Join(root, "evaluations"),
		filepath.Join(root, "fixtures"),
		filepath.Join(root, "repairs"),
		filepath.Join(root, "screenshots"),
		filepath.Join(root, "reports"),
	}
	if normalizePlanningMode(planningMode) == PlanningSpecDriven {
		dirs = append(dirs,
			filepath.Join(root, "context"),
			filepath.Join(root, "design"),
			filepath.Join(root, "tasks"),
		)
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
	}
	if err := ensureSpecsSkeleton(root); err != nil {
		return err
	}
	return nil
}

// ensureSpecsSkeleton creates the canonical TLC artifact tree at .specs/
// next to the workspace's .harness/ directory. New projects start here;
// existing projects pick it up via `harness upgrade`.
func ensureSpecsSkeleton(harnessRoot string) error {
	specsRoot := siblingSpecsRoot(harnessRoot)
	for _, d := range []string{
		specsRoot,
		filepath.Join(specsRoot, "project"),
		filepath.Join(specsRoot, "codebase"),
		filepath.Join(specsRoot, "features"),
		filepath.Join(specsRoot, "quick"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
	}
	return nil
}

// siblingSpecsRoot returns the canonical .specs/ directory next to the
// caller-provided .harness/ root. Mirrors agreement.defaultSpecsRoot so
// both paths resolve to the same workspace location.
func siblingSpecsRoot(harnessRoot string) string {
	clean := filepath.Clean(harnessRoot)
	parent := filepath.Dir(clean)
	if parent == "" || parent == "." {
		return ".specs"
	}
	return filepath.Join(parent, ".specs")
}

func ensureProjectMemoryFiles(harnessRoot string) error {
	specsRoot := siblingSpecsRoot(harnessRoot)
	if err := writeTemplate(filepath.Join(specsRoot, "project", "PROJECT.md"), specTemplate); err != nil {
		return err
	}
	return writeTemplate(filepath.Join(specsRoot, "project", "STATE.md"), progressTemplate)
}

func writeTemplate(path, content string) error {
	if _, err := os.Stat(path); err == nil {
		return nil // do not overwrite user content
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func ensureHarnessGitignore(root string) error {
	path := filepath.Join(root, ".gitignore")
	existing := ""
	if b, err := os.ReadFile(path); err == nil {
		existing = string(b)
	}
	lines := []string{"memory.db", "reports/", "repairs/", "screenshots/", "tmp/", "run-progress.json", "run-progress.json.tmp", "watch/", "events.jsonl", "commands.log", "test-count.json", "traceability.json"}
	for _, line := range lines {
		if !strings.Contains(existing, line) {
			if existing != "" && !strings.HasSuffix(existing, "\n") {
				existing += "\n"
			}
			existing += line + "\n"
		}
	}
	return os.WriteFile(path, []byte(existing), 0o644)
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

const specTemplate = `# Product Specification

## Vision
<what you are building, in 2-3 sentences>

## Personas
- ...

## Core Features
- ...

## Non-Goals
- ...

## Constraints
- runtime: ...
- deployment: ...
- compliance: ...

## Acceptance Bar
Define here the global criteria that every sprint must satisfy.
The harness validates each sprint's contract against this bar.
`

const progressTemplate = `# Project Progress

This file is the narrative brain of the project. It is append-only, versioned
in Git, and read by every CLI as the first source of truth when resuming work.

## History

<!-- harness append below -->
`

func agentProtocolTemplate(invoke string, planningMode string) string {
	planningMode = normalizePlanningMode(planningMode)
	if planningMode == PlanningAuto {
		planningMode = PlanningManual
	}
	return `# Harness Agent Protocol
<!-- harness-agent-protocol-v4 planning:` + planningMode + ` -->

This file is for Codex, Claude Code, Cursor, and any other coding CLI working
in this repository.

The agent MUST call Harness functions autonomously through the public Harness
CLI. Do not ask the user to run Harness commands for normal QA, scoring, or
status checks.

Harness functions:

| Function | Required CLI call | When the agent calls it |
|---|---|---|
| harness.status | ` + "`" + invoke + ` feature status` + "`" + ` | At session start and before final response |
| harness.start_sprint | ` + "`" + invoke + ` feature new "<goal>"` + "`" + ` | When no active sprint contract exists |
| harness.contract_status | ` + "`" + invoke + ` contract status` + "`" + ` | Before implementation and before QA |
| harness.contract_propose | ` + "`" + invoke + ` contract propose` + "`" + ` | After writing or changing the sprint contract |
| harness.contract_approve | ` + "`" + invoke + ` contract approve --role <planner|tester>` + "`" + ` | When a required agent role agrees with the exact contract hash |
| harness.contract_reject | ` + "`" + invoke + ` contract reject --role <planner|tester> --reason "<why>"` + "`" + ` | When a required role cannot accept the contract |
| harness.qa | ` + "`" + invoke + ` feature qa --format=json` + "`" + ` | After meaningful code changes and before completion |
| harness.repair | ` + "`" + invoke + ` feature repair` + "`" + ` | When QA returns FAIL |
| harness.score | ` + "`" + invoke + ` feature score` + "`" + ` | Only after QA verdict is PASS |
| harness.doctor | ` + "`" + invoke + ` doctor` + "`" + ` | When a required sensor/tool is missing |
| harness.doctor_fix | ` + "`" + invoke + ` doctor --fix` + "`" + ` | When Doctor recommends safe Harness config repair |
| harness.terminal | ` + "`" + invoke + ` run --resume` + "`" + ` | When the user wants the live terminal dashboard |

` + planningAutomationProtocol(planningMode) + `

Autonomy rules:

1. Read .specs/project/STATE.md, .specs/project/PROJECT.md, and this file at session
   start.
2. Create or update the sprint contract before implementing a feature.
3. Run ` + "`" + invoke + ` contract propose` + "`" + ` after the contract is written.
4. If agent automation is available, use a contract-author agent for creation
   and repair, and an independent contract-reviewer/tester agent for approval
   or rejection.
5. Do not implement until ` + "`" + invoke + ` contract status` + "`" + ` returns AGREED. The
   planner and tester roles must approve the same contract hash.
6. If status is DRAFT, PROPOSED, CHANGED, REJECTED, MISSING, STALE, or
   BLOCKED, product-file edits are forbidden. Repair the contract first.
7. Never run ` + "`" + invoke + ` feature qa --allow-unagreed` + "`" + ` unless the user explicitly
   asks for an emergency override.
8. Run ` + "`" + invoke + ` feature qa --format=json` + "`" + ` without waiting for the user after
   meaningful code changes.
9. Read .harness/reports/latest.json after QA. If verdict is FAIL, run
   ` + "`" + invoke + ` feature repair` + "`" + `, read .harness/repairs/latest.md, fix the
   listed findings, and rerun QA. Repeat until verdict is PASS.
10. If Doctor reports safe Harness config drift or says to run doctor --fix,
   run ` + "`" + invoke + ` doctor --fix` + "`" + ` autonomously before asking the user.
   Only ask for user approval when installing or changing project dependencies.
11. Run ` + "`" + invoke + ` feature score` + "`" + ` only after QA returns PASS. A failing
   sprint is not complete and must not be scored unless the user explicitly
   asks for an emergency failure record.
12. Only ask the user for decisions Harness cannot make deterministically:
   product intent, changing acceptance criteria, installing missing project
   tools when that changes the app stack, or accepting visual baselines with
   ` + "`" + invoke + ` feature qa --accept-screenshots` + "`" + `, or approved behavior fixtures with
   ` + "`" + invoke + ` feature qa --accept-fixtures` + "`" + `.
`
}
