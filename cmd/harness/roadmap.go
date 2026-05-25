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

// newRoadmapCmd implements TLC's project ROADMAP per references/roadmap.md.
// The roadmap is a single markdown file at .specs/project/ROADMAP.md that
// lists features in priority order with their status (Pending / In
// Design / Implementing / Verified). The harness ships two minimal verbs:
//
//	harness roadmap                  # prints the roadmap (creating an
//	                                 # empty template if needed)
//	harness roadmap append "<line>"  # appends a "- [ ] <line>" entry
//
// Editing the full structure is the agent's job — the harness only
// guarantees the file exists, the entry-point is consistent, and every
// append emits a roadmap.updated event for the trend / TUI.
func newRoadmapCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "roadmap",
		Short: "View or append to .specs/project/ROADMAP.md (TLC roadmap.md)",
	}
	cmd.AddCommand(newRoadmapAppendCmd())
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return printRoadmap()
	}
	return cmd
}

func newRoadmapAppendCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "append \"<line>\"",
		Short: "Append a new entry to the roadmap (renders as a checklist item)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			line := strings.TrimSpace(strings.Join(args, " "))
			if line == "" {
				return fmt.Errorf("roadmap append requires a non-empty line")
			}
			if err := appendRoadmap(line); err != nil {
				return err
			}
			events.Record(".harness", "roadmap.updated", events.PhaseContract,
				line, "")
			fmt.Printf("✓ Appended to %s\n", roadmapPath())
			return nil
		},
	}
}

func roadmapPath() string {
	return filepath.Join(siblingSpecsRoot(".harness"), "project", "ROADMAP.md")
}

func ensureRoadmap() (string, error) {
	path := roadmapPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.WriteFile(path, []byte(roadmapTemplate), 0o644); err != nil {
			return "", err
		}
	}
	return path, nil
}

func printRoadmap() error {
	path, err := ensureRoadmap()
	if err != nil {
		return err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

func appendRoadmap(line string) error {
	path, err := ensureRoadmap()
	if err != nil {
		return err
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	entry := fmt.Sprintf("- [ ] %s  <!-- added %s -->\n", line, time.Now().UTC().Format(time.RFC3339))
	updated := string(body)
	if !strings.HasSuffix(updated, "\n") {
		updated += "\n"
	}
	updated += entry
	return os.WriteFile(path, []byte(updated), 0o644)
}

const roadmapTemplate = `# Project Roadmap

> TLC ROADMAP per references/roadmap.md. Order entries by priority. Each
> entry should map to a feature spec under .specs/features/<slug>/ once
> it becomes active. Use ` + "`harness roadmap append`" + ` to add items
> without touching this file directly.

## Now
- [ ] <highest-priority feature in flight>

## Next
- [ ] <feature scheduled after Now>

## Later
- [ ] <feature deferred or speculative>
`
