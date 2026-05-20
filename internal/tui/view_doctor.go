package tui

import (
	"strings"
)

func (m *model) renderDoctorView(width int) string {
	d := m.data.Doctor
	var lines []string
	lines = append(lines,
		kv("Stack", defaultString(d.Stack, "unknown"), width-4),
		kv("Package", defaultString(d.PackageManager, "-"), width-4),
	)
	lines = append(lines, "", styles.CardTitle.Render("Validations"))
	lines = appendList(lines, d.Validations, "No validations detected.")
	lines = append(lines, "", styles.CardTitle.Render("Scripts"))
	lines = appendList(lines, d.Scripts, "No scripts detected.")
	lines = append(lines, "", styles.CardTitle.Render("Important Files"))
	lines = appendList(lines, d.Files, "No Harness files detected.")
	if len(d.Alerts) > 0 {
		lines = append(lines, "", styles.CardTitle.Render("Alerts"))
		for _, alert := range d.Alerts {
			lines = append(lines, styles.Warning.Render("- "+alert))
		}
	}
	if len(d.Risks) > 0 {
		lines = append(lines, "", styles.CardTitle.Render("Risks"))
		for _, risk := range d.Risks {
			lines = append(lines, styles.Danger.Render("- "+risk))
		}
	}
	return card("Doctor", width, strings.Join(lines, "\n"))
}

func appendList(lines []string, values []string, empty string) []string {
	if len(values) == 0 {
		return append(lines, styles.Muted.Render(empty))
	}
	for _, value := range values {
		lines = append(lines, styles.Text.Render("- "+value))
	}
	return lines
}
