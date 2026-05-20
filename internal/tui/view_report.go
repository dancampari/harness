package tui

import (
	"strconv"
	"strings"
)

func (m *model) renderReportView(width int) string {
	if strings.TrimSpace(m.data.ReportMarkdown) == "" {
		return card("Report", width, emptyState(width-4,
			"No report available.",
			"harness sprint qa"))
	}
	limit := maxInt(6, m.availableBodyHeight()-7)
	lines := cleanLines(strings.Split(m.data.ReportMarkdown, "\n"))
	start := minInt(m.scrollFor(viewReport), maxInt(0, len(lines)-limit))
	end := minInt(len(lines), start+limit)
	var rendered []string
	for _, line := range lines[start:end] {
		rendered = append(rendered, renderMarkdownPreviewLine(line, width-4))
	}
	if len(lines) > limit {
		rendered = append(rendered, styles.Muted.Render(rangeLabel("Report", start, end, len(lines))))
	}
	if m.data.ReportPath != "" {
		rendered = append([]string{styles.Muted.Render("Path: " + m.data.ReportPath), ""}, rendered...)
	}
	return card("Report", width, strings.Join(rendered, "\n"))
}

func renderMarkdownPreviewLine(line string, width int) string {
	line = strings.TrimRight(line, "\r")
	switch {
	case strings.HasPrefix(line, "#"):
		return styles.Primary.Render(truncate(strings.TrimSpace(strings.TrimLeft(line, "#")), width))
	case strings.HasPrefix(line, "|"):
		return styles.Text.Render(truncate(line, width))
	case strings.HasPrefix(strings.TrimSpace(line), "- "):
		return styles.Muted.Render(truncate(line, width))
	default:
		return styles.Text.Render(truncate(line, width))
	}
}

func rangeLabel(label string, start, end, total int) string {
	return label + " " + strconv.Itoa(start+1) + "-" + strconv.Itoa(end) + "/" + strconv.Itoa(total)
}
