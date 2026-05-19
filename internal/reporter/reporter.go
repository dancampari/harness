// Package reporter formats trends, findings, and summary views for the
// terminal. The TUI lives in /internal/tui; this package is for one-shot
// prints from commands like `harness trend` and `harness explain`.
package reporter

import (
	"fmt"
	"io"

	"github.com/charmbracelet/lipgloss"
	"github.com/dancampari/harness/internal/memory"
)

// PrintTrend renders a score trend over recent runs. Includes a small
// ASCII sparkline so the trend is visible at a glance — the central
// answer to problem 6 of the video (silent quality decay).
func PrintTrend(w io.Writer, runs []memory.Run) error {
	if len(runs) == 0 {
		fmt.Fprintln(w, "No runs recorded yet.")
		return nil
	}
	// Reverse so oldest is left, newest is right.
	scores := make([]int, len(runs))
	for i, r := range runs {
		scores[len(runs)-1-i] = r.ScoreTotal
	}

	headerStyle := lipgloss.NewStyle().Bold(true)
	fmt.Fprintln(w, headerStyle.Render("Score trend (oldest → newest)"))
	fmt.Fprintln(w)
	fmt.Fprintln(w, sparkline(scores))
	fmt.Fprintln(w)

	// Table
	fmt.Fprintln(w, "  Sprint   Date          Score   Verdict")
	fmt.Fprintln(w, "  ──────   ──────────    ─────   ───────")
	for i := len(runs) - 1; i >= 0; i-- {
		r := runs[i]
		fmt.Fprintf(w, "  %3d      %s    %3d     %s\n",
			r.SprintNumber,
			r.Timestamp.Format("2006-01-02"),
			r.ScoreTotal,
			r.Verdict)
	}

	// Delta vs last
	if len(scores) >= 2 {
		delta := scores[len(scores)-1] - scores[len(scores)-2]
		mark := "▲"
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
		if delta < 0 {
			mark = "▼"
			style = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
		} else if delta == 0 {
			mark = "▬"
			style = lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
		}
		fmt.Fprintf(w, "\n  Latest vs previous: %s\n",
			style.Render(fmt.Sprintf("%s %+d", mark, delta)))
	}
	return nil
}

// sparkline renders a score series using unicode block characters.
func sparkline(values []int) string {
	if len(values) == 0 {
		return ""
	}
	blocks := []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}
	out := make([]rune, len(values))
	for i, v := range values {
		idx := v * (len(blocks) - 1) / 100
		if idx < 0 {
			idx = 0
		}
		if idx >= len(blocks) {
			idx = len(blocks) - 1
		}
		out[i] = blocks[idx]
	}
	return "  " + string(out)
}

// PrintFinding renders one finding with recurrence metadata. Surfaces
// "this same issue has appeared in N runs since Y" — the core mechanism
// that makes silent decay visible.
func PrintFinding(w io.Writer, f memory.Finding) error {
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	fmt.Fprintln(w, header.Render(fmt.Sprintf("Finding %s", f.ID)))
	fmt.Fprintf(w, "  Dimension: %s\n", f.Dimension)
	fmt.Fprintf(w, "  Severity:  %s\n", f.Severity)
	fmt.Fprintf(w, "  Location:  %s:%d\n", f.File, f.Line)
	fmt.Fprintf(w, "  Rule:      %s\n", f.Rule)
	fmt.Fprintf(w, "  Message:   %s\n", f.Message)
	fmt.Fprintln(w)
	if f.Recurrence > 1 {
		warn := lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
		fmt.Fprintln(w, warn.Render(
			fmt.Sprintf("  ⚠ This fingerprint has appeared in %d runs",
				f.Recurrence)))
		if !f.FirstSeen.IsZero() {
			fmt.Fprintf(w, "    First seen: %s\n", f.FirstSeen.Format("2006-01-02"))
		}
		fmt.Fprintln(w, "    Consider documenting a decision or refactoring the source.")
	}
	return nil
}
