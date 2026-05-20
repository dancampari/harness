package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m *model) renderLogsView(width int) string {
	events := m.renderEventsPanel(width)
	commands := m.renderCommandsPanel(width)
	if width >= 120 {
		left := width/2 - 1
		right := width - left - 2
		return lipgloss.JoinHorizontal(lipgloss.Top, m.renderEventsPanel(left), "  ", m.renderCommandsPanel(right))
	}
	return lipgloss.JoinVertical(lipgloss.Left, events, commands)
}

func (m *model) renderEventsPanel(width int) string {
	if len(m.data.Events) == 0 {
		return card("Events", width, styles.Muted.Render("No events found."))
	}
	limit := maxInt(6, m.availableBodyHeight()/2)
	start := minInt(m.scrollFor(viewLogs), maxInt(0, len(m.data.Events)-limit))
	end := minInt(len(m.data.Events), start+limit)
	var lines []string
	for _, ev := range m.data.Events[start:end] {
		lines = append(lines, renderEventLine(ev, width-4))
	}
	if len(m.data.Events) > limit {
		lines = append(lines, styles.Muted.Render(rangeLabel("Events", start, end, len(m.data.Events))))
	}
	return card("Events", width, strings.Join(lines, "\n"))
}

func (m *model) renderCommandsPanel(width int) string {
	commands := append([]string{}, m.commandLog...)
	commands = append(commands, m.data.Commands...)
	if m.commandBusy {
		commands = append(commands, "running: "+m.commandRun)
	}
	if len(commands) == 0 {
		return card("Commands", width, styles.Muted.Render("No command log found. Use : to run a harness command."))
	}
	limit := maxInt(6, m.availableBodyHeight()/2)
	if len(commands) > limit {
		commands = commands[len(commands)-limit:]
	}
	var lines []string
	for _, line := range commands {
		lines = append(lines, renderLogLine(line, width-4))
	}
	return card("Commands", width, strings.Join(lines, "\n"))
}

func renderLogLine(line string, width int) string {
	low := strings.ToLower(line)
	line = truncate(line, width)
	switch {
	case strings.Contains(low, "fail"), strings.Contains(low, "error"):
		return styles.Danger.Render(line)
	case strings.Contains(low, "warn"), strings.Contains(low, "missing"):
		return styles.Warning.Render(line)
	case strings.Contains(low, "pass"), strings.Contains(low, "done"):
		return styles.Success.Render(line)
	default:
		return styles.Text.Render(line)
	}
}
