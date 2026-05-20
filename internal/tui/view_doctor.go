package tui

import (
	"fmt"
	"strings"
)

type doctorRow struct {
	name   string
	status string
	detail string
}

func (m *model) renderDoctorView(width int) string {
	header := section("Doctor · environment check", width)
	d := m.data.Doctor

	rows := []doctorRow{
		{"stack", "PASS", defaultString(d.Stack, "unknown")},
		{"package manager", statusOrSkip(d.PackageManager), defaultString(d.PackageManager, "-")},
	}
	for _, v := range d.Validations {
		rows = append(rows, doctorRow{"validation: " + v, "PASS", "configured"})
	}
	for _, s := range d.Scripts {
		rows = append(rows, doctorRow{"script: " + s, "PASS", "package.json"})
	}
	for _, f := range d.Files {
		rows = append(rows, doctorRow{"file: " + f, "PASS", "present"})
	}
	for _, a := range d.Alerts {
		rows = append(rows, doctorRow{"alert", "WARN", a})
	}
	for _, r := range d.Risks {
		rows = append(rows, doctorRow{"risk", "FAIL", r})
	}

	lines := []string{header}
	nameW := pickDoctorNameWidth(rows, width)
	for _, r := range rows {
		lines = append(lines, doctorRowLine(r, nameW, width))
	}

	ok, warn, fail, skip := tally(rows)
	footer := styles.Success.Render(fmt.Sprintf("%d ok", ok)) +
		styles.Muted.Render(" · ") +
		styles.Warning.Render(fmt.Sprintf("%d warn", warn)) +
		styles.Muted.Render(" · ") +
		styles.Danger.Render(fmt.Sprintf("%d fail", fail)) +
		styles.Muted.Render(" · ") +
		styles.Faint.Render(fmt.Sprintf("%d skipped", skip)) +
		styles.Muted.Render("   ·  run ") +
		styles.Primary.Render("harness doctor --fix") +
		styles.Muted.Render(" to auto-resolve")
	lines = append(lines, "", footer)
	return strings.Join(lines, "\n")
}

func doctorRowLine(r doctorRow, nameW, width int) string {
	glyph := statusGlyph(r.status)
	style := statusStyle(r.status)
	stWidth := 6
	detailW := maxInt(8, width-2-nameW-2-stWidth-2)
	return style.Render(glyph) + " " +
		styles.Text.Render(padRight(r.name, nameW)) + "  " +
		style.Render(padRight(statusLabel(r.status), stWidth)) + "  " +
		styles.Muted.Render(truncate(r.detail, detailW))
}

func pickDoctorNameWidth(rows []doctorRow, width int) int {
	max := 16
	for _, r := range rows {
		if l := runeLen(r.name); l > max {
			max = l
		}
	}
	// Leave room for glyph + status + detail.
	cap := maxInt(12, width-2-8-2-12)
	if max > cap {
		max = cap
	}
	return max
}

func statusOrSkip(value string) string {
	if strings.TrimSpace(value) == "" || value == "-" {
		return "SKIP"
	}
	return "PASS"
}

func tally(rows []doctorRow) (ok, warn, fail, skip int) {
	for _, r := range rows {
		switch normalizeStatus(r.status) {
		case "pass", "passed", "done":
			ok++
		case "warn", "warning":
			warn++
		case "fail", "failed", "error":
			fail++
		case "skip", "skipped":
			skip++
		}
	}
	return
}
