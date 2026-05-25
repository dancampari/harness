package harness

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dancampari/harness/internal/events"
	"github.com/spf13/cobra"
)

// newStateCmd implements TLC's state-management.md schema: STATE.md is a
// long-running narrative file with five kinds of structured entries —
// decision, blocker, todo, deferred, lesson. The harness exposes one
// command, `state record`, that appends a stamped entry under the
// appropriate section, creating the file (and section) when missing.
//
// Reading STATE.md is the agent's job at session start; the harness
// only owns the append path so the schema stays consistent.
func newStateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "state",
		Short: "Manage .specs/project/STATE.md (TLC state-management.md)",
	}
	cmd.AddCommand(newStateRecordCmd())
	return cmd
}

func newStateRecordCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "record <decision|blocker|todo|deferred|lesson> \"<message>\"",
		Short: "Append a structured entry to STATE.md under the matching section",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			kind := normalizeStateKind(args[0])
			if kind == "" {
				return fmt.Errorf("unknown state kind %q; use one of: decision, blocker, todo, deferred, lesson", args[0])
			}
			message := strings.TrimSpace(strings.Join(args[1:], " "))
			if message == "" {
				return fmt.Errorf("state record requires a non-empty message")
			}
			path, err := appendStateEntry(kind, message)
			if err != nil {
				return err
			}
			events.Record(".harness", "state.recorded", events.PhaseContract,
				fmt.Sprintf("%s · %s", kind, message), "")
			fmt.Printf("✓ Recorded %s in %s\n", kind, path)
			return nil
		},
	}
}

func normalizeStateKind(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "decision", "decided":
		return "decision"
	case "blocker", "block":
		return "blocker"
	case "todo", "todos":
		return "todo"
	case "deferred", "defer":
		return "deferred"
	case "lesson", "lessons", "learning":
		return "lesson"
	}
	return ""
}

func statePath() string {
	return filepath.Join(siblingSpecsRoot(".harness"), "project", "STATE.md")
}

// appendStateEntry inserts a `- <timestamp> — <message>` line under the
// matching section, creating both file and section when needed. The
// section ordering matches TLC's state-management.md so the file stays
// scannable for the agent at session start.
func appendStateEntry(kind, message string) (string, error) {
	path := statePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	body := ""
	if b, err := os.ReadFile(path); err == nil {
		body = string(b)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if body == "" {
		body = stateTemplate
	}
	heading := stateHeadingFor(kind)
	entry := fmt.Sprintf("- %s — %s\n", time.Now().UTC().Format(time.RFC3339), message)
	if strings.Contains(body, heading) {
		body = insertUnderHeading(body, heading, entry)
	} else {
		if !strings.HasSuffix(body, "\n") {
			body += "\n"
		}
		body += "\n" + heading + "\n" + entry
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func stateHeadingFor(kind string) string {
	switch kind {
	case "decision":
		return "## Decisions"
	case "blocker":
		return "## Blockers"
	case "todo":
		return "## Todos"
	case "deferred":
		return "## Deferred"
	case "lesson":
		return "## Lessons"
	}
	return "## Notes"
}

// insertUnderHeading places entry immediately after the heading line so
// the most recent record stays near the top of its section — matching
// TLC's "scan latest at session start" intent.
func insertUnderHeading(body, heading, entry string) string {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) != heading {
			continue
		}
		updated := append([]string{}, lines[:i+1]...)
		updated = append(updated, strings.TrimRight(entry, "\n"))
		updated = append(updated, lines[i+1:]...)
		return strings.Join(updated, "\n")
	}
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	return body + "\n" + heading + "\n" + entry
}

const stateTemplate = `# Project State

> TLC STATE.md per references/state-management.md. The harness appends
> structured entries via ` + "`harness state record <kind> \"<message>\"`" + `.
> Agents should read this at session start and write decisions, blockers,
> todos, deferred work, and lessons here rather than scattering them
> across chat history.

## Decisions

## Blockers

## Todos

## Deferred

## Lessons
`
