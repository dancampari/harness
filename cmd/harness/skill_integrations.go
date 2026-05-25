package harness

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SkillIntegration describes one third-party skill the harness can
// detect at bootstrap. The agent uses these names to know what
// orchestration helpers are available without having to discover them
// at runtime.
//
// TLC's `## Skill Integrations` row in the unification plan calls out
// mermaid-studio (diagram authoring) and codenavi (code navigation),
// which are typical Claude Code skill packs. The detection is
// best-effort: looking for the canonical `.claude/skills/<name>/`
// directory or its Codex equivalent is enough to decide that the agent
// runtime "has" the skill.
type SkillIntegration struct {
	Name        string
	Description string
	// LookupPaths are project-relative paths checked at bootstrap. The
	// first one that exists wins; an empty list disables the entry.
	LookupPaths []string
}

// KnownSkillIntegrations lists every external skill the harness teaches
// the agent about. Adding a new integration: append a SkillIntegration
// here. The detection routine handles the rest.
var KnownSkillIntegrations = []SkillIntegration{
	{
		Name:        "mermaid-studio",
		Description: "diagram authoring; prefer over inline ASCII when sketching architecture or sequence flows",
		LookupPaths: []string{
			filepath.Join(".claude", "skills", "mermaid-studio"),
			filepath.Join(".codex", "skills", "mermaid-studio"),
		},
	},
	{
		Name:        "codenavi",
		Description: "code navigation; prefer over Grep/Glob for symbol lookups in large codebases",
		LookupPaths: []string{
			filepath.Join(".claude", "skills", "codenavi"),
			filepath.Join(".codex", "skills", "codenavi"),
		},
	},
}

// DetectInstalledSkills returns the subset of KnownSkillIntegrations
// whose LookupPaths resolve to an existing directory in projectRoot.
// Sorted by name so the generated agent doc is deterministic.
func DetectInstalledSkills(projectRoot string) []SkillIntegration {
	var found []SkillIntegration
	for _, skill := range KnownSkillIntegrations {
		for _, rel := range skill.LookupPaths {
			candidate := filepath.Join(projectRoot, rel)
			if info, err := os.Stat(candidate); err == nil && info.IsDir() {
				found = append(found, skill)
				break
			}
		}
	}
	sort.Slice(found, func(i, j int) bool { return found[i].Name < found[j].Name })
	return found
}

// SkillIntegrationsBlock returns the markdown the generated agent docs
// embed when at least one skill integration is detected. When no skills
// are installed the function returns an empty string so the agent doc
// stays compact instead of carrying a stale "none detected" message.
func SkillIntegrationsBlock(projectRoot string) string {
	found := DetectInstalledSkills(projectRoot)
	if len(found) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("## Skill Integrations\n\n")
	sb.WriteString("The following agent-runtime skills are installed in this project. Prefer them over generic tools when their description matches the task:\n\n")
	for _, skill := range found {
		sb.WriteString("- `")
		sb.WriteString(skill.Name)
		sb.WriteString("` — ")
		sb.WriteString(skill.Description)
		sb.WriteString("\n")
	}
	return sb.String()
}
