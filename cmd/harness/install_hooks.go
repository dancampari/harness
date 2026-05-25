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
	CLI              string
	Skills           string
	Planning         string
	Interactive      bool
	InstallGit       bool
	InstallPreCommit bool
}

func newInstallHooksCmd() *cobra.Command {
	var only string
	var cli string
	var skills string
	var planning string
	var interactive bool
	var git bool
	var preCommit bool

	cmd := &cobra.Command{
		Use:   "install-hooks",
		Short: "Install integration references for Claude Code, Codex, or Cursor",
		Long: `Installs the Harness references for the coding CLI used in this repo:
  - Claude Code:  CLAUDE.md instructions + .claude/settings.json hooks
  - Codex:        AGENTS.md instructions
  - Cursor:       .cursor/rules/harness.mdc

By default Harness auto-detects existing CLI markers. Use --interactive for
a guided install, or --cli codex|claude|cursor|all|none in scripts.

Pass --pre-commit to also install a git pre-commit hook that runs the
fast-feedback shift-left check (harness sprint qa --fast) on every commit.
The pre-push hook remains non-blocking; pre-commit blocks the commit when
fast QA returns FAIL.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if only != "" {
				cli = only
				git = only == "git"
			}
			return runInstallHooks(installHookOptions{
				CLI:              cli,
				Skills:           skills,
				Planning:         planning,
				Interactive:      interactive,
				InstallGit:       git,
				InstallPreCommit: preCommit,
			})
		},
	}
	cmd.Flags().StringVar(&only, "only", "", "deprecated: install only one: claude|codex|cursor|git")
	cmd.Flags().StringVar(&cli, "cli", "auto", "coding CLI: auto|codex|claude|cursor|all|none")
	cmd.Flags().StringVar(&planning, "planning", "auto", "planning automation: auto|spec-driven|contract|manual")
	cmd.Flags().StringVar(&skills, "skills", "auto", "legacy alias for planning: auto|on|off")
	cmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "ask which coding CLI to configure")
	cmd.Flags().BoolVar(&git, "git", true, "also install the git pre-push safety hook")
	cmd.Flags().BoolVar(&preCommit, "pre-commit", false,
		"install a git pre-commit hook that runs harness sprint qa --fast and blocks on FAIL")
	return cmd
}

func runInstallHooks(opts installHookOptions) error {
	if _, err := os.Stat(".harness"); err == nil {
		if err := installProjectCommand(); err != nil {
			return err
		}
		if err := ensureHarnessGitignore(".harness"); err != nil {
			return err
		}
	}
	project := detect.DetectProject(".")
	targets, err := resolveHookTargets(opts, project)
	if err != nil {
		return err
	}
	planningMode, err := resolveHookPlanning(opts.Planning, opts.Skills)
	if err != nil {
		return err
	}
	if planningUsesSkills(planningMode) {
		if !skillsInstalled(".harness") {
			if err := runInstallSkillsWithMode(".harness", planningMode); err != nil {
				return err
			}
		} else {
			if err := runInstallSkillsWithOptions(".harness", true, planningMode); err != nil {
				return err
			}
		}
	} else if _, err := os.Stat(".harness"); err == nil {
		if err := ensureAgentProtocolMode(".harness", PlanningManual); err != nil {
			return err
		}
	}

	installers := map[string]func(string) error{
		"claude": installClaudeHooks,
		"codex":  installCodexHooks,
		"cursor": installCursorHooks,
	}
	for _, target := range targets {
		fn := installers[target]
		if err := fn(planningMode); err != nil {
			return fmt.Errorf("%s: %w", target, err)
		}
	}
	if opts.InstallGit {
		if err := installGitHook(); err != nil {
			fmt.Fprintf(os.Stderr, "  ! git hook skipped: %v\n", err)
		}
	}
	if opts.InstallPreCommit {
		if err := installGitPreCommitHook(); err != nil {
			fmt.Fprintf(os.Stderr, "  ! git pre-commit hook skipped: %v\n", err)
		}
	}
	if len(targets) == 0 && !opts.InstallGit && !opts.InstallPreCommit {
		fmt.Println("No Harness references installed.")
	}
	return nil
}

func resolveHookPlanning(planning, skills string) (string, error) {
	mode := normalizePlanningMode(planning)
	switch mode {
	case PlanningSpecDriven, PlanningContract, PlanningManual:
		return mode, nil
	case PlanningAuto:
		fromSkills, err := planningModeFromSkills(skills)
		if err != nil {
			return "", err
		}
		if fromSkills != PlanningAuto {
			return fromSkills, nil
		}
		if _, err := os.Stat(".harness"); err == nil {
			if state := readSetupState(filepath.Join(".harness", "setup.json")); state.PlanningMode != "" {
				return state.PlanningMode, nil
			}
			return planningModeFromInstalled(".harness"), nil
		}
		return PlanningManual, nil
	default:
		return "", fmt.Errorf("unknown planning mode %q; use auto|spec-driven|contract|manual", planning)
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
	autoLabel := "Auto / all references"
	autoDescription := "Install all agent references when no marker exists"
	if len(project.CodingCLIs) > 0 {
		autoLabel = "Auto-detect existing markers"
		autoDescription = "Use the CLI markers already present in this repo"
	}
	return promptSelect("Which coding CLI will implement code in this repo?", []promptOption{
		{Label: "Claude Code", Description: "Generate CLAUDE.md and Claude Code hooks/settings", Value: "claude"},
		{Label: "Codex", Description: "Generate AGENTS.md Harness Gate", Value: "codex"},
		{Label: "Cursor IDE", Description: "Generate .cursor/rules/harness.mdc", Value: "cursor"},
		{Label: autoLabel, Description: autoDescription, Value: "auto"},
		{Label: "All three", Description: "Install Claude Code, Codex, and Cursor references", Value: "all"},
		{Label: "None", Description: "Skip agent references and keep Harness manual", Value: "none"},
	}, 3)
}

func installClaudeHooks(planningMode string) error {
	if err := installClaudeMemory(planningMode); err != nil {
		return err
	}
	if planningUsesSkills(planningMode) {
		if err := installClaudeAgents(planningMode); err != nil {
			return err
		}
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
	appendClaudeHook(hooks, "PreToolUse", "Edit|MultiEdit|Write", invoke+" guard pre-tool")
	appendClaudeHook(hooks, "PostToolUse", "Edit|MultiEdit|Write", invoke+" guard post-tool")
	appendClaudeHook(hooks, "PostToolUse", "Bash", invoke+" guard post-tool")
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

func installClaudeAgents(planningMode string) error {
	dir := filepath.Join(".claude", "agents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	files := map[string]string{
		filepath.Join(dir, "harness-contract-author.md"):   claudeContractAuthorAgent,
		filepath.Join(dir, "harness-contract-reviewer.md"): claudeContractReviewerAgent,
	}
	if planningMode == PlanningSpecDriven {
		files[filepath.Join(dir, "harness-spec-planner.md")] = claudeSpecPlannerAgent
		files[filepath.Join(dir, "harness-task-worker.md")] = claudeTaskWorkerAgent
		files[filepath.Join(dir, "harness-researcher.md")] = claudeResearcherAgent
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func installClaudeMemory(planningMode string) error {
	path := "CLAUDE.md"
	content := claudeMemory(harnessInvocation(), planningMode)
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

func installCodexHooks(planningMode string) error {
	path := "AGENTS.md"
	addendum := codexAddendum(harnessInvocation(), planningMode)
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
	if err := installCodexHookConfig(); err != nil {
		return err
	}
	if planningUsesSkills(planningMode) {
		if err := installCodexAgents(planningMode); err != nil {
			return err
		}
	}
	fmt.Println("  OK Codex references installed:", path, ".codex/hooks.json")
	return nil
}

func installCodexHookConfig() error {
	dir := ".codex"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "hooks.json")
	hooks := map[string]any{}
	if existing, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(existing))) > 0 {
		if err := json.Unmarshal(existing, &hooks); err != nil {
			return fmt.Errorf("parse existing %s: %w", path, err)
		}
	}
	root := objectValue(hooks, "hooks")
	appendCodexHook(root, "PreToolUse", "apply_patch|Edit|MultiEdit|Write", harnessInvocation()+" guard pre-tool")
	appendCodexHook(root, "PostToolUse", "apply_patch|Edit|MultiEdit|Write|Bash|Shell", harnessInvocation()+" guard post-tool")
	content, err := json.MarshalIndent(hooks, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')
	return os.WriteFile(path, content, 0o644)
}

func appendCodexHook(hooks map[string]any, event, matcher, command string) {
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
			"timeout": 10,
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
			"timeout": 10,
		}},
	})
}

func installCodexAgents(planningMode string) error {
	dir := filepath.Join(".codex", "agents")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	files := map[string]string{
		filepath.Join(dir, "harness-contract-author.toml"):   codexContractAuthorAgent,
		filepath.Join(dir, "harness-contract-reviewer.toml"): codexContractReviewerAgent,
	}
	if planningMode == PlanningSpecDriven {
		files[filepath.Join(dir, "harness-spec-planner.toml")] = codexSpecPlannerAgent
		files[filepath.Join(dir, "harness-task-worker.toml")] = codexTaskWorkerAgent
		files[filepath.Join(dir, "harness-researcher.toml")] = codexResearcherAgent
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return err
		}
	}
	configPath := filepath.Join(".codex", "config.toml")
	if existing, err := os.ReadFile(configPath); err == nil {
		text := string(existing)
		if !strings.Contains(text, "[agents]") {
			text = strings.TrimRight(text, "\r\n") + "\n\n[agents]\nmax_threads = 4\nmax_depth = 1\n"
			return os.WriteFile(configPath, []byte(text), 0o644)
		}
		return nil
	}
	return os.WriteFile(configPath, []byte("[agents]\nmax_threads = 4\nmax_depth = 1\n"), 0o644)
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

func installCursorHooks(planningMode string) error {
	dir := filepath.Join(".cursor", "rules")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "harness.mdc")
	if err := os.WriteFile(path, []byte(cursorRule(harnessInvocation(), planningMode)), 0o644); err != nil {
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
repo_root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$repo_root" || exit 0
if [ ! -f ".harness/config.yaml" ]; then
  exit 0
fi
%s sprint qa --format=tty || true
exit 0
`, harnessInvocation())
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		return err
	}
	fmt.Println("  OK Git pre-push hook installed:", path)
	return nil
}

// installGitPreCommitHook writes a blocking pre-commit hook that runs
// the shift-left fast QA pass. Unlike pre-push, this hook DOES block on
// FAIL: the whole point is to catch lint/typecheck/architecture issues
// before the bad commit lands in history. Users can still bypass with
// `git commit --no-verify` when needed, which is standard git behavior.
//
// The hook skips itself when there is no .harness/config.yaml so cloning
// a repo and committing before running `harness init` does not break.
func installGitPreCommitHook() error {
	dir := filepath.Join(".git", "hooks")
	if _, err := os.Stat(".git"); err != nil {
		return fmt.Errorf("not a git repo")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "pre-commit")
	content := fmt.Sprintf(`#!/bin/sh
# harness pre-commit hook - shift-left fast feedback
# Blocks the commit when fast QA returns FAIL. Bypass with --no-verify.
repo_root="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
cd "$repo_root" || exit 0
if [ ! -f ".harness/config.yaml" ]; then
  exit 0
fi
%s sprint qa --fast --format=tty
exit $?
`, harnessInvocation())
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		return err
	}
	fmt.Println("  OK Git pre-commit hook installed:", path)
	return nil
}

func harnessInvocation() string {
	local := filepath.Join(".harness", "bin", executableName("harness"))
	localInvoke := ""
	if _, err := os.Stat(local); err == nil {
		localInvoke = "./" + filepath.ToSlash(local)
	}
	if value := strings.TrimSpace(os.Getenv("HARNESS_INVOKE")); value != "" {
		if localInvoke != "" && isNpxHarnessInvocation(value) {
			return localInvoke
		}
		return value
	}
	if localInvoke != "" {
		return "./" + filepath.ToSlash(local)
	}
	return "harness"
}

func isNpxHarnessInvocation(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(value, "npx github:dancampari/harness#") ||
		strings.HasPrefix(value, "npx @dancampari/harness")
}

func codexAddendum(invoke string, planningMode string) string {
	planningMode = normalizePlanningMode(planningMode)
	return `## Harness Gate
<!-- harness-agent-protocol-v4 planning:` + planningMode + ` -->

This project uses Harness Engineering. You MUST call Harness functions
autonomously through the CLI commands below. Do not ask the user to run Harness
for normal QA, status, or score checks.

Harness function calls:

- harness.status: ` + "`" + invoke + ` sprint status` + "`" + `
- harness.start_sprint: ` + "`" + invoke + ` sprint new "<goal>"` + "`" + `
- harness.contract_status: ` + "`" + invoke + ` contract status` + "`" + `
- harness.contract_propose: ` + "`" + invoke + ` contract propose` + "`" + `
- harness.contract_approve: ` + "`" + invoke + ` contract approve --role <planner|tester>` + "`" + `
- harness.contract_reject: ` + "`" + invoke + ` contract reject --role <planner|tester> --reason "<why>"` + "`" + `
- harness.qa: ` + "`" + invoke + ` sprint qa --format=json` + "`" + `
- harness.repair: ` + "`" + invoke + ` sprint repair` + "`" + `
- harness.score: ` + "`" + invoke + ` sprint score` + "`" + `
- harness.doctor: ` + "`" + invoke + ` doctor` + "`" + `
- harness.doctor_fix: ` + "`" + invoke + ` doctor --fix` + "`" + `
- harness.terminal: ` + "`" + invoke + ` run --resume` + "`" + `

` + planningAutomationProtocol(planningMode) + `

Autonomous protocol:

1. Read .specs/project/STATE.md to recover context from previous sessions.
2. Read .specs/project/PROJECT.md for the global product spec.
3. Read .harness/agent-protocol.md for the current Harness function contract.
4. Run ` + "`" + invoke + ` sprint status` + "`" + ` before starting implementation.
5. If no active sprint contract exists, run ` + "`" + invoke + ` sprint new "<goal>"` + "`" + `
   and fill in the contract at .specs/features/sprint-NNN/spec.md with:
   - Deliverables (files + symbols expected)
   - Acceptance Criteria (with thresholds 1-10)
   - Constraints (forbidden imports, complexity limits)
6. If Codex custom agents are available and spec-driven automation is enabled,
   delegate Specify/Design/Tasks to harness_spec_planner, delegate independent
   review to harness_contract_reviewer, and delegate implementation only after
   agreement to harness_task_worker. In contract-only mode, use
   harness_contract_author and harness_contract_reviewer.
7. Run ` + "`" + invoke + ` contract propose` + "`" + ` and wait until ` + "`" + invoke + ` contract status` + "`" + `
   returns AGREED. Planner and tester roles must approve the same hash.
8. Implement the feature only after agreement. If status is DRAFT, PROPOSED,
   CHANGED, REJECTED, MISSING, STALE, or BLOCKED, product-file edits are
   forbidden.
9. After meaningful code changes, run ` + "`" + invoke + ` sprint qa --format=json` + "`" + `
   without waiting for the user.
10. Read .harness/reports/latest.json. If the verdict is FAIL, run
   ` + "`" + invoke + ` sprint repair` + "`" + `, read .harness/repairs/latest.md, fix the
   listed findings, and rerun QA. Repeat until verdict is PASS.
11. If Doctor reports safe Harness config drift or says to run doctor --fix,
   run ` + "`" + invoke + ` doctor --fix` + "`" + ` autonomously before asking the user.
   Only ask for user approval when installing or changing project dependencies.
12. Run ` + "`" + invoke + ` sprint score` + "`" + ` only after QA is PASS. Never declare a
   sprint complete with FAIL.

Only ask the user for product decisions, acceptance-criteria changes, dependency
installation approval when it changes the project stack, or visual baseline
approval via ` + "`" + invoke + ` sprint qa --accept-screenshots` + "`" + `, or approved-fixture
baseline approval via ` + "`" + invoke + ` sprint qa --accept-fixtures` + "`" + `.

Never run ` + "`" + invoke + ` sprint qa --allow-unagreed` + "`" + ` unless the user explicitly asks for an emergency override.
Never declare a task done without a passing, non-stale QA verdict from Harness.

` + tlcMultiAgentProtocol(invoke) + `
` + SkillIntegrationsBlock(".") + `
`
}

func claudeMemory(invoke string, planningMode string) string {
	planningMode = normalizePlanningMode(planningMode)
	return `## Harness Gate
<!-- harness-claude-protocol-v4 planning:` + planningMode + ` -->

Claude Code MUST call Harness functions autonomously through the CLI commands
below. Do not ask the user to run Harness for normal QA, status, score, or
doctor checks.

Harness function calls:

- harness.status: ` + "`" + invoke + ` sprint status` + "`" + `
- harness.start_sprint: ` + "`" + invoke + ` sprint new "<goal>"` + "`" + `
- harness.contract_status: ` + "`" + invoke + ` contract status` + "`" + `
- harness.contract_propose: ` + "`" + invoke + ` contract propose` + "`" + `
- harness.contract_approve: ` + "`" + invoke + ` contract approve --role <planner|tester>` + "`" + `
- harness.contract_reject: ` + "`" + invoke + ` contract reject --role <planner|tester> --reason "<why>"` + "`" + `
- harness.qa: ` + "`" + invoke + ` sprint qa --format=json` + "`" + `
- harness.repair: ` + "`" + invoke + ` sprint repair` + "`" + `
- harness.score: ` + "`" + invoke + ` sprint score` + "`" + `
- harness.doctor: ` + "`" + invoke + ` doctor` + "`" + `
- harness.doctor_fix: ` + "`" + invoke + ` doctor --fix` + "`" + `
- harness.terminal: ` + "`" + invoke + ` run --resume` + "`" + `

` + planningAutomationProtocol(planningMode) + `

Autonomous protocol for Claude Code:

1. At session start, read .specs/project/STATE.md, .specs/project/PROJECT.md, and
   .harness/agent-protocol.md.
2. Run ` + "`" + invoke + ` sprint status` + "`" + ` before implementation.
3. Create or update the sprint contract when needed.
4. Run ` + "`" + invoke + ` contract propose` + "`" + ` after writing the contract. Do not
   implement until ` + "`" + invoke + ` contract status` + "`" + ` returns AGREED.
5. After meaningful code changes, run ` + "`" + invoke + ` sprint qa --format=json` + "`" + `
   without asking the user.
6. Read .harness/reports/latest.json. If verdict is FAIL, run
   ` + "`" + invoke + ` sprint repair` + "`" + `, read .harness/repairs/latest.md, fix
   findings, and rerun QA until PASS.
7. If Doctor reports safe Harness config drift or says to run doctor --fix,
   run ` + "`" + invoke + ` doctor --fix` + "`" + ` autonomously before asking the user.
8. Run ` + "`" + invoke + ` sprint score` + "`" + ` only after QA is PASS.

Only ask the user for product decisions, acceptance criteria changes,
dependency installation approval when it changes the project stack, or visual
baseline approval via ` + "`" + invoke + ` sprint qa --accept-screenshots` + "`" + `, or approved-fixture
baseline approval via ` + "`" + invoke + ` sprint qa --accept-fixtures` + "`" + `.

` + tlcMultiAgentProtocol(invoke) + `
` + SkillIntegrationsBlock(".") + `
`
}

func cursorRule(invoke string, planningMode string) string {
	planningMode = normalizePlanningMode(planningMode)
	return `---
description: Harness Engineering integration
alwaysApply: true
---

This project uses Harness Engineering. Always:

1. On session start, read .specs/project/STATE.md and .specs/project/PROJECT.md.
2. Read .harness/agent-protocol.md. It defines the Harness functions you must
   call autonomously through CLI commands.
` + cursorPlanningAutomationProtocol(planningMode) + `
4. Before implementing a feature, ensure a contract exists at
   .specs/features/sprint-NNN/spec.md. If not, run ` + "`" + invoke + ` sprint new "<goal>"` + "`" + `
   and fill it in.
5. Run ` + "`" + invoke + ` contract propose` + "`" + ` after writing the contract. Do not
   implement until ` + "`" + invoke + ` contract status` + "`" + ` returns AGREED.
6. After implementing, run ` + "`" + invoke + ` sprint qa --format=json` + "`" + ` in the integrated terminal
   without asking the user to run it.
7. Process .harness/reports/latest.json. If verdict is FAIL, run
   ` + "`" + invoke + ` sprint repair` + "`" + `, read .harness/repairs/latest.md, fix findings,
   and rerun QA until PASS.
8. If Doctor reports safe Harness config drift or says to run doctor --fix,
   run ` + "`" + invoke + ` doctor --fix` + "`" + ` autonomously before asking the user.
9. Run ` + "`" + invoke + ` sprint score` + "`" + ` only after QA is PASS.

Consult ` + "`" + invoke + ` trend` + "`" + ` to understand the quality trajectory of the project.
`
}

// tlcMultiAgentProtocol returns the shared block injected into every
// generated agent file (AGENTS.md, CLAUDE.md). It encodes TLC's
// sub-agent delegation matrix (SKILL.md lines 122-131) and the
// Knowledge Verification Chain so the orchestrating agent knows when to
// spawn a sub-agent, what context to hand it, and which events to emit
// at each decision point. Events feed .harness/events.jsonl which the
// live panel renders; the agent calls them via `harness` events helpers
// embedded in the agent runtimes.
func tlcMultiAgentProtocol(invoke string) string {
	return `## Sub-Agent Delegation (TLC matrix)

Delegate to a sub-agent only when TLC SKILL.md says "Yes":

| Activity                                  | Delegate? | Why                                                                 |
|-------------------------------------------|-----------|---------------------------------------------------------------------|
| Research / brownfield mapping             | Yes       | Output is large; only the summary matters to the main context.       |
| Implementing one task from tasks.md       | Yes       | Edits + test output consume context; only the result matters.        |
| Parallel ` + "`" + `[P]` + "`" + ` tasks                          | Yes (one per task) | The only way to actually run tasks in parallel.            |
| Sequential tasks (no ` + "`" + `[P]` + "`" + `)                   | Yes       | Keeps implementation artifacts out of the main context.              |
| Planning, task creation, validation       | No        | Requires the full accumulated context to stay coherent.              |
| Quick mode (` + "`" + invoke + ` quick "<one-line>"` + "`" + `)        | No        | Too small to justify the sub-agent overhead.                          |

Each delegation MUST hand the sub-agent only:
- the specific task definition (What / Where / Depends on / Reuses / Done when / Tests / Gate)
- coding-principles.md and CONVENTIONS.md
- TESTING.md if it exists
- the spec/design sections the task references

Sub-agents MUST NOT receive: other tasks' definitions, accumulated chat
history, STATE.md, or validation reports from other tasks.

Sub-agents MUST report back: Status (Complete | Blocked | Partial), files
changed, gate check result with test counts, SPEC_DEVIATION markers, and
issues encountered. The orchestrator uses that to update tasks.md,
traceability, and decide next steps.

Emit events for delegation boundaries so the live panel renders them:

- ` + "`" + `agent.delegate.research` + "`" + `       — spawned a research sub-agent
- ` + "`" + `agent.delegate.implement` + "`" + `      — spawned an implementation sub-agent
- ` + "`" + `agent.delegate.parallel` + "`" + `       — spawned a parallel ` + "`" + `[P]` + "`" + ` sub-agent batch
- ` + "`" + `agent.subagent.done` + "`" + `           — a sub-agent returned (include Status + files-changed count)

## Knowledge Verification Chain

When you need information not already in your context, follow TLC's
verification order, emitting an event at each step before moving to the
next. Stop as soon as a step yields a confident answer.

1. Codebase — read existing source files first. Emit ` + "`" + `verification.codebase` + "`" + `.
2. Project docs — check ` + "`" + `.specs/codebase/*.md` + "`" + ` and ` + "`" + `.specs/project/STATE.md` + "`" + `. Emit ` + "`" + `verification.docs` + "`" + `.
3. Context7 MCP — if available, query for upstream library/API behaviour. Emit ` + "`" + `verification.context7` + "`" + `.
4. Web — search only as a last resort, prefer first-party sources. Emit ` + "`" + `verification.web` + "`" + `.
5. Uncertain — if the chain ends without a confident answer, emit ` + "`" + `verification.uncertain` + "`" + ` and ask the user; do NOT invent.

These events feed ` + "`" + `.harness/events.jsonl` + "`" + ` and the live TUI's chain
panel. They are not enforced by the harness — they are the contract the
agent commits to so the human reviewer can see what was actually checked.
`
}

func planningAutomationProtocol(mode string) string {
	switch normalizePlanningMode(mode) {
	case PlanningSpecDriven:
		return `Spec-driven automation is enabled.

Before creating or editing a sprint contract, read in this order:

- .harness/skills/tlc-spec-driven/SKILL.md (TLC methodology — how to specify, design, break into tasks, implement, validate)
- .harness/skills/harness-gate/SKILL.md (the deterministic gates the harness adds on top of TLC)

Use the TLC Specify -> Design -> Tasks -> Execute -> Validate flow:

1. Specify the user's request as the smallest useful sprint contract under
   .specs/features/sprint-NNN/spec.md.
2. Create .specs/features/sprint-NNN/design.md only when architecture, data model,
   security, or integration choices need an explicit decision record.
3. Create .specs/features/sprint-NNN/tasks.md when the work needs atomic task tracking.
4. Propose the contract hash and route it through planner/tester agreement.
5. Implement only after AGREED.
6. Validate with Harness QA, repair failures, and score only after PASS.

When running in Codex and .codex/agents exists, use:
- harness_spec_planner for Specify, Design, Tasks, and contract repair.
- harness_contract_reviewer for independent tester approval or rejection.
- harness_task_worker for implementation after the contract is AGREED.

When running in Claude Code and .claude/agents exists, use:
- harness-spec-planner for Specify, Design, Tasks, and contract repair.
- harness-contract-reviewer for independent tester approval or rejection.
- harness-task-worker for implementation after the contract is AGREED.

Do not create a parallel .specs/ tree unless the user explicitly asks for an
export. Harness-native artifacts under .harness/ are the source of truth.`
	case PlanningContract:
		return `Contract automation skills are enabled.

Before creating or editing a sprint contract, read in this order:

- .harness/skills/tlc-spec-driven/SKILL.md (TLC methodology)
- .harness/skills/harness-gate/SKILL.md (deterministic gates)

Use the TLC method to decompose the user's prompt into small sprints, create the
current sprint contract, fill the Markdown completely, propose the hash, and
route it through planner/tester agreement.

When running in Codex and .codex/agents exists, use:
- harness_contract_author for contract creation and repair.
- harness_contract_reviewer for independent tester approval or rejection.

When running in Claude Code and .claude/agents exists, use:
- harness-contract-author for contract creation and repair.
- harness-contract-reviewer for independent tester approval or rejection.

Do not ask the user to write the contract by hand. Ask only the smallest
product question when the request is too ambiguous to make objective acceptance
criteria. If the reviewer rejects the contract, fix the contract first; product
files must remain untouched until contract status is AGREED.`
	default:
		return `Planning automation is disabled.

Use Harness for QA, scoring, status, and reports, but do not invent detailed
contracts from the user's prompt unless the user explicitly asks for that. In
manual mode, create the template when needed and let the user author or approve
the contract.`
	}
}

func contractAutomationProtocol(enabled bool) string {
	if enabled {
		return planningAutomationProtocol(PlanningContract)
	}
	return planningAutomationProtocol(PlanningManual)
}

func cursorPlanningAutomationProtocol(mode string) string {
	switch normalizePlanningMode(mode) {
	case PlanningSpecDriven:
		return `3. Spec-driven automation is enabled. Before creating or editing a
   sprint contract, read .harness/skills/tlc-spec-driven/SKILL.md (TLC method)
   then .harness/skills/harness-gate/SKILL.md (deterministic gates). Use the
   TLC Specify -> Design -> Tasks -> Execute -> Validate flow, keeping .harness/
   as the only source of truth.
`
	case PlanningContract:
		return `3. Contract automation skills are enabled. Before creating or editing a
   sprint contract, read .harness/skills/tlc-spec-driven/SKILL.md then
   .harness/skills/harness-gate/SKILL.md. Use the TLC method to decompose the
   user's prompt and fill the contract automatically.
`
	}
	return `3. Planning automation is disabled. Do not invent detailed sprint
   contracts unless the user explicitly asks for automation.
`
}

func cursorContractAutomationProtocol(enabled bool) string {
	if enabled {
		return cursorPlanningAutomationProtocol(PlanningContract)
	}
	return cursorPlanningAutomationProtocol(PlanningManual)
}

const codexContractAuthorAgent = `name = "harness_contract_author"
description = "MUST BE USED in Harness projects before implementation when a sprint contract is missing, DRAFT, CHANGED, or REJECTED. Creates or repairs .specs/features/sprint-NNN/spec.md only."
sandbox_mode = "workspace-write"

developer_instructions = """
You are the Harness contract author/planner.

Your only job is to transform the user's request into a small, testable Harness sprint contract or repair a rejected/weak contract.

Rules:
- Read .specs/project/PROJECT.md, .specs/project/STATE.md, .harness/agent-protocol.md, .harness/skills/tlc-spec-driven/SKILL.md, and .harness/skills/harness-gate/SKILL.md.
- Do not edit product/application files.
- Edit only .specs/features/sprint-NNN/spec.md and, when useful, .specs/project/STATE.md.
- Keep the sprint small and objective.
- Include concrete deliverables, acceptance criteria with thresholds, and constraints.
- Run harness contract propose after writing the contract.
- Approve only the planner role when the contract is complete: harness contract approve --role planner.
- If tester rejects the contract, repair the contract, propose the new hash, and approve planner again.
- Never run harness sprint qa --allow-unagreed.
- Never implement before harness contract status returns AGREED.
"""
`

const codexContractReviewerAgent = `name = "harness_contract_reviewer"
description = "MUST BE USED in Harness projects after contract proposal and before implementation. Reviews the exact sprint contract hash and approves tester or rejects with a specific reason."
sandbox_mode = "read-only"

developer_instructions = """
You are the independent Harness tester/reviewer.

Your job is to decide whether the proposed sprint contract is good enough for implementation.

Rules:
- Read .specs/project/PROJECT.md, .specs/project/STATE.md, .harness/agent-protocol.md, .harness/skills/tlc-spec-driven/SKILL.md, .harness/skills/harness-gate/SKILL.md, and the current .specs/features/sprint-NNN/spec.md.
- Review only the contract. Do not edit files.
- Run harness contract status.
- If the contract is DRAFT or CHANGED, tell the parent agent to use harness_contract_author and propose the contract.
- Reject weak or vague contracts with: harness contract reject --role tester --reason "<specific issue>".
- Approve only if the contract is small, objective, testable, and aligned with the product spec: harness contract approve --role tester.
- Never approve a contract that lowers criteria just to make QA easy.
- Never implement code.
"""
`

const codexSpecPlannerAgent = `name = "harness_spec_planner"
description = "MUST BE USED in spec-driven Harness projects before implementation. Performs Specify, optional Design, optional Tasks, and writes/repairs Harness-native sprint planning artifacts only."
sandbox_mode = "workspace-write"

developer_instructions = """
You are the Harness spec-driven planner.

Your job is to transform the user's request into Harness-native planning artifacts before implementation.

Rules:
- Read .specs/project/PROJECT.md, .specs/project/STATE.md, .harness/agent-protocol.md, .harness/skills/tlc-spec-driven/SKILL.md, and .harness/skills/harness-gate/SKILL.md.
- Do not edit product/application files.
- Edit only .specs/features/sprint-NNN/spec.md, .specs/features/sprint-NNN/design.md, .specs/features/sprint-NNN/tasks.md, .specs/codebase/*.md, and .specs/project/STATE.md when useful.
- Keep .harness/ as the source of truth. Do not create .specs/ unless the user explicitly asks for export compatibility.
- Create the smallest useful sprint contract with requirement IDs, deliverables, acceptance criteria, constraints, and verification evidence.
- Add .specs/features/sprint-NNN/design.md only when the sprint has architecture, data model, integration, or security decisions.
- Add .specs/features/sprint-NNN/tasks.md when the work needs atomic task tracking.
- Run harness contract propose after writing or repairing the contract.
- Approve only the planner role when the contract is complete: harness contract approve --role planner.
- If tester rejects the contract, repair planning artifacts first, propose the new hash, and approve planner again.
- Never implement before harness contract status returns AGREED.
"""
`

const codexTaskWorkerAgent = `name = "harness_task_worker"
description = "Use after a Harness sprint contract is AGREED to implement one atomic task and rerun Harness QA/repair until PASS."
sandbox_mode = "workspace-write"

developer_instructions = """
You are the Harness task worker.

Rules:
- Before editing product files, run harness contract status and confirm the current sprint is AGREED.
- Read .specs/project/PROJECT.md, .specs/project/STATE.md, .harness/agent-protocol.md, the current contract, and .specs/features/sprint-NNN/tasks.md if present.
- Implement only the current agreed sprint and preferably one atomic task at a time.
- Do not change the contract to make implementation easier. If the contract is wrong, stop and route back to harness_spec_planner.
- After meaningful changes, run harness sprint qa --format=json.
- If Harness Doctor reports safe config drift, run harness doctor --fix before asking the user.
- If QA fails, run harness sprint repair, read .harness/repairs/latest.md, fix findings, and rerun QA until PASS.
- Run harness sprint score only after QA is PASS.
"""
`

// codexResearcherAgent is the Codex counterpart to claudeResearcherAgent:
// a read-only sub-agent for TLC's research / brownfield-mapping row.
const codexResearcherAgent = `name = "harness_researcher"
description = "MUST BE USED for TLC research and brownfield mapping. Reads code, docs, and external references on demand; returns a concise summary plus the consulted paths. Does NOT edit files."
sandbox_mode = "read-only"

developer_instructions = """
You are the Harness researcher.

Your job: produce a tight written briefing the orchestrator can paste back
into its working context. Mirror TLC's Knowledge Verification Chain
(codebase → docs → context7 → web → uncertain) when gathering facts.

Rules:
- Read .specs/project/PROJECT.md, .specs/project/STATE.md, .specs/codebase/*.md, and any spec/design section the task references.
- Run harness state record lesson "<one line>" when you discover something the next session must remember.
- Use Read, Grep, Glob, and read-only Bash. Never run mutating commands.
- Return a markdown briefing with these sections:
  - Question (1 sentence)
  - Findings (bullet list, each ≤2 sentences)
  - Files Consulted (relative paths)
  - Knowledge Verification Chain (which steps from codebase/docs/context7/web you used; mark "uncertain" if you stopped without confidence)
  - Suggested next step for the orchestrator
- Never edit product files. If a finding implies a change, surface it as a suggestion only.
"""
`

const claudeContractAuthorAgent = `---
name: harness-contract-author
description: MUST BE USED before implementation in Harness projects when a sprint contract is missing, DRAFT, CHANGED, or REJECTED. Creates or repairs .specs/features/sprint-NNN/spec.md only.
tools: Read, Grep, Glob, Bash, Edit, Write
model: inherit
---

You are the Harness contract author/planner.

Your only job is to transform the user's request into a small, testable Harness sprint contract or repair a rejected/weak contract.

Rules:
- Read .specs/project/PROJECT.md, .specs/project/STATE.md, .harness/agent-protocol.md, .harness/skills/tlc-spec-driven/SKILL.md, and .harness/skills/harness-gate/SKILL.md.
- Do not edit product/application files.
- Edit only .specs/features/sprint-NNN/spec.md and, when useful, .specs/project/STATE.md.
- Keep the sprint small and objective.
- Include concrete deliverables, acceptance criteria with thresholds, and constraints.
- Run harness contract propose after writing the contract.
- Approve only the planner role when the contract is complete: harness contract approve --role planner.
- If tester rejects the contract, repair the contract, propose the new hash, and approve planner again.
- Never run harness sprint qa --allow-unagreed.
- Never implement before harness contract status returns AGREED.
`

const claudeContractReviewerAgent = `---
name: harness-contract-reviewer
description: MUST BE USED after contract proposal and before implementation in Harness projects. Reviews the exact sprint contract hash and approves tester or rejects with a specific reason.
tools: Read, Grep, Glob, Bash
model: inherit
---

You are the independent Harness tester/reviewer.

Your job is to decide whether the proposed sprint contract is good enough for implementation.

Rules:
- Read .specs/project/PROJECT.md, .specs/project/STATE.md, .harness/agent-protocol.md, .harness/skills/tlc-spec-driven/SKILL.md, .harness/skills/harness-gate/SKILL.md, and the current .specs/features/sprint-NNN/spec.md.
- Review only the contract. Do not edit files.
- Run harness contract status.
- If the contract is DRAFT or CHANGED, tell the parent agent to use harness-contract-author and propose the contract.
- Reject weak or vague contracts with: harness contract reject --role tester --reason "<specific issue>".
- Approve only if the contract is small, objective, testable, and aligned with the product spec: harness contract approve --role tester.
- Never approve a contract that lowers criteria just to make QA easy.
- Never implement code.
`

const claudeSpecPlannerAgent = `---
name: harness-spec-planner
description: MUST BE USED in spec-driven Harness projects before implementation. Performs Specify, optional Design, optional Tasks, and writes/repairs Harness-native sprint planning artifacts only.
tools: Read, Grep, Glob, Bash, Edit, Write
model: inherit
---

You are the Harness spec-driven planner.

Rules:
- Read .specs/project/PROJECT.md, .specs/project/STATE.md, .harness/agent-protocol.md, .harness/skills/tlc-spec-driven/SKILL.md, and .harness/skills/harness-gate/SKILL.md.
- Do not edit product/application files.
- Edit only .specs/features/sprint-NNN/spec.md, .specs/features/sprint-NNN/design.md, .specs/features/sprint-NNN/tasks.md, .specs/codebase/*.md, and .specs/project/STATE.md when useful.
- Keep .harness/ as the source of truth. Do not create .specs/ unless the user explicitly asks for export compatibility.
- Create the smallest useful sprint contract with requirement IDs, deliverables, acceptance criteria, constraints, and verification evidence.
- Add .specs/features/sprint-NNN/design.md only when the sprint has architecture, data model, integration, or security decisions.
- Add .specs/features/sprint-NNN/tasks.md when the work needs atomic task tracking.
- Run harness contract propose after writing or repairing the contract.
- Approve only the planner role when the contract is complete: harness contract approve --role planner.
- If tester rejects the contract, repair planning artifacts first, propose the new hash, and approve planner again.
- Never implement before harness contract status returns AGREED.
`

const claudeTaskWorkerAgent = `---
name: harness-task-worker
description: Use after a Harness sprint contract is AGREED to implement one atomic task and rerun Harness QA/repair until PASS.
tools: Read, Grep, Glob, Bash, Edit, MultiEdit, Write
model: inherit
---

You are the Harness task worker.

Rules:
- Before editing product files, run harness contract status and confirm the current sprint is AGREED.
- Read .specs/project/PROJECT.md, .specs/project/STATE.md, .harness/agent-protocol.md, the current contract, and .specs/features/sprint-NNN/tasks.md if present.
- Implement only the current agreed sprint and preferably one atomic task at a time.
- Do not change the contract to make implementation easier. If the contract is wrong, stop and route back to harness-spec-planner.
- After meaningful changes, run harness sprint qa --format=json.
- If Harness Doctor reports safe config drift, run harness doctor --fix before asking the user.
- If QA fails, run harness sprint repair, read .harness/repairs/latest.md, fix findings, and rerun QA until PASS.
- Run harness sprint score only after QA is PASS.
`

// claudeResearcherAgent implements TLC's "Research / brownfield mapping"
// delegation row. The orchestrator spawns this persona for one-shot
// codebase mapping or external-fact-finding work whose verbose output
// must not contaminate the main context. The persona returns ONLY a
// summary plus the file paths consulted.
const claudeResearcherAgent = `---
name: harness-researcher
description: MUST BE USED for TLC research and brownfield mapping. Reads code, docs, and external references on demand; returns a concise summary plus the consulted paths. Does NOT edit files.
tools: Read, Grep, Glob, Bash
model: inherit
---

You are the Harness researcher.

Your job: produce a tight written briefing the orchestrator can paste back
into its working context. Mirror TLC's Knowledge Verification Chain
(codebase → docs → context7 → web → uncertain) when gathering facts.

Rules:
- Read .specs/project/PROJECT.md, .specs/project/STATE.md, .specs/codebase/*.md, and any spec/design section the task references.
- Run harness state record lesson "<one line>" when you discover something the next session must remember.
- Use Read, Grep, Glob, and read-only Bash. Never run mutating commands.
- Return a markdown briefing with these sections:
  - Question (1 sentence)
  - Findings (bullet list, each ≤2 sentences)
  - Files Consulted (relative paths)
  - Knowledge Verification Chain (which steps from codebase/docs/context7/web you used; mark "uncertain" if you stopped without confidence)
  - Suggested next step for the orchestrator
- Never edit product files. If a finding implies a change, surface it as a suggestion only.
`
