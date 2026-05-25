package harness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectInstalledSkillsFindsKnownIntegrations(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{
		filepath.Join(root, ".claude", "skills", "mermaid-studio"),
		filepath.Join(root, ".codex", "skills", "codenavi"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	got := DetectInstalledSkills(root)
	if len(got) != 2 {
		t.Fatalf("expected 2 skills, got %d: %#v", len(got), got)
	}
	if got[0].Name != "codenavi" || got[1].Name != "mermaid-studio" {
		t.Fatalf("expected alphabetical order, got %#v", got)
	}
}

func TestSkillIntegrationsBlockEmptyWhenNoneDetected(t *testing.T) {
	root := t.TempDir()
	if block := SkillIntegrationsBlock(root); block != "" {
		t.Fatalf("expected empty block when no skills installed, got %q", block)
	}
}

func TestSkillIntegrationsBlockRendersBullet(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".claude", "skills", "mermaid-studio"), 0o755); err != nil {
		t.Fatal(err)
	}
	block := SkillIntegrationsBlock(root)
	if !strings.Contains(block, "## Skill Integrations") {
		t.Fatalf("expected heading, got:\n%s", block)
	}
	if !strings.Contains(block, "`mermaid-studio`") {
		t.Fatalf("expected mermaid-studio bullet, got:\n%s", block)
	}
}
