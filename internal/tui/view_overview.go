package tui

import (
	"fmt"
	"strings"
)

func (m *model) renderOverview(width int, mode screenMode) string {
	parts := []string{
		m.renderCurrentRun(width),
		"",
		m.renderPipeline(width),
		"",
		m.renderQualityGate(width),
		"",
		m.renderLatestActivity(width, mode),
	}
	return strings.Join(parts, "\n")
}

// renderCurrentRun: section + sprint/title row + dim/value rows + score bar.
func (m *model) renderCurrentRun(width int) string {
	run := m.data.Current
	header := section("Current run", width)
	if run.RunID == "" && run.Feature == "" {
		return header + "\n" + emptyState(
			"No active run found.",
			`harness feature new "first goal"`)
	}
	labelW := 12
	titleAvail := width - labelW - 2
	if titleAvail < 12 {
		titleAvail = 12
	}
	title := truncate(defaultString(run.Feature, "-"), titleAvail)

	titleLine := styles.Primary.Render("Sprint "+runNumber(run)) + "   " + styles.Text.Render(title)

	barWidth := pickBarWidth(width)
	bar := progressBar(run.Score, runTarget(run), barWidth)

	lines := []string{
		header,
		titleLine,
		labelValue("Status", statusBadge(run.Status), labelW),
		labelValue("Runtime", styles.Text.Render(defaultString(run.Runtime, "-")), labelW),
		labelValue("Updated", styles.Text.Render(relativeUpdated(run.UpdatedAt)), labelW),
		labelValue("Branch", styles.Text.Render(defaultString(run.Branch, defaultString(m.data.Project.Branch, "-"))), labelW),
		labelValue("Score", styles.Text.Render(fmt.Sprintf("%d/100", run.Score))+"   "+bar, labelW),
	}
	return strings.Join(lines, "\n")
}

// renderPipeline: one line "✓ Contract agreed  →  ✓ Build done  →  ..." or
// stacked rows when the pipeline doesn't fit on a single line.
func (m *model) renderPipeline(width int) string {
	run := m.data.Current
	if run.Validations == nil {
		run.Validations = map[string]string{}
	}
	stages := []string{"contract", "build", "qa", "report", "accept"}
	labels := []string{"Contract", "Build", "QA", "Report", "Accept"}

	type seg struct{ name, state string }
	segs := make([]seg, len(stages))
	for i, key := range stages {
		state := defaultString(run.Validations[key], "pending")
		segs[i] = seg{labels[i], strings.ToLower(state)}
	}

	header := section("Pipeline", width)
	s := symbols()
	arrow := "  " + styles.Faint.Render(s.Arrow) + "  "

	// Try one-liner first.
	var oneLine strings.Builder
	for i, sg := range segs {
		if i > 0 {
			oneLine.WriteString(arrow)
		}
		stateLabel := strings.ToLower(sg.state)
		oneLine.WriteString(statusStyle(sg.state).Render(statusGlyph(sg.state)))
		oneLine.WriteString(" ")
		oneLine.WriteString(styles.Text.Render(sg.name + " " + stateLabel))
	}
	body := oneLine.String()
	if visibleWidth(body) <= width {
		return header + "\n" + body
	}

	// Stacked fallback: one stage per row.
	rows := make([]string, 0, len(segs))
	for _, sg := range segs {
		rows = append(rows, styles.Text.Render(padRight(sg.name, 10))+"  "+
			statusStyle(sg.state).Render(statusGlyph(sg.state)+" "+strings.ToUpper(sg.state)))
	}
	return header + "\n" + strings.Join(rows, "\n")
}

// renderQualityGate: simple `name   score/target   ✓ PASS` rows. No box header.
func (m *model) renderQualityGate(width int) string {
	header := section("Quality gate", width)
	run := m.data.Current
	if len(run.Quality) == 0 {
		return header + "\n" + emptyState(
			"No quality report available.",
			"harness feature qa")
	}
	lines := []string{header}
	for _, q := range run.Quality {
		scoreStr := fmt.Sprintf("%d/%d", q.Score, q.Threshold)
		scoreStyle := styles.Text
		if q.Score < q.Threshold {
			scoreStyle = styles.Warning
		}
		line := styles.Text.Render(padRight(q.Dimension, 14)) + "  " +
			scoreStyle.Render(padRight(scoreStr, 10)) + "  " +
			statusBadge(q.Status)
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// renderLatestActivity: "HH:MM:SS   event.name   detail" rows.
func (m *model) renderLatestActivity(width int, mode screenMode) string {
	header := section("Latest activity", width)
	events := append([]ActivityEvent{}, m.notices...)
	events = append(events, m.data.Events...)
	if len(events) == 0 {
		return header + "\n" + styles.Muted.Render("Waiting for agent activity...")
	}
	limit := 4
	switch mode {
	case modeMedium:
		limit = 5
	case modeWide:
		limit = 6
	}
	lines := []string{header}
	for _, ev := range events[:minInt(limit, len(events))] {
		lines = append(lines, renderActivityRow(ev, width))
	}
	return strings.Join(lines, "\n")
}

func renderActivityRow(ev ActivityEvent, width int) string {
	clock := formatClock(ev.Timestamp)
	eventType := defaultString(ev.Type, "event")
	message := defaultString(ev.Message, "-")
	if eventType == "report.opened" || eventType == "report.open.failed" {
		message = filepathBase(message)
	}
	msgWidth := maxInt(8, width-8-3-24-3)
	return styles.Faint.Render(clock) + "   " +
		styles.Text.Render(padRight(eventType, 22)) + "   " +
		styles.Muted.Render(truncate(message, msgWidth))
}

func runTarget(run RunRecord) int {
	for _, q := range run.Quality {
		if q.Threshold > 0 {
			return q.Threshold
		}
	}
	return 70
}

func pickBarWidth(width int) int {
	switch {
	case width >= 140:
		return 32
	case width >= 100:
		return 24
	default:
		return 16
	}
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

// visibleWidth returns the rendered cell width of s, ignoring ANSI escapes.
func visibleWidth(s string) int {
	plain := stripANSI(s)
	return runeLen(plain)
}
