package harness

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/dancampari/harness/internal/agreement"
	"github.com/dancampari/harness/internal/events"
	"github.com/spf13/cobra"
)

type preToolHookInput struct {
	HookEventName string         `json:"hook_event_name"`
	CWD           string         `json:"cwd"`
	ToolName      string         `json:"tool_name"`
	ToolInput     map[string]any `json:"tool_input"`
}

type preToolDecision struct {
	HookSpecificOutput preToolDecisionOutput `json:"hookSpecificOutput"`
}

type preToolDecisionOutput struct {
	HookEventName            string `json:"hookEventName"`
	PermissionDecision       string `json:"permissionDecision"`
	PermissionDecisionReason string `json:"permissionDecisionReason"`
}

func newGuardCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "guard",
		Short:  "Guard agent tool calls against Harness workflow violations",
		Hidden: true,
	}
	cmd.AddCommand(&cobra.Command{
		Use:    "pre-tool",
		Short:  "Claude Code PreToolUse guard",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGuardPreTool(os.Stdin, os.Stdout)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:    "post-tool",
		Short:  "PostToolUse activity recorder",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGuardPostTool(os.Stdin)
		},
	})
	return cmd
}

func runGuardPreTool(input io.Reader, output io.Writer) error {
	var hook preToolHookInput
	if err := json.NewDecoder(input).Decode(&hook); err != nil {
		guardDiagnostic("PreToolUse hook payload could not be decoded: " + err.Error())
		return nil
	}
	tool := strings.TrimSpace(hook.ToolName)
	if tool != "Edit" && tool != "MultiEdit" && tool != "Write" && tool != "apply_patch" {
		return nil
	}
	projectRoot, harnessDir, ok := resolveHarnessDir(hook.CWD)
	if !ok {
		return nil
	}
	targets := hookTargetPaths(hook)
	st, stErr := agreement.NewManager(harnessDir).Status(0)
	agreed := stErr == nil && strings.EqualFold(st.State, "agreed")

	// Record the edit attempt so the harness sees the Build phase
	// happening live. Before agreement the agent is still in the
	// contract loop; after it, the agent is implementing.
	phase := events.PhaseBuild
	if !agreed {
		phase = events.PhaseContract
	}

	if stErr != nil || agreed {
		recordAgentEdit(harnessDir, "agent.edit", phase, targets)
		return nil
	}
	if len(targets) == 0 {
		recordAgentEdit(harnessDir, "agent.edit.blocked", phase, []string{tool})
		return writePreToolDeny(output, fmt.Sprintf(
			"Harness blocked %s because sprint %03d contract is %s and the target path could not be verified. Do not edit product files before planner/tester agreement.",
			tool, st.SprintNumber, strings.ToUpper(st.State),
		))
	}
	for _, target := range targets {
		if !isPreAgreementControlPath(projectRoot, target) {
			recordAgentEdit(harnessDir, "agent.edit.blocked", phase,
				[]string{displayHookPath(projectRoot, target)})
			return denyPreAgreementWrite(output, tool, projectRoot, target, st)
		}
	}
	recordAgentEdit(harnessDir, "agent.edit", phase, targets)
	return nil
}

// recordAgentEdit appends one activity event describing an agent edit.
// Multiple target paths are collapsed into a single message so the
// activity stream stays one line per tool call.
func recordAgentEdit(harnessDir, eventType, phase string, targets []string) {
	msg := strings.Join(targets, ", ")
	if msg == "" {
		msg = "(unknown path)"
	}
	events.Record(harnessDir, eventType, phase, msg, "")
}

// runGuardPostTool records a completed agent tool call (notably Bash
// commands) so the activity log captures what the agent runs, not just
// what it edits. It never blocks: PostToolUse fires after the fact.
func runGuardPostTool(input io.Reader) error {
	var hook preToolHookInput
	if err := json.NewDecoder(input).Decode(&hook); err != nil {
		guardDiagnostic("PostToolUse hook payload could not be decoded: " + err.Error())
		return nil
	}
	_, harnessDir, ok := resolveHarnessDir(hook.CWD)
	if !ok {
		return nil
	}
	tool := strings.TrimSpace(hook.ToolName)
	switch tool {
	case "Bash", "Shell", "shell", "run":
		cmd := hookCommandText(hook)
		if cmd == "" {
			return nil
		}
		events.Record(harnessDir, "agent.bash", events.PhaseBuild, cmd, "")
	}
	return nil
}

// hookCommandText extracts the command string from a Bash-like tool
// payload, trimming it to a single readable line.
func hookCommandText(hook preToolHookInput) string {
	for _, key := range []string{"command", "cmd", "script"} {
		if value, ok := hook.ToolInput[key].(string); ok {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			if idx := strings.IndexByte(value, '\n'); idx >= 0 {
				value = strings.TrimSpace(value[:idx]) + " …"
			}
			if len(value) > 160 {
				value = value[:160] + "…"
			}
			return value
		}
	}
	return ""
}

func denyPreAgreementWrite(output io.Writer, tool, projectRoot, target string, st agreement.Status) error {
	reason := fmt.Sprintf(
		"Harness blocked %s on %s because sprint %03d contract is %s. Do not edit product files before planner/tester agreement. Repair .harness/contracts/sprint-%03d.md, run `harness contract propose`, have planner and tester approve the same hash, then implement. If the tester rejected the contract, call the contract-authoring flow to rewrite it before changing code.",
		tool, displayHookPath(projectRoot, target), st.SprintNumber, strings.ToUpper(st.State), st.SprintNumber,
	)
	return writePreToolDeny(output, reason)
}

func hookTargetPaths(hook preToolHookInput) []string {
	for _, key := range []string{"file_path", "path"} {
		if value, ok := hook.ToolInput[key].(string); ok {
			if value = strings.TrimSpace(value); value != "" {
				return []string{value}
			}
		}
	}
	if value, ok := hook.ToolInput["cmd"].(string); ok {
		return patchTargetPaths(value)
	}
	if value, ok := hook.ToolInput["patch"].(string); ok {
		return patchTargetPaths(value)
	}
	if value, ok := hook.ToolInput["command"].(string); ok {
		return patchTargetPaths(value)
	}
	return nil
}

func patchTargetPaths(patch string) []string {
	var paths []string
	for _, line := range strings.Split(patch, "\n") {
		line = strings.TrimSpace(line)
		for _, prefix := range []string{
			"*** Add File: ",
			"*** Update File: ",
			"*** Delete File: ",
			"*** Move to: ",
		} {
			if strings.HasPrefix(line, prefix) {
				path := strings.TrimSpace(strings.TrimPrefix(line, prefix))
				if path != "" {
					paths = append(paths, path)
				}
			}
		}
	}
	return paths
}

// resolveHarnessDir locates the project root and .harness directory for
// a hook invocation. It tries the hook-provided cwd first, then falls
// back to the process working directory. The fallback matters because a
// coding CLI may report a cwd in a path form Go cannot resolve on this
// OS (for example a Unix-style path on Windows); without the fallback
// the guard would silently do nothing.
func resolveHarnessDir(hookCWD string) (string, string, bool) {
	candidates := []string{hookCWD}
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, cwd)
	}
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		if projectRoot, harnessDir, ok := findHarnessDir(candidate); ok {
			return projectRoot, harnessDir, true
		}
	}
	return "", "", false
}

// guardDiagnostic surfaces a guard failure that would otherwise pass
// unnoticed. A guard that cannot read its input must fail open (never
// block the agent) but must not fail silent: the warning is recorded as
// a guard.warn event so it shows in the TUI activity panel and in
// `harness doctor`. When no .harness project is reachable at all, the
// message goes to stderr, which coding CLIs surface in hook debug logs.
func guardDiagnostic(message string) {
	if _, harnessDir, ok := resolveHarnessDir(""); ok {
		events.Record(harnessDir, "guard.warn", "", message, "")
		return
	}
	fmt.Fprintln(os.Stderr, "harness guard: "+message)
}

func findHarnessDir(start string) (string, string, bool) {
	if start == "" {
		return "", "", false
	}
	abs, err := filepath.Abs(start)
	if err != nil {
		return "", "", false
	}
	info, err := os.Stat(abs)
	if err == nil && !info.IsDir() {
		abs = filepath.Dir(abs)
	}
	for {
		harnessDir := filepath.Join(abs, ".harness")
		if _, err := os.Stat(harnessDir); err == nil {
			return abs, harnessDir, true
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", "", false
		}
		abs = parent
	}
}

func isPreAgreementControlPath(projectRoot, target string) bool {
	rel := normalizedRel(projectRoot, target)
	if rel == "" {
		return false
	}
	switch rel {
	case "AGENTS.md", "CLAUDE.md":
		return true
	case ".harness/spec.md", ".harness/progress.md", ".harness/agent-protocol.md", ".harness/setup.json":
		return true
	}
	if strings.HasPrefix(rel, ".harness/contracts/sprint-") && strings.HasSuffix(rel, ".md") {
		return true
	}
	if strings.HasPrefix(rel, ".harness/skills/") ||
		strings.HasPrefix(rel, ".claude/") ||
		strings.HasPrefix(rel, ".codex/") ||
		strings.HasPrefix(rel, ".cursor/rules/") {
		return true
	}
	return false
}

func normalizedRel(projectRoot, target string) string {
	if target == "" {
		return ""
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(projectRoot, target)
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return ""
	}
	rel, err := filepath.Rel(projectRoot, absTarget)
	if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return ""
	}
	return filepath.ToSlash(rel)
}

func displayHookPath(projectRoot, target string) string {
	if rel := normalizedRel(projectRoot, target); rel != "" {
		return rel
	}
	return target
}

func writePreToolDeny(output io.Writer, reason string) error {
	decision := preToolDecision{
		HookSpecificOutput: preToolDecisionOutput{
			HookEventName:            "PreToolUse",
			PermissionDecision:       "deny",
			PermissionDecisionReason: reason,
		},
	}
	return json.NewEncoder(output).Encode(decision)
}
