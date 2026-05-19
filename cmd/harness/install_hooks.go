package harness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dancampari/harness/internal/detect"
	"github.com/spf13/cobra"
)

type installHookOptions struct {
	CLI         string
	Skills      string
	Interactive bool
	InstallGit  bool
}

func newInstallHooksCmd() *cobra.Command {
	var only string
	var cli string
	var skills string
	var interactive bool
	var git bool

	cmd := &cobra.Command{
		Use:   "install-hooks",
		Short: "Install integration references for Claude Code, Codex, or Cursor",
		Long: `Installs the Harness references for the coding CLI used in this repo:
  - Claude Code:  CLAUDE.md instructions + .claude/settings.json hooks
  - Codex:        AGENTS.md instructions
  - Cursor:       .cursor/rules/harness.mdc

By default Harness auto-detects existing CLI markers. Use --interactive for
a guided install, or --cli codex|claude|cursor|all|none in scripts.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if only != "" {
				cli = only
				git = only == "git"
			}
			return runInstallHooks(installHookOptions{
				CLI:         cli,
				Skills:      skills,
				Interactive: interactive,
				InstallGit:  git,
			})
		},
	}
	cmd.Flags().StringVar(&only, "only", "", "deprecated: install only one: claude|codex|cursor|git")
	cmd.Flags().StringVar(&cli, "cli", "auto", "coding CLI: auto|codex|claude|cursor|all|none")
	cmd.Flags().StringVar(&skills, "skills", "auto", "contract automation skills: auto|on|off")
	cmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "ask which coding CLI to configure")
	cmd.Flags().BoolVar(&git, "git", true, "also install the git pre-push safety hook")
	return cmd
}

func runInstallHooks(opts installHookOptions) error {
	project := detect.DetectProject(".")
	targets, err := resolveHookTargets(opts, project)
	if err != nil {
		return err
	}
	skillsEnabled, err := resolveHookSkills(opts.Skills)
	if err != nil {
		return err
	}
	if skillsEnabled {
		if !skillsInstalled(".harness") {
			if err := runInstallSkills(".harness"); err != nil {
				return err
			}
		} else if err := ensureAgentProtocolMode(".harness", true); err != nil {
			return err
		}
	} else if _, err := os.Stat(".harness"); err == nil {
		if err := ensureAgentProtocolMode(".harness", false); err != nil {
			return err
		}
	}

	installers := map[string]func(bool) error{
		"claude": installClaudeHooks,
		"codex":  installCodexHooks,
		"cursor": installCursorHooks,
	}
	for _, target := range targets {
		fn := installers[target]
		if err := fn(skillsEnabled); err != nil {
			return fmt.Errorf("%s: %w", target, err)
		}
	}
	if opts.InstallGit {
		if err := installGitHook(); err != nil {
			fmt.Fprintf(os.Stderr, "  ! git hook skipped: %v\n", err)
		}
	}
	if len(targets) == 0 && !opts.InstallGit {
		fmt.Println("No Harness references installed.")
	}
	return nil
}

func resolveHookSkills(mode string) (bool, error) {
	switch normalizeSkillsMode(mode) {
	case "auto":
		return skillsInstalled(".harness"), nil
	case "on":
		return true, nil
	case "off":
		return false, nil
	default:
		return false, fmt.Errorf("unknown skills mode %q; use auto|on|off", mode)
	}
}

func resolveHookTargets(opts installHookOptions, project detect.ProjectInfo) ([]string, error) {
	cli := normalizeCLI(opts.CLI)
	if opts.Interactive {
		picked, err := promptHookTarget(project)
		if err != nil {
			return nil, err
		}
		cli = picked
	}

	switch cli {
	case "auto":
		if len(project.CodingCLIs) > 0 {
			return project.CodingCLIs, nil
		}
		if isTerminal(os.Stdin) {
			picked, err := promptHookTarget(project)
			if err != nil {
				return nil, err
			}
			if normalizeCLI(picked) == "auto" {
				if len(project.CodingCLIs) > 0 {
					return project.CodingCLIs, nil
				}
				fmt.Println("  No existing CLI markers found; installing all agent references.")
				return []string{"claude", "codex", "cursor"}, nil
			}
			return resolveHookTargets(installHookOptions{CLI: picked}, project)
		}
		fmt.Println("  No coding CLI detected in non-interactive mode; installing all agent references.")
		return []string{"claude", "codex", "cursor"}, nil
	case "all":
		return []string{"claude", "codex", "cursor"}, nil
	case "none", "git":
		return nil, nil
	case "claude", "codex", "cursor":
		return []string{cli}, nil
	default:
		return nil, fmt.Errorf("unknown coding CLI %q; use auto|codex|claude|cursor|all|none", cli)
	}
}

func normalizeCLI(value string) string {
	v := strings.ToLower(strings.TrimSpace(value))
	switch v {
	case "", "detect":
		return "auto"
	case "claude-code", "claude_code", "claudecode":
		return "claude"
	}
	return v
}

func promptHookTarget(project detect.ProjectInfo) (string, error) {
	if !isTerminal(os.Stdin) {
		return "", fmt.Errorf("--interactive requires a terminal")
	}
	fmt.Println("Detected project:")
	fmt.Printf("  Name: %s\n", valueOr(project.Name, "unknown"))
	fmt.Printf("  Stack: %s\n", valueOr(project.Stack, "unknown"))
	if project.PackageManager != "" {
		fmt.Printf("  Package manager: %s\n", project.PackageManager)
	}
	if len(project.Frameworks) > 0 {
		fmt.Printf("  Frameworks: %s\n", joinList(project.Frameworks))
	}
	if len(project.CodingCLIs) > 0 {
		fmt.Printf("  Existing CLI markers: %s\n", joinList(project.CodingCLIs))
	}
	fmt.Println()
	fmt.Println("Which coding CLI will implement code in this repo?")
	fmt.Println("  1) Claude Code")
	fmt.Println("  2) Codex")
	fmt.Println("  3) Cursor IDE")
	if len(project.CodingCLIs) > 0 {
		fmt.Println("  4) Auto-detect existing markers")
	} else {
		fmt.Println("  4) Auto / all references")
	}
	fmt.Println("  5) All three")
	fmt.Println("  6) None")
	fmt.Print("Select [4]: ")

	line, err := readPromptLine()
	if err != nil {
		return "", err
	}
	switch strings.TrimSpace(strings.ToLower(line)) {
	case "1", "claude", "claude code", "claude-code":
		return "claude", nil
	case "2", "codex":
		return "codex", nil
	case "3", "cursor", "cursor ide":
		return "cursor", nil
	case "", "4", "auto":
		return "auto", nil
	case "5", "all":
		return "all", nil
	case "6", "none", "skip":
		return "none", nil
	default:
		return "", fmt.Errorf("invalid selection %q", strings.TrimSpace(line))
	}
}

func installClaudeHooks(skillsEnabled bool) error {
	if err := installClaudeMemory(skillsEnabled); err != nil {
		return err
	}
	dir := ".claude"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "settings.json")
	settings := map[string]any{}
	if existing, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(existing))) > 0 {
		if err := json.Unmarshal(existing, &settings); err != nil {
			return fmt.Errorf("parse existing %s: %w", path, err)
		}
	}
	hooks := objectValue(settings, "hooks")
	invoke := harnessInvocation()
	appendClaudeHook(hooks, "Stop", "*", invoke+" sprint qa --format=json")
	appendClaudeHook(hooks, "PostToolUse", "Bash(git commit*)", invoke+" sprint qa --format=tty")

	content, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return err
	}
	fmt.Println("  OK Claude Code references installed: CLAUDE.md,", path)
	return nil
}

func installClaudeMemory(skillsEnabled bool) error {
	path := "CLAUDE.md"
	content := claudeMemory(harnessInvocation(), skillsEnabled)
	if existing, err := os.ReadFile(path); err == nil {
		text := string(existing)
		if strings.Contains(text, "## Harness Gate") {
			content = replaceMarkdownSection(text, "## Harness Gate", content)
		} else {
			content = text + "\n\n" + content
		}
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func objectValue(parent map[string]any, key string) map[string]any {
	if existing, ok := parent[key].(map[string]any); ok {
		return existing
	}
	next := map[string]any{}
	parent[key] = next
	return next
}

func appendClaudeHook(hooks map[string]any, event, matcher, command string) {
	entries, _ := hooks[event].([]any)
	for i, entry := range entries {
		entryMap, ok := entry.(map[string]any)
		if !ok || fmt.Sprint(entryMap["matcher"]) != matcher {
			continue
		}
		hookList, _ := entryMap["hooks"].([]any)
		for _, hook := range hookList {
			hookMap, ok := hook.(map[string]any)
			if ok && fmt.Sprint(hookMap["command"]) == command {
				return
			}
		}
		entryMap["hooks"] = append(hookList, map[string]any{
			"type":    "command",
			"command": command,
		})
		entries[i] = entryMap
		hooks[event] = entries
		return
	}
	hooks[event] = append(entries, map[string]any{
		"matcher": matcher,
		"hooks": []any{map[string]any{
			"type":    "command",
			"command": command,
		}},
	})
}

func installCodexHooks(skillsEnabled bool) error {
	path := "AGENTS.md"
	addendum := codexAddendum(harnessInvocation(), skillsEnabled)
	if existing, err := os.ReadFile(path); err == nil {
		text := string(existing)
		if strings.Contains(text, "## Harness Gate") {
			addendum = replaceMarkdownSection(text, "## Harness Gate", addendum)
		} else {
			addendum = text + "\n\n" + addendum
		}
	}
	if err := os.WriteFile(path, []byte(addendum), 0o644); err != nil {
		return err
	}
	fmt.Println("  OK Codex references installed:", path)
	return nil
}

func replaceMarkdownSection(source, heading, replacement string) string {
	start := strings.Index(source, heading)
	if start < 0 {
		return source + "\n\n" + replacement
	}
	afterStart := start + len(heading)
	nextRel := strings.Index(source[afterStart:], "\n## ")
	if nextRel < 0 {
		return strings.TrimSpace(source[:start]) + "\n\n" + replacement
	}
	end := afterStart + nextRel
	return strings.TrimSpace(source[:start]) + "\n\n" + replacement + "\n\n" + strings.TrimSpace(source[end:])
}

func installCursorHooks(skillsEnabled bool) error {
	dir := filepath.Join(".cursor", "rules")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "harness.mdc")
	if err := os.WriteFile(path, []byte(cursorRule(harnessInvocation(), skillsEnabled)), 0o644); err != nil {
		return err
	}
	fmt.Println("  OK Cursor references installed:", path)
	return nil
}

func installGitHook() error {
	dir := filepath.Join(".git", "hooks")
	if _, err := os.Stat(".git"); err != nil {
		return fmt.Errorf("not a git repo")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "pre-push")
	content := fmt.Sprintf(`#!/bin/sh
# harness pre-push hook - reports only, never blocks
%s sprint qa --format=tty || true
exit 0
`, harnessInvocation())
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		return err
	}
	fmt.Println("  OK Git pre-push hook installed:", path)
	return nil
}

func harnessInvocation() string {
	if value := strings.TrimSpace(os.Getenv("HARNESS_INVOKE")); value != "" {
		return value
	}
	return "harness"
}

func codexAddendum(invoke string, skillsEnabled bool) string {
	return `## Harness Gate
<!-- harness-agent-protocol-v3 -->

This project uses Harness Engineering. You MUST call Harness functions
autonomously through the CLI commands below. Do not ask the user to run Harness
for normal QA, status, or score checks.

Harness function calls:

- harness.status: ` + "`" + invoke + ` sprint status` + "`" + `
- harness.start_sprint: ` + "`" + invoke + ` sprint new "<goal>"` + "`" + `
- harness.qa: ` + "`" + invoke + ` sprint qa --format=json` + "`" + `
- harness.score: ` + "`" + invoke + ` sprint score` + "`" + `
- harness.doctor: ` + "`" + invoke + ` doctor` + "`" + `
- harness.terminal: ` + "`" + invoke + ` run --resume` + "`" + `

` + contractAutomationProtocol(skillsEnabled) + `

Autonomous protocol:

1. Read .harness/progress.md to recover context from previous sessions.
2. Read .harness/spec.md for the global product spec.
3. Read .harness/agent-protocol.md for the current Harness function contract.
4. Run ` + "`" + invoke + ` sprint status` + "`" + ` before starting implementation.
5. If no active sprint contract exists, run ` + "`" + invoke + ` sprint new "<goal>"` + "`" + `
   and fill in the contract at .harness/contracts/sprint-NNN.md with:
   - Deliverables (files + symbols expected)
   - Acceptance Criteria (with thresholds 1-10)
   - Constraints (forbidden imports, complexity limits)
6. Implement the feature.
7. After meaningful code changes, run ` + "`" + invoke + ` sprint qa --format=json` + "`" + `
   without waiting for the user.
8. Read .harness/reports/latest.json. If the verdict is FAIL or any
   high/critical findings exist, fix them and rerun QA.
9. Run ` + "`" + invoke + ` sprint score` + "`" + ` before declaring the task complete.

Only ask the user for product decisions, acceptance-criteria changes, dependency
installation approval when it changes the project stack, or visual baseline
approval via ` + "`" + invoke + ` sprint qa --accept-screenshots` + "`" + `.

Never declare a task done without a passing QA verdict from Harness.
`
}

func claudeMemory(invoke string, skillsEnabled bool) string {
	return `## Harness Gate
<!-- harness-claude-protocol-v3 -->

Claude Code MUST call Harness functions autonomously through the CLI commands
below. Do not ask the user to run Harness for normal QA, status, score, or
doctor checks.

Harness function calls:

- harness.status: ` + "`" + invoke + ` sprint status` + "`" + `
- harness.start_sprint: ` + "`" + invoke + ` sprint new "<goal>"` + "`" + `
- harness.qa: ` + "`" + invoke + ` sprint qa --format=json` + "`" + `
- harness.score: ` + "`" + invoke + ` sprint score` + "`" + `
- harness.doctor: ` + "`" + invoke + ` doctor` + "`" + `
- harness.terminal: ` + "`" + invoke + ` run --resume` + "`" + `

` + contractAutomationProtocol(skillsEnabled) + `

Autonomous protocol for Claude Code:

1. At session start, read .harness/progress.md, .harness/spec.md, and
   .harness/agent-protocol.md.
2. Run ` + "`" + invoke + ` sprint status` + "`" + ` before implementation.
3. Create or update the sprint contract when needed.
4. After meaningful code changes, run ` + "`" + invoke + ` sprint qa --format=json` + "`" + `
   without asking the user.
5. Read .harness/reports/latest.json. Fix high/critical findings and rerun QA.
6. Run ` + "`" + invoke + ` sprint score` + "`" + ` before saying the task is complete.

Only ask the user for product decisions, acceptance criteria changes,
dependency installation approval when it changes the project stack, or visual
baseline approval via ` + "`" + invoke + ` sprint qa --accept-screenshots` + "`" + `.
`
}

func cursorRule(invoke string, skillsEnabled bool) string {
	return `---
description: Harness Engineering integration
alwaysApply: true
---

This project uses Harness Engineering. Always:

1. On session start, read .harness/progress.md and .harness/spec.md.
2. Read .harness/agent-protocol.md. It defines the Harness functions you must
   call autonomously through CLI commands.
` + cursorContractAutomationProtocol(skillsEnabled) + `
4. Before implementing a feature, ensure a contract exists at
   .harness/contracts/sprint-NNN.md. If not, run ` + "`" + invoke + ` sprint new "<goal>"` + "`" + `
   and fill it in.
5. After implementing, run ` + "`" + invoke + ` sprint qa --format=json` + "`" + ` in the integrated terminal
   without asking the user to run it.
6. Process .harness/reports/latest.json. Iterate on findings before
   marking the task complete.
7. Run ` + "`" + invoke + ` sprint score` + "`" + ` to update progress.md.

Consult ` + "`" + invoke + ` trend` + "`" + ` to understand the quality trajectory of the project.
`
}

func contractAutomationProtocol(enabled bool) string {
	if enabled {
		return `Contract automation skills are enabled.

Before creating or editing a sprint contract, read:

- .harness/skills/contract-authoring/SKILL.md

Use that skill to decompose the user's prompt into small sprints, create the
current sprint contract, and fill the Markdown completely. Do not ask the user
to write the contract by hand. Ask only the smallest product question when the
request is too ambiguous to make objective acceptance criteria.`
	}
	return `Contract automation skills are disabled.

Use Harness for QA, scoring, status, and reports, but do not invent detailed
contracts from the user's prompt unless the user explicitly asks for that. In
manual mode, create the template when needed and let the user author or approve
the contract.`
}

func cursorContractAutomationProtocol(enabled bool) string {
	if enabled {
		return `3. Contract automation skills are enabled. Before creating or editing a
   sprint contract, read .harness/skills/contract-authoring/SKILL.md. Use it to
   decompose the user's prompt and fill the contract automatically.
`
	}
	return `3. Contract automation skills are disabled. Do not invent detailed sprint
   contracts unless the user explicitly asks for automation.
`
}
