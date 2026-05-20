package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

type screenMode int

const (
	modeTiny screenMode = iota
	modeCompact
	modeMedium
	modeFull
)

func modeFor(width, height int) screenMode {
	if width < 70 || (height > 0 && height < 18) {
		return modeTiny
	}
	if width < 90 {
		return modeCompact
	}
	if width < 120 {
		return modeMedium
	}
	return modeFull
}

func (m *model) contentWidth() int {
	if m.width <= 0 {
		return 118
	}
	return maxInt(40, m.width-2)
}

func (m *model) availableBodyHeight() int {
	if m.height <= 0 {
		return 28
	}
	return maxInt(6, m.height-5)
}

func card(title string, width int, body string) string {
	width = maxInt(20, width)
	contentWidth := maxInt(1, width-4)
	title = styles.CardTitle.Render(" " + strings.ToUpper(title) + " ")
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	for i, line := range lines {
		lines[i] = fitPlainLine(line, contentWidth)
	}
	return styles.Card.Width(width).Render(title + "\n" + strings.Join(lines, "\n"))
}

func emptyState(width int, message, command string) string {
	lines := []string{styles.Muted.Render(message)}
	if command != "" {
		lines = append(lines, styles.Primary.Render(command))
	}
	return fitBlock(strings.Join(lines, "\n"), width)
}

func fitBlock(s string, width int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, line := range lines {
		lines[i] = fitPlainLine(line, width)
	}
	return strings.Join(lines, "\n")
}

func fitPlainLine(line string, width int) string {
	if width <= 0 {
		return ""
	}
	visible := lipgloss.Width(line)
	if visible <= width {
		return line
	}
	return truncate(stripANSI(line), width)
}

func (m *model) fitToScreen(view string) string {
	if m.width <= 0 || m.height <= 0 {
		return view
	}
	width := maxInt(1, m.width)
	height := maxInt(1, m.height)
	lines := strings.Split(view, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for i, line := range lines {
		visible := lipgloss.Width(line)
		if visible > width {
			lines[i] = truncate(stripANSI(line), width)
			visible = lipgloss.Width(lines[i])
		}
		if visible < width {
			lines[i] = line + strings.Repeat(" ", width-visible)
		}
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	return strings.Join(lines, "\n")
}

func stripANSI(s string) string {
	var out strings.Builder
	inEsc := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == 0x1b {
			inEsc = true
			continue
		}
		if inEsc {
			if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') {
				inEsc = false
			}
			continue
		}
		out.WriteByte(ch)
	}
	return out.String()
}

func row(cols ...column) string {
	var sb strings.Builder
	for i, col := range cols {
		if i > 0 {
			sb.WriteString("  ")
		}
		value := truncate(col.Value, col.Width)
		sb.WriteString(value)
		if pad := col.Width - runeLen(value); pad > 0 {
			sb.WriteString(strings.Repeat(" ", pad))
		}
	}
	return sb.String()
}

type column struct {
	Value string
	Width int
}

type styledColumn struct {
	Value  string
	Width  int
	Style  lipgloss.Style
	Styled bool
}

func styledRow(cols ...styledColumn) string {
	var sb strings.Builder
	for i, col := range cols {
		if i > 0 {
			sb.WriteString("  ")
		}
		value := truncate(col.Value, col.Width)
		if col.Styled {
			sb.WriteString(col.Style.Render(value))
		} else {
			sb.WriteString(value)
		}
		if pad := col.Width - runeLen(value); pad > 0 {
			sb.WriteString(strings.Repeat(" ", pad))
		}
	}
	return sb.String()
}

func kv(label, value string, width int) string {
	labelWidth := 9
	return truncate(fmt.Sprintf("%-*s: %s", labelWidth, label, value), width)
}

func progressBar(score, width int) string {
	width = maxInt(4, width)
	score = minInt(100, maxInt(0, score))
	filled := int(float64(width) * float64(score) / 100.0)
	if score > 0 && filled == 0 {
		filled = 1
	}
	s := symbols()
	bar := strings.Repeat(s.Bar, filled) + strings.Repeat(s.Empty, width-filled)
	if score >= 80 {
		return styles.Success.Render(bar)
	}
	if score >= 60 {
		return styles.Warning.Render(bar)
	}
	return styles.Danger.Render(bar)
}

func visibleWindow(total, limit, cursor int) (int, int) {
	if total <= 0 {
		return 0, 0
	}
	limit = maxInt(1, minInt(limit, total))
	cursor = minInt(maxInt(0, cursor), total-1)
	start := cursor - limit/2
	if start < 0 {
		start = 0
	}
	if start+limit > total {
		start = total - limit
	}
	end := minInt(total, start+limit)
	return start, end
}

func compactDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	d = d.Round(time.Second)
	if d < time.Second {
		return "just now"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh ago", int(d.Hours()))
}

func formatClock(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("15:04:05")
}

func formatDateTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("2006-01-02 15:04:05")
}

func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}

func runeLen(s string) int {
	return len([]rune(s))
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func displayVersion(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return "dev"
	}
	if version == "dev" || strings.HasPrefix(version, "v") {
		return version
	}
	return "v" + version
}
