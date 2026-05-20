package tui

import (
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var theme = struct {
	Cyan   lipgloss.Color
	Green  lipgloss.Color
	Red    lipgloss.Color
	Yellow lipgloss.Color
	Fg     lipgloss.Color
	Dim    lipgloss.Color
	Muted  lipgloss.Color
}{
	Cyan:   lipgloss.Color("81"),
	Green:  lipgloss.Color("78"),
	Red:    lipgloss.Color("203"),
	Yellow: lipgloss.Color("179"),
	Fg:     lipgloss.Color("252"),
	Dim:    lipgloss.Color("244"),
	Muted:  lipgloss.Color("238"),
}

// styles holds every text style used by the TUI. No backgrounds, no borders —
// the design rule is: one filled background per screen (the selection row),
// hierarchy comes from color and weight, never from boxes.
var styles = struct {
	Brand       lipgloss.Style
	Text        lipgloss.Style
	Muted       lipgloss.Style
	Faint       lipgloss.Style
	Primary     lipgloss.Style
	Success     lipgloss.Style
	Warning     lipgloss.Style
	Danger      lipgloss.Style
	Section     lipgloss.Style
	Rule        lipgloss.Style
	TabActive   lipgloss.Style
	TabIdle     lipgloss.Style
	TabNumOn    lipgloss.Style
	TabNumOff   lipgloss.Style
	KeyHint     lipgloss.Style
	TableHeader lipgloss.Style
	Selected    lipgloss.Style
	SelectMark  lipgloss.Style
}{
	// Tool mark: "harness" in cyan + bold (the only place we use bold).
	Brand: lipgloss.NewStyle().Foreground(theme.Cyan).Bold(true),
	// Primary text and values.
	Text: lipgloss.NewStyle().Foreground(theme.Fg),
	// Labels, captions, metadata, ─ separators.
	Muted: lipgloss.NewStyle().Foreground(theme.Dim),
	// Skipped / disabled / idle.
	Faint: lipgloss.NewStyle().Foreground(theme.Muted),
	// Cyan accent for titles / key hints / highlights.
	Primary: lipgloss.NewStyle().Foreground(theme.Cyan),
	// Semantic colors — applied only to glyph + label, never whole rows.
	Success: lipgloss.NewStyle().Foreground(theme.Green),
	Warning: lipgloss.NewStyle().Foreground(theme.Yellow),
	Danger:  lipgloss.NewStyle().Foreground(theme.Red),
	// Section header text — fg, regular weight; followed by a ─ rule.
	Section: lipgloss.NewStyle().Foreground(theme.Fg),
	Rule:    lipgloss.NewStyle().Foreground(theme.Dim),
	// Tabs: active is cyan + underline; idle is dim.
	TabActive: lipgloss.NewStyle().Foreground(theme.Cyan).Underline(true),
	TabIdle:   lipgloss.NewStyle().Foreground(theme.Dim),
	TabNumOn:  lipgloss.NewStyle().Foreground(theme.Cyan),
	TabNumOff: lipgloss.NewStyle().Foreground(theme.Muted),
	// Key hint in footer.
	KeyHint: lipgloss.NewStyle().Foreground(theme.Cyan),
	// Table header row: dim labels, regular weight.
	TableHeader: lipgloss.NewStyle().Foreground(theme.Dim),
	// Selection: the one allowed filled background — a near-black tint that
	// reads as "highlighted row" but doesn't shout. We do NOT colorize the
	// text inside; the row keeps its semantic glyphs.
	Selected:   lipgloss.NewStyle().Background(lipgloss.Color("236")),
	SelectMark: lipgloss.NewStyle().Foreground(theme.Cyan),
}

type symbolSet struct {
	Check string
	Cross string
	Dot   string
	Open  string
	Skip  string
	Bang  string
	Arrow string
	Bar   string
	Empty string
	Mark  string
	Ell   string
}

func symbols() symbolSet {
	if useASCII() {
		return symbolSet{
			Check: "v",
			Cross: "x",
			Dot:   "*",
			Open:  "o",
			Skip:  ".",
			Bang:  "!",
			Arrow: "->",
			Bar:   "#",
			Empty: "-",
			Mark:  ">",
			Ell:   "...",
		}
	}
	return symbolSet{
		Check: "✓",
		Cross: "✕",
		Dot:   "●",
		Open:  "○",
		Skip:  "·",
		Bang:  "!",
		Arrow: "→",
		Bar:   "█",
		Empty: "░",
		Mark:  "›",
		Ell:   "…",
	}
}

func useASCII() bool {
	if strings.TrimSpace(os.Getenv("HARNESS_ASCII")) == "1" {
		return true
	}
	return strings.EqualFold(os.Getenv("TERM"), "dumb")
}

// statusStyle returns the colour for a semantic status. Apply only to the
// glyph + label, never to the surrounding line.
func statusStyle(status string) lipgloss.Style {
	switch normalizeStatus(status) {
	case "pass", "passed", "done", "agreed", "accepted":
		return styles.Success
	case "fail", "failed", "error", "rejected", "stale", "blocked":
		return styles.Danger
	case "warning", "warn", "needs_fix", "changed", "draft":
		return styles.Warning
	case "running", "validating", "review", "proposed":
		return styles.Warning
	case "skip", "skipped":
		return styles.Faint
	default:
		return styles.Faint
	}
}

func statusLabel(status string) string {
	status = strings.TrimSpace(status)
	if status == "" {
		status = "pending"
	}
	return strings.ToUpper(status)
}

// statusGlyph returns the design-mandated glyph for a status.
func statusGlyph(status string) string {
	s := symbols()
	switch normalizeStatus(status) {
	case "pass", "passed", "done", "agreed", "accepted":
		return s.Check
	case "fail", "failed", "error", "rejected", "stale", "blocked":
		return s.Cross
	case "warning", "warn", "needs_fix", "changed", "draft":
		return s.Bang
	case "running", "validating", "review", "proposed":
		return s.Dot
	case "skip", "skipped":
		return s.Skip
	default:
		return s.Open
	}
}

// statusBadge renders "glyph LABEL", colored together.
func statusBadge(status string) string {
	return statusStyle(status).Render(statusGlyph(status) + " " + statusLabel(status))
}

func normalizeStatus(status string) string {
	return strings.ToLower(strings.TrimSpace(status))
}

func spinner(frame int) string {
	if useASCII() {
		frames := []string{"|", "/", "-", "\\"}
		return frames[frame%len(frames)]
	}
	frames := []string{"◐", "◓", "◑", "◒"}
	return frames[frame%len(frames)]
}
