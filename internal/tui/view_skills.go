package tui

import (
	"strings"
)

func (m *model) renderSkillsView(width int) string {
	skills := m.data.Skills
	var sections []string
	sections = append(sections, listSection("Active Skills", skills.Active, "No active skills installed."))
	sections = append(sections, listSection("Suggested", skills.Suggested, "No suggestions."))
	sections = append(sections, listSection("Categories", skills.Categories, "No categories."))
	sections = append(sections, listSection("Adapters", skills.Adapters, "No adapters configured."))
	body := strings.Join(sections, "\n\n")
	return card("Skills", width, fitBlock(body, width-4))
}

func listSection(title string, values []string, empty string) string {
	var lines []string
	lines = append(lines, styles.CardTitle.Render(title))
	if len(values) == 0 {
		lines = append(lines, styles.Muted.Render(empty))
		return strings.Join(lines, "\n")
	}
	for _, value := range values {
		lines = append(lines, styles.Text.Render("- "+value))
	}
	return strings.Join(lines, "\n")
}
