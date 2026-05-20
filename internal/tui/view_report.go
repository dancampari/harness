package tui

import (
	"fmt"
	"strconv"
	"strings"
)

func (m *model) renderReportView(width int) string {
	run := m.data.Current
	headerLabel := "Report"
	if run.RunID != "" {
		headerLabel = "Report · " + truncate(run.RunID, 32)
	}
	header := section(headerLabel, width)

	if strings.TrimSpace(m.data.ReportMarkdown) == "" && len(run.Quality) == 0 {
		return header + "\n" + emptyState("No report available.", "harness sprint qa")
	}

	labelW := 12
	target := runTarget(run)
	verdictLine := labelValue("Verdict", statusBadge(run.Status), labelW) +
		"   " + styles.Muted.Render("score ") +
		styles.Text.Render(fmt.Sprintf("%d/100", run.Score)) +
		"   " + progressBar(run.Score, target, pickBarWidth(width))

	durationLine := labelValue("Duration", styles.Text.Render(defaultString(run.Runtime, "-")), labelW) +
		"   " + styles.Muted.Render("·  findings ") +
		styles.Text.Render(strconv.Itoa(run.Findings))

	lines := []string{
		header,
		labelValue("Run", styles.Text.Render(defaultString(run.RunID, "-")), labelW),
		labelValue("Feature", styles.Text.Render(truncate(defaultString(run.Feature, "-"), width-labelW-2)), labelW),
		verdictLine,
		durationLine,
		"",
		section("Score breakdown", width),
	}
	for _, q := range run.Quality {
		barW := pickBarWidth(width)
		scoreStyle := styles.Text
		if q.Score < q.Threshold {
			scoreStyle = styles.Warning
		}
		line := styles.Text.Render(padRight(q.Dimension, 14)) + "  " +
			scoreStyle.Render(padRight(fmt.Sprintf("%d/%d", q.Score, q.Threshold), 10)) + "  " +
			progressBar(q.Score, q.Threshold, barW)
		lines = append(lines, line)
	}

	if m.data.ReportPath != "" {
		lines = append(lines, "", styles.Muted.Render("artifact: ")+styles.Text.Render(m.data.ReportPath)+
			"   "+styles.Muted.Render("·  ")+styles.Primary.Render("press [o]")+
			styles.Muted.Render(" to open in $PAGER"))
	}

	// Preview the markdown body, if present.
	if strings.TrimSpace(m.data.ReportMarkdown) != "" {
		lines = append(lines, "", section("Preview", width))
		previewLines := cleanLines(strings.Split(m.data.ReportMarkdown, "\n"))
		limit := maxInt(3, m.availableBodyHeight()-len(lines)-2)
		if limit > len(previewLines) {
			limit = len(previewLines)
		}
		start := minInt(m.scrollFor(viewReport), maxInt(0, len(previewLines)-limit))
		end := minInt(len(previewLines), start+limit)
		for _, line := range previewLines[start:end] {
			lines = append(lines, renderMarkdownPreviewLine(line, width))
		}
		if len(previewLines) > limit {
			lines = append(lines, styles.Muted.Render(rangeLabel("preview", start, end, len(previewLines))))
		}
	}
	return strings.Join(lines, "\n")
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
