package tui

import (
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var theme = struct {
	Primary lipgloss.Color
	Success lipgloss.Color
	Warning lipgloss.Color
	Danger  lipgloss.Color
	Muted   lipgloss.Color
	Text    lipgloss.Color
	Border  lipgloss.Color
	Panel   lipgloss.Color
	Purple  lipgloss.Color
}{
	Primary: lipgloss.Color("81"),
	Success: lipgloss.Color("114"),
	Warning: lipgloss.Color("215"),
	Danger:  lipgloss.Color("203"),
	Muted:   lipgloss.Color("244"),
	Text:    lipgloss.Color("252"),
	Border:  lipgloss.Color("238"),
	Panel:   lipgloss.Color("236"),
	Purple:  lipgloss.Color("141"),
}

var styles = struct {
	Header      lipgloss.Style
	Brand       lipgloss.Style
	Nav         lipgloss.Style
	NavActive   lipgloss.Style
	Card        lipgloss.Style
	CardTitle   lipgloss.Style
	Muted       lipgloss.Style
	Text        lipgloss.Style
	Primary     lipgloss.Style
	Success     lipgloss.Style
	Warning     lipgloss.Style
	Danger      lipgloss.Style
	Purple      lipgloss.Style
	TableHeader lipgloss.Style
	Footer      lipgloss.Style
	Selected    lipgloss.Style
}{
	Header: lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(theme.Primary).
		Foreground(theme.Text).
		Padding(0, 1),
	Brand: lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("230")).
		Background(lipgloss.Color("238")).
		Padding(0, 2),
	Nav: lipgloss.NewStyle().
		Foreground(theme.Muted),
	NavActive: lipgloss.NewStyle().
		Foreground(theme.Primary).
		Bold(true),
	Card: lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(theme.Border).
		Foreground(theme.Text).
		Background(theme.Panel).
		Padding(0, 1),
	CardTitle: lipgloss.NewStyle().
		Foreground(lipgloss.Color("230")).
		Bold(true),
	Muted:       lipgloss.NewStyle().Foreground(theme.Muted),
	Text:        lipgloss.NewStyle().Foreground(theme.Text),
	Primary:     lipgloss.NewStyle().Foreground(theme.Primary).Bold(true),
	Success:     lipgloss.NewStyle().Foreground(theme.Success).Bold(true),
	Warning:     lipgloss.NewStyle().Foreground(theme.Warning).Bold(true),
	Danger:      lipgloss.NewStyle().Foreground(theme.Danger).Bold(true),
	Purple:      lipgloss.NewStyle().Foreground(theme.Purple).Bold(true),
	TableHeader: lipgloss.NewStyle().Foreground(theme.Muted).Bold(true),
	Footer: lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(theme.Border).
		Foreground(theme.Muted).
		Background(theme.Panel).
		Padding(0, 1),
	Selected: lipgloss.NewStyle().
		Foreground(theme.Primary).
		Bold(true),
}

type symbolSet struct {
	Check string
	Cross string
	Dot   string
	Open  string
	Arrow string
	Bar   string
	Empty string
}

func symbols() symbolSet {
	if useASCII() {
		return symbolSet{
			Check: "OK",
			Cross: "X",
			Dot:   "*",
			Open:  "o",
			Arrow: "->",
			Bar:   "#",
			Empty: "-",
		}
	}
	return symbolSet{
		Check: "✓",
		Cross: "✕",
		Dot:   "●",
		Open:  "○",
		Arrow: "→",
		Bar:   "█",
		Empty: "░",
	}
}

func useASCII() bool {
	if strings.TrimSpace(os.Getenv("HARNESS_ASCII")) == "1" {
		return true
	}
	return strings.EqualFold(os.Getenv("TERM"), "dumb")
}

func statusStyle(status string) lipgloss.Style {
	switch normalizeStatus(status) {
	case "pass", "passed", "done", "agreed", "accepted":
		return styles.Success
	case "fail", "failed", "error", "rejected", "stale", "blocked":
		return styles.Danger
	case "running", "validating", "review", "proposed":
		return styles.Primary
	case "warning", "needs_fix", "changed", "draft":
		return styles.Warning
	default:
		return styles.Muted
	}
}

func statusLabel(status string) string {
	status = strings.TrimSpace(status)
	if status == "" {
		status = "pending"
	}
	return strings.ToUpper(status)
}

func statusBadge(status string) string {
	s := symbols()
	label := statusLabel(status)
	switch normalizeStatus(status) {
	case "pass", "passed", "done", "agreed", "accepted":
		return styles.Success.Render(s.Check + " " + label)
	case "fail", "failed", "error", "rejected", "stale", "blocked":
		return styles.Danger.Render(s.Cross + " " + label)
	case "running", "validating", "review", "proposed":
		return styles.Primary.Render(spinner(0) + " " + label)
	case "warning", "needs_fix", "changed", "draft":
		return styles.Warning.Render(s.Dot + " " + label)
	default:
		return styles.Muted.Render(s.Open + " " + label)
	}
}

func normalizeStatus(status string) string {
	return strings.ToLower(strings.TrimSpace(status))
}

func spinner(frame int) string {
	if useASCII() {
		frames := []string{"|", "/", "-", "\\"}
		return frames[frame%len(frames)]
	}
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	return frames[frame%len(frames)]
}
