package tui

import (
	"fmt"
	"strings"
)

func (m *model) renderSkillsView(width int) string {
	header := section("Skills", width)
	skills := m.data.Skills
	if len(skills.Active) == 0 && len(skills.Suggested) == 0 && len(skills.Categories) == 0 && len(skills.Adapters) == 0 {
		return header + "\n" + styles.Muted.Render("No skills installed.")
	}

	cols := []string{
		styles.TableHeader.Render(padRight("", 2)),
		styles.TableHeader.Render(padRight("name", 22)),
		styles.TableHeader.Render(padRight("state", 10)),
		styles.TableHeader.Render(padRight("description", 28)),
	}
	lines := []string{
		header,
		strings.Join(cols, ""),
		rule(width),
	}
	for _, name := range skills.Active {
		lines = append(lines, skillRow(name, "ready", "active skill"))
	}
	for _, name := range skills.Suggested {
		lines = append(lines, skillRow(name, "suggested", "suggestion"))
	}
	for _, name := range skills.Categories {
		lines = append(lines, skillRow(name, "category", "category"))
	}
	for _, name := range skills.Adapters {
		lines = append(lines, skillRow(name, "adapter", "adapter"))
	}

	ready := len(skills.Active)
	total := ready + len(skills.Suggested) + len(skills.Categories) + len(skills.Adapters)
	footerLine := styles.Muted.Render(fmt.Sprintf("%d skills · ", total)) +
		styles.Success.Render(fmt.Sprintf("%d ready", ready)) +
		styles.Muted.Render(" · ") +
		styles.Faint.Render(fmt.Sprintf("%d suggested", len(skills.Suggested)))
	lines = append(lines, "", footerLine)
	return strings.Join(lines, "\n")
}

func skillRow(name, state, desc string) string {
	glyph := symbols().Dot
	stateStyle := styles.Success
	switch state {
	case "ready", "active":
		stateStyle = styles.Success
		glyph = symbols().Dot
	case "warn":
		stateStyle = styles.Warning
	case "off", "category", "adapter":
		stateStyle = styles.Faint
		glyph = symbols().Open
	case "suggested":
		stateStyle = styles.Muted
		glyph = symbols().Open
	}
	return stateStyle.Render(padRight(glyph, 2)) +
		styles.Text.Render(padRight(name, 22)) +
		stateStyle.Render(padRight(state, 10)) +
		styles.Muted.Render(desc)
}
