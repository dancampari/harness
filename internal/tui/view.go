package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func (m *model) View() string {
	width := m.contentWidth()
	mode := modeFor(m.width, m.height)
	if mode == modeTiny {
		return m.fitToScreen(m.renderTiny(width))
	}
	parts := []string{
		m.renderHeader(width),
		m.renderNav(width),
	}
	switch m.activeView {
	case viewRuns:
		parts = append(parts, m.renderRunsView(width))
	case viewReport:
		parts = append(parts, m.renderReportView(width))
	case viewLogs:
		parts = append(parts, m.renderLogsView(width))
	case viewSkills:
		parts = append(parts, m.renderSkillsView(width))
	case viewDoctor:
		parts = append(parts, m.renderDoctorView(width))
	default:
		parts = append(parts, m.renderOverview(width, mode))
	}
	if m.detailOpen {
		parts = append(parts, m.renderDetails(width))
	}
	if m.helpVisible {
		parts = append(parts, m.renderHelp(width))
	}
	parts = append(parts, m.renderFooter(width))
	return m.fitToScreen(strings.Join(parts, "\n"))
}

func (m *model) renderHeader(width int) string {
	project := defaultString(m.data.Project.Name, "project")
	agent := defaultString(m.data.Project.Agent, "manual")
	status := defaultString(m.data.Project.Status, "idle")
	left := lipgloss.JoinHorizontal(lipgloss.Center,
		styles.Brand.Render("harness"),
		"  ",
		styles.Muted.Render("Autonomous Development Pipeline"),
		"  ",
		styles.Primary.Render(m.version),
	)
	right := "Project: " + project + "   Agent: " + styles.Purple.Render(agent) + "   Status: " + statusStyle(status).Render(statusLabel(status))
	gap := width - lipgloss.Width(left) - lipgloss.Width(right) - 4
	if gap < 2 {
		right = truncate(stripANSI(right), maxInt(10, width-lipgloss.Width(left)-6))
		gap = 2
	}
	line := left + strings.Repeat(" ", gap) + right
	return styles.Header.Width(width).Render(line)
}

func (m *model) renderNav(width int) string {
	var items []string
	for i, label := range viewLabels {
		item := fmt.Sprintf("[%d] %s", i+1, label)
		if viewID(i) == m.activeView {
			item = styles.NavActive.Render(item)
		} else {
			item = styles.Nav.Render(item)
		}
		items = append(items, item)
	}
	left := strings.Join(items, "   ")
	right := styles.Muted.Render("[r] Refresh   [?] Help   [q] Quit")
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 2 {
		return fitPlainLine(left, width)
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m *model) renderFooter(width int) string {
	line := "[enter] Details   [o] Open Report   [d] Doctor   [1-6] Switch View   [r] Refresh   [q] Quit"
	if m.commandMode {
		line = "> " + m.commandInput
	}
	return styles.Footer.Width(width).Render(truncate(line, width-4))
}

func (m *model) renderTiny(width int) string {
	current := m.data.Current
	lines := []string{
		styles.Brand.Render("harness") + " " + styles.Muted.Render(m.version),
		fmt.Sprintf("Project: %s", defaultString(m.data.Project.Name, "project")),
		fmt.Sprintf("Status : %s", statusBadge(defaultString(current.Status, "idle"))),
		fmt.Sprintf("Run    : %s", truncate(defaultString(current.Feature, "No active run"), width-9)),
		fmt.Sprintf("Score  : %s", scoreText(current.Score, current.Status)),
		"",
		styles.Muted.Render("Terminal is too small for dashboard mode."),
		styles.Muted.Render("Increase width/height or use [1-6], [r], [q]."),
	}
	if len(m.data.Events) > 0 {
		lines = append(lines, "", styles.CardTitle.Render("Latest"))
		for _, ev := range m.data.Events[:minInt(3, len(m.data.Events))] {
			lines = append(lines, renderEventLine(ev, width))
		}
	}
	return strings.Join(lines, "\n")
}

func (m *model) renderDetails(width int) string {
	run := m.selectedRun()
	body := []string{
		kv("Run", defaultString(run.RunID, "-"), width-4),
		kv("Goal", defaultString(run.Feature, "-"), width-4),
		kv("Status", statusLabel(run.Status), width-4),
		kv("Score", fmt.Sprintf("%d/100", run.Score), width-4),
		kv("Agent", defaultString(run.Agent, "-"), width-4),
		kv("Started", formatDateTime(run.StartedAt), width-4),
		kv("Updated", formatDateTime(run.UpdatedAt), width-4),
		kv("Runtime", defaultString(run.Runtime, "-"), width-4),
		kv("Report", defaultString(run.ReportPath, "-"), width-4),
		"",
		styles.Muted.Render("esc or enter closes details"),
	}
	return card("Details", minInt(width, 96), strings.Join(body, "\n"))
}

func (m *model) renderHelp(width int) string {
	body := []string{
		"[1-6] switch view     [tab] next view     [shift+tab] previous view",
		"[up/down] navigate    [enter] details     [o] open latest report",
		"[r] refresh           [d] doctor          [:] command mode",
		"[esc] close help/details                  [q] quit",
		"",
		"Command mode shortcuts: qa, repair, accept, score, status, doctor, propose, approve tester, new <goal>, !shell",
	}
	return card("Help", minInt(width, 96), strings.Join(body, "\n"))
}

func scoreText(score int, status string) string {
	text := fmt.Sprintf("%d/100", score)
	if score <= 0 && normalizeStatus(status) == "pending" {
		text = "-"
	}
	return statusStyle(status).Render(text)
}

func renderEventLine(ev ActivityEvent, width int) string {
	clock := formatClock(ev.Timestamp)
	eventType := defaultString(ev.Type, "event")
	message := defaultString(ev.Message, "-")
	if eventType == "report.opened" || eventType == "report.open.failed" {
		message = filepathBase(message)
	}
	line := row(
		column{Value: clock, Width: 8},
		column{Value: eventType, Width: 24},
		column{Value: message, Width: maxInt(12, width-38)},
	)
	switch {
	case strings.Contains(eventType, "fail"), strings.Contains(strings.ToLower(message), "fail"):
		return styles.Danger.Render(truncate(line, width))
	case strings.Contains(eventType, "pass"):
		return styles.Success.Render(truncate(line, width))
	case strings.Contains(eventType, "warn"), strings.Contains(strings.ToLower(message), "missing"):
		return styles.Warning.Render(truncate(line, width))
	default:
		return styles.Text.Render(truncate(line, width))
	}
}

func relativeUpdated(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return compactDuration(time.Since(t))
}
