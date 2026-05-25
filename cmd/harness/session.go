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

// newSessionCmd implements TLC's session-handoff.md: a "pause" verb that
// writes a HANDOFF.md note capturing where the agent left off, and a
// "resume" verb that prints that note (and removes it if --clear is
// passed) so the next session picks up cleanly.
//
// Both verbs are intentionally minimal — the structure of HANDOFF.md is
// the agent's responsibility per TLC. The harness only owns the file
// lifecycle and the events that surface session boundaries on the live
// panel and trend.
func newSessionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "session",
		Short: "Pause and resume sessions (TLC session-handoff.md)",
	}
	cmd.AddCommand(newSessionPauseCmd(), newSessionResumeCmd())
	return cmd
}

func newSessionPauseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pause [\"<note>\"]",
		Short: "Write a handoff note describing where the current session stopped",
		RunE: func(cmd *cobra.Command, args []string) error {
			note := strings.TrimSpace(strings.Join(args, " "))
			path, err := writeHandoff(note)
			if err != nil {
				return err
			}
			events.Record(".harness", "session.paused", events.PhaseContract,
				note, "")
			fmt.Printf("✓ Session paused. Resume note at %s\n", path)
			return nil
		},
	}
}

func newSessionResumeCmd() *cobra.Command {
	var clear bool
	cmd := &cobra.Command{
		Use:   "resume",
		Short: "Print the most recent handoff note (use --clear to consume it)",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := handoffPath()
			b, err := os.ReadFile(path)
			if err != nil {
				if os.IsNotExist(err) {
					fmt.Println("No handoff note found. Run `harness session pause \"<note>\"` to create one.")
					return nil
				}
				return err
			}
			fmt.Println(string(b))
			events.Record(".harness", "session.resumed", events.PhaseContract,
				"", "")
			if clear {
				if err := os.Remove(path); err != nil {
					return err
				}
				fmt.Printf("\n✓ Cleared %s\n", path)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&clear, "clear", false, "remove the handoff note after printing it")
	return cmd
}

func handoffPath() string {
	return filepath.Join(siblingSpecsRoot(".harness"), "project", "HANDOFF.md")
}

// writeHandoff writes (or appends to) HANDOFF.md. When the file already
// has a previous note, the new note is appended under a fresh timestamp
// so the agent sees the full chain on resume. The pre-existing note is
// kept in case the agent wants to consult earlier context.
func writeHandoff(note string) (string, error) {
	path := handoffPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	entry := fmt.Sprintf("## %s\n", time.Now().UTC().Format(time.RFC3339))
	if note == "" {
		entry += "<no note supplied — describe where you left off, what is next, and any blockers>\n"
	} else {
		entry += note + "\n"
	}
	existing := ""
	if b, err := os.ReadFile(path); err == nil {
		existing = strings.TrimSpace(string(b))
	} else if !os.IsNotExist(err) {
		return "", err
	}
	header := "# Session Handoff\n\n"
	if existing == "" {
		existing = header
	} else if !strings.HasPrefix(existing, "# Session Handoff") {
		existing = header + existing + "\n"
	} else {
		existing += "\n\n"
	}
	body := existing + entry
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
