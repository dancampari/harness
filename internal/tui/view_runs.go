package tui

import (
	"fmt"
	"strings"
)

func (m *model) renderRunsView(width int) string {
	header := section("Runs history", width)
	if len(m.data.Runs) == 0 {
		return header + "\n" + emptyState("No run history found.", `harness sprint new "feature"`)
	}
	showBranch := width >= 110
	limit := maxInt(4, m.availableBodyHeight()-5)
	start, end := visibleWindow(len(m.data.Runs), limit, m.runCursor)

	lines := []string{
		header,
		runsHeader(width, showBranch),
		rule(width),
	}
	for i := start; i < end; i++ {
		lines = append(lines, m.runsRow(i, width, showBranch))
	}
	footerLine := styles.Muted.Render(
		fmt.Sprintf("%d runs · filter: ", len(m.data.Runs))) +
		styles.Text.Render("all") +
		styles.Muted.Render(" · sort: ") +
		styles.Text.Render("recent")
	if len(m.data.Runs) > limit {
		footerLine = styles.Muted.Render(fmt.Sprintf("rows %d-%d/%d", start+1, end, len(m.data.Runs))) +
			styles.Muted.Render("   ·   ") + footerLine
	}
	lines = append(lines, "", footerLine)
	return strings.Join(lines, "\n")
}

func runsColumns(width int, showBranch bool) (idW, goalW, stW, rtW, ageW, scW, brW int) {
	idW, stW, rtW, ageW, scW, brW = 6, 10, 8, 10, 8, 22
	fixed := 2 + idW + stW + rtW + ageW + scW
	if showBranch {
		fixed += brW
	}
	goalW = maxInt(14, width-fixed-2)
	return
}

func runsHeader(width int, showBranch bool) string {
	_, goalW, stW, rtW, ageW, scW, brW := runsColumns(width, showBranch)
	headers := []string{
		styles.TableHeader.Render(padRight("", 2)),
		styles.TableHeader.Render(padRight("#", 6)),
		styles.TableHeader.Render(padRight("sprint", goalW)),
		styles.TableHeader.Render(padRight("status", stW)),
		styles.TableHeader.Render(padRight("runtime", rtW)),
		styles.TableHeader.Render(padRight("age", ageW)),
		styles.TableHeader.Render(padRight("score", scW)),
	}
	if showBranch {
		headers = append(headers, styles.TableHeader.Render(padRight("branch", brW)))
	}
	return strings.Join(headers, "")
}

func (m *model) runsRow(index, width int, showBranch bool) string {
	run := m.data.Runs[index]
	isSel := index == m.runCursor
	mark := " "
	if isSel {
		mark = symbols().Mark
	}

	stStyle := statusStyle(run.Status)
	glyph := statusGlyph(run.Status)
	stLabel := statusLabel(run.Status)

	idW, goalW, stW, rtW, ageW, scW, brW := runsColumns(width, showBranch)

	// Compose the row from cell-aligned segments. Status glyph + label are the
	// only painted parts; everything else stays in fg / dim per design.
	cells := []string{
		styles.SelectMark.Render(padRight(mark, 2)),
		styles.Text.Render(padRight(runNumber(run), idW)),
		styles.Text.Render(padRight(defaultString(run.Feature, "-"), goalW)),
		stStyle.Render(padRight(glyph+" "+stLabel, stW)),
		styles.Text.Render(padRight(defaultString(run.Runtime, "-"), rtW)),
		styles.Muted.Render(padRight(relativeUpdated(run.UpdatedAt), ageW)),
		styles.Text.Render(padRight(fmt.Sprintf("%d/100", run.Score), scW)),
	}
	if showBranch {
		cells = append(cells, styles.Muted.Render(padRight(defaultString(run.Branch, "-"), brW)))
	}
	line := strings.Join(cells, "")

	// Selection highlight: extend background across the row width.
	if isSel {
		visible := visibleWidth(line)
		if visible < width {
			line = line + strings.Repeat(" ", width-visible)
		}
		line = styles.Selected.Render(line)
	}
	return line
}

func filepathBase(path string) string {
	path = strings.ReplaceAll(path, "\\", "/")
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}
