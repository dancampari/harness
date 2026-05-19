package harness

import (
	"bufio"
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
	Interactive bool
	InstallGit  bool
}

func newInstallHooksCmd() *cobra.Command {
	var only string
	var cli string
	var interactive bool
	var git bool

	cmd := &cobra.Command{
		Use:   "install-hooks",
		Short: "Install integration references for Claude Code, Codex, or Cursor",
		Long: `Installs the Harness references for the coding CLI used in this repo:
  - Claude Code:  .claude/settings.json hooks
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
				Interactive: interactive,
				InstallGit:  git,
			})
		},
	}
	cmd.Flags().StringVar(&only, "only", "", "deprecated: install only one: claude|codex|cursor|git")
	cmd.Flags().StringVar(&cli, "cli", "auto", "coding CLI: auto|codex|claude|cursor|all|none")
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

	installers := map[string]func() error{
		"claude": installClaudeHooks,
		"codex":  installCodexHooks,
		"cursor": installCursorHooks,
	}
	for _, target := range targets {
		fn := installers[target]
		if err := fn(); err != nil {
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
			return resolveHookTargets(installHookOptions{CLI: picked}, project)
		}
		return nil, fmt.Errorf("no coding CLI detected; rerun with --cli codex, --cli claude, --cli cursor, or --interactive")
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
	fmt.Println("  1) Auto-detect existing markers")
	fmt.Println("  2) Codex")
	fmt.Println("  3) Claude Code")
	fmt.Println("  4) Cursor")
	fmt.Println("  5) All three")
	fmt.Println("  6) None")
	fmt.Print("Select [1]: ")

	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return "", err
	}
	switch strings.TrimSpace(strings.ToLower(line)) {
	case "", "1", "auto":
		return "auto", nil
	case "2", "codex":
		return "codex", nil
	case "3", "claude", "claude code", "claude-code":
		return "claude", nil
	case "4", "cursor":
		return "cursor", nil
	case "5", "all":
		return "all", nil
	case "6", "none", "skip":
		return "none", nil
	default:
		return "", fmt.Errorf("invalid selection %q", strings.TrimSpace(line))
	}
}

func installClaudeHooks() error {
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
	fmt.Println("  OK Claude Code references installed:", path)
	return nil
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

func installCodexHooks() error {
	path := "AGENTS.md"
	addendum := codexAddendum(harnessInvocation())
	if existing, err := os.ReadFile(path); err == nil {
		if strings.Contains(string(existing), "## Harness Gate") {
			fmt.Println("  OK Codex: AGENTS.md already contains Harness Gate")
			return nil
		}
		addendum = string(existing) + "\n\n" + addendum
	}
	if err := os.WriteFile(path, []byte(addendum), 0o644); err != nil {
		return err
	}
	fmt.Println("  OK Codex references installed:", path)
	return nil
}

func installCursorHooks() error {
	dir := filepath.Join(".cursor", "rules")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "harness.mdc")
	if err := os.WriteFile(path, []byte(cursorRule(harnessInvocation())), 0o644); err != nil {
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

func codexAddendum(invoke string) string {
	return `## Harness Gate

This project uses Harness Engineering. After any feature, PR, or development
phase, you MUST follow this protocol:

1. Read .harness/progress.md to recover context from previous sessions.
2. Read .harness/spec.md for the global product spec.
3. If starting a new sprint, run: ` + "`" + invoke + ` sprint new "<goal>"` + "`" + `
4. Fill in the contract at .harness/contracts/sprint-NNN.md with:
   - Deliverables (files + symbols expected)
   - Acceptance Criteria (with thresholds 1-10)
   - Constraints (forbidden imports, complexity limits)
5. Implement the feature.
6. Run: ` + "`" + invoke + ` sprint qa --format=json` + "`" + `
7. Read .harness/reports/latest.json. If score < 80 or any high/critical
   findings exist, iterate before declaring the task complete.
8. Run: ` + "`" + invoke + ` sprint score` + "`" + ` to consolidate and update progress.md.

Never declare a task done without a passing QA verdict from the harness.
`
}

func cursorRule(invoke string) string {
	return `---
description: Harness Engineering integration
alwaysApply: true
---

This project uses Harness Engineering. Always:

1. On session start, read .harness/progress.md and .harness/spec.md.
2. Before implementing a feature, ensure a contract exists at
   .harness/contracts/sprint-NNN.md. If not, run ` + "`" + invoke + ` sprint new "<goal>"` + "`" + `
   and fill it in.
3. After implementing, run ` + "`" + invoke + ` sprint qa` + "`" + ` in the integrated terminal.
4. Process .harness/reports/latest.json. Iterate on findings before
   marking the task complete.
5. Run ` + "`" + invoke + ` sprint score` + "`" + ` to update progress.md.

Consult ` + "`" + invoke + ` trend` + "`" + ` to understand the quality trajectory of the project.
`
}
