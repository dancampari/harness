package tui

import (
	"fmt"
	"strings"
)

func (m *model) renderRunsView(width int) string {
	if len(m.data.Runs) == 0 {
		return card("Runs", width, emptyState(width-4,
			"No run history found.",
			`harness sprint new "feature"`))
	}
	limit := maxInt(4, m.availableBodyHeight()-7)
	start, end := visibleWindow(len(m.data.Runs), limit, m.runCursor)
	lines := []string{styles.TableHeader.Render(runsHeader(width - 4))}
	for i := start; i < end; i++ {
		lines = append(lines, m.runsRow(i, width-4))
	}
	if len(m.data.Runs) > limit {
		lines = append(lines, styles.Muted.Render(fmt.Sprintf("Rows %d-%d/%d", start+1, end, len(m.data.Runs))))
	}
	lines = append(lines, "", styles.Muted.Render("Use arrows to select a run. Press enter for details."))
	return card("Runs", width, strings.Join(lines, "\n"))
}

func runsHeader(width int) string {
	if width < 88 {
		return row(
			column{Value: "#", Width: 5},
			column{Value: "Goal", Width: maxInt(14, width-44)},
			column{Value: "Status", Width: 9},
			column{Value: "Score", Width: 6},
			column{Value: "Time", Width: 8},
		)
	}
	return row(
		column{Value: "#", Width: 5},
		column{Value: "Goal", Width: maxInt(20, width-78)},
		column{Value: "Status", Width: 9},
		column{Value: "Score", Width: 6},
		column{Value: "Time", Width: 8},
		column{Value: "Updated", Width: 12},
		column{Value: "Find", Width: 5},
		column{Value: "Report", Width: 20},
	)
}

func (m *model) runsRow(index, width int) string {
	run := m.data.Runs[index]
	marker := " "
	if index == m.runCursor {
		marker = ">"
	}
	if width < 88 {
		line := styledRow(
			styledColumn{Value: marker + runNumber(run), Width: 5},
			styledColumn{Value: run.Feature, Width: maxInt(14, width-44)},
			styledColumn{Value: statusLabel(run.Status), Width: 9, Style: statusStyle(run.Status), Styled: true},
			styledColumn{Value: fmt.Sprintf("%d", run.Score), Width: 6, Style: statusStyle(run.Status), Styled: true},
			styledColumn{Value: defaultString(run.Runtime, "-"), Width: 8},
		)
		return styleSelected(line, run.Status, index == m.runCursor)
	}
	report := run.ReportPath
	if report != "" {
		report = filepathBase(report)
	}
	line := styledRow(
		styledColumn{Value: marker + runNumber(run), Width: 5},
		styledColumn{Value: run.Feature, Width: maxInt(20, width-78)},
		styledColumn{Value: statusLabel(run.Status), Width: 9, Style: statusStyle(run.Status), Styled: true},
		styledColumn{Value: fmt.Sprintf("%d", run.Score), Width: 6, Style: statusStyle(run.Status), Styled: true},
		styledColumn{Value: defaultString(run.Runtime, "-"), Width: 8},
		styledColumn{Value: relativeUpdated(run.UpdatedAt), Width: 12},
		styledColumn{Value: fmt.Sprintf("%d", run.Findings), Width: 5},
		styledColumn{Value: defaultString(report, "-"), Width: 20},
	)
	return styleSelected(line, run.Status, index == m.runCursor)
}

func styleSelected(line, status string, selected bool) string {
	if selected {
		return styles.Selected.Render(line)
	}
	return line
}

func filepathBase(path string) string {
	path = strings.ReplaceAll(path, "\\", "/")
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}
