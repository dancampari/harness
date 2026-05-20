package harness

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/dancampari/harness/internal/agreement"
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
	return cmd
}

func runGuardPreTool(input io.Reader, output io.Writer) error {
	var hook preToolHookInput
	if err := json.NewDecoder(input).Decode(&hook); err != nil {
		if err == io.EOF {
			return nil
		}
		return nil
	}
	tool := strings.TrimSpace(hook.ToolName)
	if tool != "Edit" && tool != "MultiEdit" && tool != "Write" && tool != "apply_patch" {
		return nil
	}
	start := hook.CWD
	if start == "" {
		if cwd, err := os.Getwd(); err == nil {
			start = cwd
		}
	}
	projectRoot, harnessDir, ok := findHarnessDir(start)
	if !ok {
		return nil
	}
	st, err := agreement.NewManager(harnessDir).Status(0)
	if err != nil || strings.EqualFold(st.State, "agreed") {
		return nil
	}
	targets := hookTargetPaths(hook)
	if len(targets) == 0 {
		return writePreToolDeny(output, fmt.Sprintf(
			"Harness blocked %s because sprint %03d contract is %s and the target path could not be verified. Do not edit product files before planner/tester agreement.",
			tool, st.SprintNumber, strings.ToUpper(st.State),
		))
	}
	for _, target := range targets {
		if !isPreAgreementControlPath(projectRoot, target) {
			return denyPreAgreementWrite(output, tool, projectRoot, target, st)
		}
	}
	return nil
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
