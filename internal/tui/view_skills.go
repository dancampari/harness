package tui

import (
	"fmt"
	"strings"
)

// renderSkillsView shows two distinct sections so users do not confuse
// skills (agent instructions installed under .harness/skills/) with
// sensor adapters (deterministic tools the evaluator invokes during QA).
// Both are part of the harness, but only the first set is what an agent
// is instructed to read at session start.
func (m *model) renderSkillsView(width int) string {
	skills := m.data.Skills
	if len(skills.Active) == 0 && len(skills.Adapters) == 0 {
		return section("Skills", width) + "\n" + styles.Muted.Render("Nothing installed yet. Run: harness skills install")
	}

	lines := []string{section("Active Skills", width)}
	lines = append(lines, inventoryHeader())
	lines = append(lines, rule(width))
	if len(skills.Active) == 0 {
		lines = append(lines,
			styles.Muted.Render("  No skill packs installed. Run: harness skills install"))
	}
	for _, name := range skills.Active {
		lines = append(lines, inventoryRow(name, "ready", skillDescription(name)))
	}

	lines = append(lines, "", section("Sensor Adapters", width))
	lines = append(lines, inventoryHeader())
	lines = append(lines, rule(width))
	if len(skills.Adapters) == 0 {
		lines = append(lines,
			styles.Muted.Render("  No adapters configured. Run: harness doctor --fix"))
	}
	for _, name := range skills.Adapters {
		lines = append(lines, inventoryRow(name, "configured", "sensor"))
	}

	lines = append(lines, "",
		styles.Muted.Render(fmt.Sprintf(
			"%d skill(s) active · %d adapter(s) configured",
			len(skills.Active), len(skills.Adapters))))
	return strings.Join(lines, "\n")
}

func inventoryHeader() string {
	cols := []string{
		styles.TableHeader.Render(padRight("", 2)),
		styles.TableHeader.Render(padRight("name", 24)),
		styles.TableHeader.Render(padRight("state", 12)),
		styles.TableHeader.Render(padRight("description", 28)),
	}
	return strings.Join(cols, "")
}

func inventoryRow(name, state, desc string) string {
	stateStyle := styles.Faint
	glyph := symbols().Open
	switch state {
	case "ready", "active":
		stateStyle = styles.Success
		glyph = symbols().Dot
	case "configured":
		stateStyle = styles.Text
		glyph = symbols().Open
	case "warn":
		stateStyle = styles.Warning
		glyph = symbols().Dot
	}
	return stateStyle.Render(padRight(glyph, 2)) +
		styles.Text.Render(padRight(name, 24)) +
		stateStyle.Render(padRight(state, 12)) +
		styles.Muted.Render(desc)
}

// skillDescription returns a short one-line summary for each known
// skill. Unknown skill names fall back to a generic label so user-added
// skill packs still render cleanly.
func skillDescription(name string) string {
	switch name {
	case "spec-driven":
		return "Specify → Design → Tasks → Execute → Validate flow"
	case "contract-authoring":
		return "Author sprint contracts that pass review"
	case "contract-review":
		return "Independent tester review before AGREED"
	}
	return "user-installed skill pack"
}
