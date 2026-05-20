package tui

import (
	"strings"
)

func (m *model) renderLogsView(width int) string {
	header := section("Logs · stream", width)

	stream := styles.Success.Render("live")
	if !m.logsFollow {
		stream = styles.Warning.Render("paused")
	}
	statusBar := styles.Muted.Render("showing all events") +
		styles.Muted.Render("   ·  scope ") + styles.Text.Render(defaultString(m.data.Current.RunID, "-")) +
		styles.Muted.Render("   ·  stream ") + stream +
		styles.Muted.Render("   ·  command mode ") + styles.Text.Render(":")

	if len(m.data.Events) == 0 && len(m.commandLog) == 0 && len(m.data.Commands) == 0 {
		return header + "\n" + statusBar + "\n\n" + styles.Muted.Render("No events found.")
	}

	limit := maxInt(6, m.availableBodyHeight()-6)
	start := minInt(m.scrollFor(viewLogs), maxInt(0, len(m.data.Events)-limit))
	end := minInt(len(m.data.Events), start+limit)

	lines := []string{header, statusBar, ""}
	for _, ev := range m.data.Events[start:end] {
		lines = append(lines, renderLogRow(ev, width))
	}
	if len(m.data.Events) > limit {
		lines = append(lines, styles.Muted.Render(rangeLabel("events", start, end, len(m.data.Events))))
	}

	// Command log strip — kept dim, no boxes.
	commands := append([]string{}, m.commandLog...)
	commands = append(commands, m.data.Commands...)
	if m.commandBusy {
		commands = append(commands, "running: "+m.commandRun)
	}
	if len(commands) > 0 {
		lines = append(lines, "", styles.Muted.Render("commands"))
		max := minInt(len(commands), 4)
		for _, cmd := range commands[len(commands)-max:] {
			lines = append(lines, styles.Muted.Render("· ")+styles.Text.Render(truncate(cmd, width-2)))
		}
	}

	return strings.Join(lines, "\n")
}

// renderLogRow: color the LEVEL token only, scope cyan, message in fg.
func renderLogRow(ev ActivityEvent, width int) string {
	clock := formatClock(ev.Timestamp)
	level := deriveLogLevel(ev)
	scope := defaultString(ev.Agent, "harness")
	if s := scopeFromType(ev.Type); s != "" {
		scope = s
	}
	msg := defaultString(ev.Message, "-")

	lvlStyle := styles.Text
	switch strings.ToLower(level) {
	case "warn":
		lvlStyle = styles.Warning
	case "error", "fail":
		lvlStyle = styles.Danger
	case "debug":
		lvlStyle = styles.Faint
	}
	msgWidth := maxInt(8, width-13-1-5-1-10-1)
	return styles.Faint.Render(clock) + " " +
		lvlStyle.Render(padRight(strings.ToUpper(level), 5)) + " " +
		styles.Primary.Render(padRight(scope, 10)) + " " +
		styles.Text.Render(truncate(msg, msgWidth))
}

func deriveLogLevel(ev ActivityEvent) string {
	t := strings.ToLower(ev.Type)
	switch {
	case strings.Contains(t, "fail"), strings.Contains(t, "error"):
		return "error"
	case strings.Contains(t, "warn"), strings.Contains(t, "stale"):
		return "warn"
	case strings.Contains(t, "debug"):
		return "debug"
	default:
		return "info"
	}
}

func scopeFromType(t string) string {
	t = strings.ToLower(t)
	for _, scope := range []string{"contract", "build", "qa", "report", "accept", "repair"} {
		if strings.HasPrefix(t, scope+".") || strings.HasPrefix(t, scope) {
			return scope
		}
	}
	return ""
}
