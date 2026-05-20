package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m *model) renderOverview(width int, mode screenMode) string {
	if mode == modeCompact {
		return lipgloss.JoinVertical(lipgloss.Left,
			m.renderCurrentRunCard(width),
			m.renderQualityGateCard(width),
			m.renderPipelineCard(width),
			m.renderRunsHistoryCard(width, 5),
			m.renderLatestActivityCard(width, 6),
		)
	}
	left := maxInt(48, minInt(64, width*42/100))
	if mode == modeMedium {
		left = maxInt(40, minInt(50, width*44/100))
	}
	right := width - left - 2
	if right < 38 {
		return m.renderOverview(width, modeCompact)
	}
	row1 := lipgloss.JoinHorizontal(lipgloss.Top,
		m.renderCurrentRunCard(left),
		"  ",
		m.renderQualityGateCard(right),
	)
	row2 := lipgloss.JoinHorizontal(lipgloss.Top,
		m.renderRunsHistoryCard(left, 5),
		"  ",
		m.renderLatestActivityCard(right, 6),
	)
	return lipgloss.JoinVertical(lipgloss.Left,
		row1,
		m.renderPipelineCard(width),
		row2,
	)
}

func (m *model) renderCurrentRunCard(width int) string {
	run := m.data.Current
	if run.RunID == "" && run.Feature == "" {
		return card("Current Run", width, emptyState(width-4,
			"No active run found.",
			`harness sprint new "first goal"`))
	}
	body := []string{
		fmt.Sprintf("Sprint %-3s", runNumber(run)),
		truncate(defaultString(run.Feature, "-"), width-4),
		"",
		kv("Status", stripANSI(statusBadge(run.Status)), width-4),
		kv("Agent", defaultString(run.Agent, defaultString(m.data.Project.Agent, "-")), width-4),
		kv("Started", formatClock(run.StartedAt), width-4),
		kv("Runtime", defaultString(run.Runtime, "-"), width-4),
		kv("Updated", relativeUpdated(run.UpdatedAt), width-4),
		kv("Branch", defaultString(run.Branch, defaultString(m.data.Project.Branch, "-")), width-4),
	}
	rendered := strings.Join(body, "\n")
	rendered = strings.Replace(rendered, statusLabel(run.Status), statusStyle(run.Status).Render(statusLabel(run.Status)), 1)
	return card("Current Run", width, rendered)
}

func (m *model) renderQualityGateCard(width int) string {
	run := m.data.Current
	if len(run.Quality) == 0 {
		return card("Quality Gate", width, emptyState(width-4,
			"No quality report available.",
			"harness sprint qa"))
	}
	barWidth := maxInt(8, minInt(22, width-20))
	lines := []string{
		styles.Muted.Render("Score"),
		statusStyle(run.Status).Render(fmt.Sprintf("%d /100", run.Score)),
		progressBar(run.Score, barWidth),
		"",
		styles.TableHeader.Render(qualityHeader(width - 4)),
	}
	limit := 6
	for _, q := range run.Quality[:minInt(limit, len(run.Quality))] {
		lines = append(lines, qualityRow(q, width-4))
	}
	if len(run.Quality) > limit {
		lines = append(lines, styles.Muted.Render(fmt.Sprintf("%d more dimensions hidden", len(run.Quality)-limit)))
	}
	return card("Quality Gate", width, strings.Join(lines, "\n"))
}

func (m *model) renderPipelineCard(width int) string {
	run := m.data.Current
	if run.Validations == nil {
		run.Validations = map[string]string{}
	}
	stages := []string{"contract", "build", "qa", "report", "accept"}
	labels := []string{"Contract", "Build", "QA", "Report", "Accept"}
	if width < 88 {
		var lines []string
		for i, key := range stages {
			status := defaultString(run.Validations[key], "pending")
			lines = append(lines, styledRow(
				styledColumn{Value: labels[i], Width: 12},
				styledColumn{Value: statusLabel(status), Width: maxInt(8, width-18), Style: statusStyle(status), Styled: true},
			))
		}
		return card("Pipeline", width, strings.Join(lines, "\n"))
	}
	contentWidth := maxInt(40, width-4)
	arrowWidth := 4 * 3
	segment := maxInt(10, (contentWidth-arrowWidth)/5)
	if segment*5+arrowWidth > contentWidth {
		segment = maxInt(8, (contentWidth-arrowWidth)/5)
	}
	var top strings.Builder
	var bottom strings.Builder
	for i, key := range stages {
		if i > 0 {
			top.WriteString(" " + symbols().Arrow + " ")
			bottom.WriteString("   ")
		}
		status := defaultString(run.Validations[key], "pending")
		top.WriteString(padRight(labels[i], segment))
		bottom.WriteString(padStyled(statusStyle(status).Render(statusLabel(status)), segment))
	}
	return card("Pipeline", width, top.String()+"\n"+bottom.String())
}

func (m *model) renderRunsHistoryCard(width, limit int) string {
	if len(m.data.Runs) == 0 {
		return card("Runs History", width, emptyState(width-4,
			"No runs yet.",
			`harness sprint new "feature"`))
	}
	lines := []string{styles.TableHeader.Render(historyHeader(width - 4))}
	for _, run := range m.data.Runs[:minInt(limit, len(m.data.Runs))] {
		lines = append(lines, historyRow(run, width-4))
	}
	return card("Runs History", width, strings.Join(lines, "\n"))
}

func (m *model) renderLatestActivityCard(width, limit int) string {
	events := append([]ActivityEvent{}, m.notices...)
	events = append(events, m.data.Events...)
	if len(events) == 0 {
		return card("Latest Activity", width, styles.Muted.Render("Waiting for agent activity..."))
	}
	lines := []string{
		styles.Muted.Render(fmt.Sprintf("watching .harness  last event: %s  updated %s",
			m.data.LastEvent, relativeUpdated(m.data.LastSeen))),
	}
	for _, ev := range events[:minInt(limit, len(events))] {
		lines = append(lines, renderEventLine(ev, width-4))
	}
	return card("Latest Activity", width, strings.Join(lines, "\n"))
}

func runNumber(run RunRecord) string {
	if run.Number > 0 {
		return fmt.Sprintf("%03d", run.Number)
	}
	if run.RunID != "" {
		return truncate(run.RunID, 12)
	}
	return "-"
}

func qualityHeader(width int) string {
	if width < 58 {
		return row(
			column{Value: "Dimension", Width: 14},
			column{Value: "Score", Width: 7},
			column{Value: "Status", Width: 10},
			column{Value: "Find", Width: 5},
		)
	}
	return row(
		column{Value: "Dimension", Width: 15},
		column{Value: "Score", Width: 7},
		column{Value: "Threshold", Width: 10},
		column{Value: "Status", Width: 10},
		column{Value: "Findings", Width: 8},
	)
}

func qualityRow(q QualityDimension, width int) string {
	if width < 58 {
		return styledRow(
			styledColumn{Value: q.Dimension, Width: 14},
			styledColumn{Value: fmt.Sprintf("%d", q.Score), Width: 7, Style: statusStyle(q.Status), Styled: true},
			styledColumn{Value: statusLabel(q.Status), Width: 10, Style: statusStyle(q.Status), Styled: true},
			styledColumn{Value: fmt.Sprintf("%d", q.Findings), Width: 5},
		)
	}
	return styledRow(
		styledColumn{Value: q.Dimension, Width: 15},
		styledColumn{Value: fmt.Sprintf("%d", q.Score), Width: 7, Style: statusStyle(q.Status), Styled: true},
		styledColumn{Value: fmt.Sprintf("%d", q.Threshold), Width: 10},
		styledColumn{Value: statusLabel(q.Status), Width: 10, Style: statusStyle(q.Status), Styled: true},
		styledColumn{Value: fmt.Sprintf("%d", q.Findings), Width: 8},
	)
}

func historyHeader(width int) string {
	if width < 58 {
		return row(column{Value: "#", Width: 4}, column{Value: "Goal", Width: maxInt(10, width-24)}, column{Value: "Status", Width: 7}, column{Value: "Score", Width: 5})
	}
	return row(column{Value: "#", Width: 4}, column{Value: "Goal", Width: maxInt(14, width-37)}, column{Value: "Status", Width: 7}, column{Value: "Score", Width: 5}, column{Value: "Time", Width: 7}, column{Value: "Find", Width: 4})
}

func historyRow(run RunRecord, width int) string {
	if width < 58 {
		return styledRow(
			styledColumn{Value: runNumber(run), Width: 4},
			styledColumn{Value: run.Feature, Width: maxInt(10, width-24)},
			styledColumn{Value: statusLabel(run.Status), Width: 7, Style: statusStyle(run.Status), Styled: true},
			styledColumn{Value: fmt.Sprintf("%d", run.Score), Width: 5, Style: statusStyle(run.Status), Styled: true},
		)
	}
	return styledRow(
		styledColumn{Value: runNumber(run), Width: 4},
		styledColumn{Value: run.Feature, Width: maxInt(14, width-37)},
		styledColumn{Value: statusLabel(run.Status), Width: 7, Style: statusStyle(run.Status), Styled: true},
		styledColumn{Value: fmt.Sprintf("%d", run.Score), Width: 5, Style: statusStyle(run.Status), Styled: true},
		styledColumn{Value: defaultString(run.Runtime, "-"), Width: 7},
		styledColumn{Value: fmt.Sprintf("%d", run.Findings), Width: 4},
	)
}

func padRight(value string, width int) string {
	value = truncate(value, width)
	if pad := width - runeLen(value); pad > 0 {
		return value + strings.Repeat(" ", pad)
	}
	return value
}

func padStyled(value string, width int) string {
	visible := lipgloss.Width(value)
	if visible > width {
		return truncate(stripANSI(value), width)
	}
	if visible < width {
		return value + strings.Repeat(" ", width-visible)
	}
	return value
}
