package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/dancampari/harness/internal/progress"
)

func (m *model) View() string {
	width := m.contentWidth()
	mode := modeFor(m.width, m.height)
	if mode == modeTiny {
		return m.fitToScreen(m.renderTiny(width))
	}
	// Build the screen with: header, blank, tabs, blank, body, grow, footer-rule, footer.
	parts := []string{
		m.renderHeader(width),
		"",
		m.renderNav(width),
		"",
	}
	// The live panel is injected above the active view so the realtime
	// feedback is visible no matter which tab the user is on — and no
	// matter whether the run was launched from the TUI or by an agent.
	if m.liveActive() {
		parts = append(parts, m.renderLiveCommand(width), "")
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
		parts = append(parts, "", m.renderDetails(width))
	}
	if m.helpVisible {
		parts = append(parts, "", m.renderHelp(width))
	}
	body := strings.Join(parts, "\n")
	footer := rule(width) + "\n" + m.renderFooter(width)
	// glue footer to the bottom of the visible area
	bodyLines := strings.Split(body, "\n")
	footerLines := strings.Split(footer, "\n")
	free := m.height - len(bodyLines) - len(footerLines)
	if free > 0 {
		body = body + strings.Repeat("\n", free)
	}
	return m.fitToScreen(body + "\n" + footer)
}

func (m *model) renderHeader(width int) string {
	project := defaultString(m.data.Project.Name, "project")
	agent := defaultString(m.data.Project.Agent, "manual")
	status := defaultString(m.data.Project.Status, "idle")

	// While a command runs, the header status cell becomes a live
	// working indicator so the user always sees the harness is busy,
	// regardless of which view is active.
	statusCell := statusBadge(status)
	if m.commandBusy {
		statusCell = styles.Warning.Render(spinner(m.frame)+" working") +
			styles.Muted.Render(" "+compactDuration(time.Since(m.commandStarted)))
	}

	left := styles.Brand.Render("harness") + "  " + styles.Muted.Render(m.version)
	right := styles.Muted.Render("project: ") + styles.Text.Render(project) +
		styles.Muted.Render("   agent: ") + styles.Text.Render(agent) +
		styles.Muted.Render("   status: ") + statusCell

	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	gap := width - leftW - rightW
	if gap < 2 {
		// Truncate the right block from the project end so the status badge stays visible.
		short := styles.Muted.Render("agent: ") + styles.Text.Render(agent) +
			styles.Muted.Render("   status: ") + statusCell
		shortW := lipgloss.Width(short)
		gap = width - leftW - shortW
		if gap < 2 {
			return left
		}
		return left + strings.Repeat(" ", gap) + short
	}
	return left + strings.Repeat(" ", gap) + right
}

// liveActive reports whether the realtime panel should be shown: either
// a command was launched from the TUI, or run-progress.json shows a QA
// run in flight (which may have been launched by an agent elsewhere).
func (m *model) liveActive() bool {
	return m.commandBusy || m.data.Progress.Active()
}

// renderLiveCommand is the realtime feedback panel shown under the nav
// bar whenever the harness is working. It has two layers:
//
//   - a structured progress checklist (current phase + per-sensor
//     state) sourced from .harness/run-progress.json, which the
//     evaluator updates live — visible even when the QA run was
//     triggered by an agent rather than the TUI;
//   - the streamed stdout/stderr tail of a command launched from the
//     TUI itself.
//
// Either layer renders on its own; when both are present the structured
// checklist leads and the raw output tail follows.
func (m *model) renderLiveCommand(width int) string {
	prog := m.data.Progress
	progActive := prog.Active()

	title := strings.TrimSpace(m.commandRun)
	if title == "" && progActive {
		title = fmt.Sprintf("QA sprint %03d", prog.SprintNumber)
	}
	lines := []string{section("Live · "+defaultString(title, "harness"), width)}

	if progActive {
		lines = append(lines, m.renderProgressBody(prog, width)...)
	}
	if m.commandBusy {
		if progActive {
			lines = append(lines, "")
		}
		lines = append(lines, m.renderStreamTail(width, progActive)...)
	}
	return strings.Join(lines, "\n")
}

// renderProgressBody renders the phase line and the per-sensor checklist
// from a live progress snapshot.
func (m *model) renderProgressBody(prog progress.Snapshot, width int) []string {
	elapsed := compactDuration(time.Since(prog.StartedAt))
	phaseLine := styles.Warning.Render(spinner(m.frame)) + " " +
		styles.Text.Render(progressPhaseLabel(prog.Phase)) + "   " +
		styles.Muted.Render("elapsed "+elapsed)
	lines := []string{phaseLine, ""}
	if len(prog.Sensors) == 0 {
		lines = append(lines, styles.Faint.Render("  preparing sensors"+symbols().Ell))
		return lines
	}
	done := 0
	for _, s := range prog.Sensors {
		if s.State == "done" || s.State == "error" || s.State == "skipped" {
			done++
		}
		lines = append(lines, m.renderSensorRow(s))
	}
	lines = append(lines, "", styles.Muted.Render(
		fmt.Sprintf("  %d/%d sensors settled", done, len(prog.Sensors))))
	return lines
}

// renderSensorRow renders one sensor's live state with a state-coloured
// glyph; a running sensor shows the animated spinner.
func (m *model) renderSensorRow(s progress.Sensor) string {
	sym := symbols()
	glyph, style := sym.Open, styles.Faint
	detail := s.State
	switch s.State {
	case "done":
		glyph, style = sym.Check, styles.Success
		if s.Duration > 0 {
			detail = compactSeconds(s.Duration)
		}
	case "running":
		glyph, style = spinner(m.frame), styles.Warning
	case "error":
		glyph, style = sym.Cross, styles.Danger
	case "skipped":
		glyph, style = sym.Skip, styles.Faint
	}
	return style.Render("  "+glyph) + " " +
		styles.Text.Render(padRight(s.Name, 18)) + " " +
		styles.Muted.Render(padRight(s.Dimension, 13)) + " " +
		style.Render(detail)
}

// renderStreamTail renders the streamed command output. withHeader is
// true when the structured progress body already showed the spinner and
// elapsed counter, so the tail just needs an "output" label.
func (m *model) renderStreamTail(width int, withHeader bool) []string {
	var lines []string
	if withHeader {
		lines = append(lines, styles.Muted.Render("  output"+symbols().Ell))
	} else {
		elapsed := compactDuration(time.Since(m.commandStarted))
		lines = append(lines, styles.Warning.Render(spinner(m.frame))+" "+
			styles.Text.Render("working")+"   "+
			styles.Muted.Render("elapsed "+elapsed))
	}
	if len(m.commandLines) == 0 {
		lines = append(lines, styles.Faint.Render("  waiting for output"+symbols().Ell))
		return lines
	}
	limit := 8
	switch modeFor(m.width, m.height) {
	case modeWide:
		limit = 12
	case modeMedium:
		limit = 10
	}
	start := maxInt(0, len(m.commandLines)-limit)
	for _, raw := range m.commandLines[start:] {
		lines = append(lines, styles.Faint.Render("  "+truncate(stripANSI(raw), maxInt(8, width-2))))
	}
	if start > 0 {
		lines = append(lines, styles.Muted.Render(
			fmt.Sprintf("  showing last %d of %d lines", len(m.commandLines)-start, len(m.commandLines))))
	}
	return lines
}

func progressPhaseLabel(phase string) string {
	switch phase {
	case "contract":
		return "checking contract"
	case "sensors":
		return "running sensors"
	case "aggregate":
		return "aggregating results"
	case "done":
		return "done"
	default:
		return defaultString(phase, "working")
	}
}

func (m *model) renderNav(width int) string {
	_ = width
	var parts []string
	for i, label := range viewLabels {
		num := fmt.Sprintf("[%d]", i+1)
		var item string
		if viewID(i) == m.activeView {
			item = styles.TabNumOn.Render(num) + " " + styles.TabActive.Render(label)
		} else {
			item = styles.TabNumOff.Render(num) + " " + styles.TabIdle.Render(label)
		}
		parts = append(parts, item)
	}
	return strings.Join(parts, "   ")
}

func (m *model) renderFooter(width int) string {
	if m.commandMode {
		line := "> " + m.commandInput
		return styles.Text.Render(truncate(line, width-1))
	}
	keys := footerKeys(m.activeView)
	return renderKeyHints(keys, width)
}

func footerKeys(v viewID) [][2]string {
	common := [][2]string{
		{"r", "refresh"},
		{"?", "help"},
		{"q", "quit"},
	}
	var specific [][2]string
	switch v {
	case viewOverview:
		specific = [][2]string{{"enter", "details"}, {"o", "report"}, {"d", "doctor"}}
	case viewRuns:
		specific = [][2]string{{"enter", "details"}, {"↑↓", "select"}}
	case viewReport:
		specific = [][2]string{{"o", "open in pager"}, {"↑↓", "scroll"}}
	case viewLogs:
		specific = [][2]string{{"space", "pause/resume"}, {":", "run command"}, {"↑↓", "scroll"}}
	case viewSkills:
		specific = [][2]string{{"enter", "details"}, {"↑↓", "scroll"}}
	case viewDoctor:
		specific = [][2]string{{"f", "doctor --fix"}}
	}
	return append(specific, common...)
}

func renderKeyHints(keys [][2]string, width int) string {
	sep := styles.Faint.Render("   ")
	var parts []string
	for _, k := range keys {
		parts = append(parts, styles.KeyHint.Render("["+k[0]+"]")+" "+styles.Text.Render(k[1]))
	}
	line := strings.Join(parts, sep)
	if lipgloss.Width(line) <= width {
		return line
	}
	return fitPlainLine(line, width)
}

func (m *model) renderTiny(width int) string {
	current := m.data.Current
	lines := []string{
		styles.Brand.Render("harness") + " " + styles.Muted.Render(m.version),
		styles.Muted.Render("project: ") + styles.Text.Render(defaultString(m.data.Project.Name, "project")),
		styles.Muted.Render("status:  ") + statusBadge(defaultString(current.Status, "idle")),
		styles.Muted.Render("run:     ") + styles.Text.Render(truncate(defaultString(current.Feature, "No active run"), width-9)),
		styles.Muted.Render("score:   ") + scoreText(current.Score, current.Status),
		"",
		styles.Muted.Render("Terminal too small for dashboard."),
		styles.Muted.Render("Use [1-6] · [r] refresh · [q] quit"),
	}
	if len(m.data.Events) > 0 {
		lines = append(lines, "", styles.Section.Render("Latest"))
		for _, ev := range m.data.Events[:minInt(3, len(m.data.Events))] {
			lines = append(lines, renderEventLine(ev, width))
		}
	}
	return strings.Join(lines, "\n")
}

func (m *model) renderDetails(width int) string {
	run := m.selectedRun()
	w := minInt(width, 96)
	lines := []string{
		section("Details", w),
		labelValue("Run", defaultString(run.RunID, "-"), 10),
		labelValue("Goal", defaultString(run.Feature, "-"), 10),
		labelValue("Status", statusBadge(run.Status), 10),
		labelValue("Score", fmt.Sprintf("%d/100", run.Score), 10),
		labelValue("Agent", defaultString(run.Agent, "-"), 10),
		labelValue("Started", formatDateTime(run.StartedAt), 10),
		labelValue("Updated", formatDateTime(run.UpdatedAt), 10),
		labelValue("Runtime", defaultString(run.Runtime, "-"), 10),
		labelValue("Report", defaultString(run.ReportPath, "-"), 10),
		"",
		styles.Muted.Render("esc or enter to close"),
	}
	return strings.Join(lines, "\n")
}

func (m *model) renderHelp(width int) string {
	w := minInt(width, 96)
	lines := []string{
		section("Help", w),
		styles.Text.Render("[1-6] switch view     [tab] next     [shift+tab] previous"),
		styles.Text.Render("[↑/↓] navigate        [enter] details   [o] open latest report"),
		styles.Text.Render("[r] refresh           [d] doctor       [:] command mode"),
		styles.Text.Render("[esc] close           [q] quit"),
		"",
		styles.Muted.Render("Commands: qa, repair, accept, score, status, doctor,"),
		styles.Muted.Render("          propose, approve tester, new <goal>, !shell"),
	}
	return strings.Join(lines, "\n")
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
	msgWidth := maxInt(8, width-8-3-24-3)
	line := styles.Faint.Render(clock) + "   " +
		styles.Text.Render(padRight(eventType, 22)) + " " +
		styles.Muted.Render(truncate(message, msgWidth))
	return line
}

func relativeUpdated(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return compactDuration(time.Since(t))
}
