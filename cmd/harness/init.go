package harness

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

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

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize .harness/ in the current repo",
		Long: `Creates the .harness/ directory with:
  - config.yaml (auto-detected for your stack)
  - spec.md (template you should fill in)
  - progress.md (the narrative brain of the project)
  - contracts/, evaluations/, screenshots/, reports/ (empty dirs)
  - memory.db (SQLite index, initialized)`,
		RunE: func(cmd *cobra.Command, args []string) error {
			skillsMode := normalizeSkillsMode(skills)
			if skillsMode != "on" && skillsMode != "off" {
				return fmt.Errorf("unknown skills mode %q; use on|off", skills)
			}
			return runInit(initOptions{
				Force:         force,
				CLI:           cli,
				InstallHooks:  installHooks,
				InstallSkills: skillsMode == "on",
			})
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing .harness/")
	cmd.Flags().StringVar(&cli, "cli", "auto", "coding CLI to configure: auto|codex|claude|cursor|all|none")
	cmd.Flags().StringVar(&skills, "skills", "off", "install contract automation skills: on|off")
	cmd.Flags().BoolVar(&installHooks, "install-hooks", false, "install coding CLI references during init")
	return cmd
}

type initOptions struct {
	Force         bool
	CLI           string
	InstallHooks  bool
	InstallSkills bool
	Quiet         bool
}

func runInit(opts initOptions) error {
	root := ".harness"
	if _, err := os.Stat(root); err == nil && !opts.Force {
		return errors.New(".harness/ already exists - use --force to overwrite")
	}

	dirs := []string{
		root,
		filepath.Join(root, "contracts"),
		filepath.Join(root, "evaluations"),
		filepath.Join(root, "screenshots"),
		filepath.Join(root, "reports"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
	}

	project := detect.DetectProject(".")
	cfg := config.DefaultFor(project.Stack)
	if err := config.Save(filepath.Join(root, "config.yaml"), cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	if err := writeTemplate(filepath.Join(root, "spec.md"), specTemplate); err != nil {
		return err
	}
	if err := writeTemplate(filepath.Join(root, "progress.md"), progressTemplate); err != nil {
		return err
	}
	if err := writeTemplate(filepath.Join(root, "agent-protocol.md"), agentProtocolTemplate(harnessInvocation(), opts.InstallSkills)); err != nil {
		return err
	}
	if opts.InstallSkills {
		if err := runInstallSkills(root); err != nil {
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

	gi := filepath.Join(root, ".gitignore")
	_ = os.WriteFile(gi, []byte("memory.db\nreports/\nscreenshots/\n"), 0o644)

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
			Skills:      boolSkillsMode(opts.InstallSkills),
			Interactive: isTerminal(os.Stdin) && opts.CLI == "auto",
			InstallGit:  true,
		}); err != nil {
			return err
		}
	}

	if !opts.Quiet {
		invoke := harnessInvocation()
		fmt.Println("  Next steps:")
		fmt.Println("    1. Edit .harness/spec.md with your product spec")
		fmt.Printf("    2. %s install-hooks --interactive    # choose Codex, Claude Code, or Cursor\n", invoke)
		fmt.Printf("    3. %s sprint new \"first goal\"\n", invoke)
	}
	return nil
}

func writeTemplate(path, content string) error {
	if _, err := os.Stat(path); err == nil {
		return nil // do not overwrite user content
	}
	return os.WriteFile(path, []byte(content), 0o644)
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

func agentProtocolTemplate(invoke string, skillsEnabled bool) string {
	return `# Harness Agent Protocol

This file is for Codex, Claude Code, Cursor, and any other coding CLI working
in this repository.

The agent MUST call Harness functions autonomously through the public Harness
CLI. Do not ask the user to run Harness commands for normal QA, scoring, or
status checks.

Harness functions:

| Function | Required CLI call | When the agent calls it |
|---|---|---|
| harness.status | ` + "`" + invoke + ` sprint status` + "`" + ` | At session start and before final response |
| harness.start_sprint | ` + "`" + invoke + ` sprint new "<goal>"` + "`" + ` | When no active sprint contract exists |
| harness.qa | ` + "`" + invoke + ` sprint qa --format=json` + "`" + ` | After meaningful code changes and before completion |
| harness.score | ` + "`" + invoke + ` sprint score` + "`" + ` | After QA has produced the final verdict |
| harness.doctor | ` + "`" + invoke + ` doctor` + "`" + ` | When a required sensor/tool is missing |
| harness.terminal | ` + "`" + invoke + ` run --resume` + "`" + ` | When the user wants the live terminal dashboard |

` + contractAutomationProtocol(skillsEnabled) + `

Autonomy rules:

1. Read .harness/progress.md, .harness/spec.md, and this file at session
   start.
2. Create or update the sprint contract before implementing a feature.
3. Run ` + "`" + invoke + ` sprint qa --format=json` + "`" + ` without waiting for the user after
   meaningful code changes.
4. Read .harness/reports/latest.json after QA. Fix high/critical findings and
   rerun QA.
5. Run ` + "`" + invoke + ` sprint score` + "`" + ` before declaring the work complete.
6. Only ask the user for decisions Harness cannot make deterministically:
   product intent, changing acceptance criteria, installing missing project
   tools when that changes the app stack, or accepting visual baselines with
   ` + "`" + invoke + ` sprint qa --accept-screenshots` + "`" + `.
`
}
