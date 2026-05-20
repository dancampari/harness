package harness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureAgentProtocolModeRefreshesStaleGeneratedProtocol(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".harness")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	stale := `# Harness Agent Protocol

Contract automation skills are enabled.

- .harness/skills/contract-authoring/SKILL.md

Run harness sprint score before declaring the work complete.
`
	path := filepath.Join(root, "agent-protocol.md")
	if err := os.WriteFile(path, []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := ensureAgentProtocolMode(root, PlanningSpecDriven); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, expected := range []string{"harness.repair", "sprint repair", ".harness/repairs/latest.md", "sprint score` only after QA", ".harness/skills/spec-driven/SKILL.md"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected refreshed protocol to contain %q\n%s", expected, text)
		}
	}
}

func TestInstallSpecDrivenSkillsWritesHarnessNativePack(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".harness")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := runInstallSkillsWithOptions(root, false, PlanningSpecDriven); err != nil {
		t.Fatal(err)
	}

	for _, rel := range []string{
		"skills/spec-driven/SKILL.md",
		"skills/spec-driven/references/specify.md",
		"skills/spec-driven/references/design.md",
		"skills/spec-driven/references/tasks.md",
		"skills/spec-driven/references/execute.md",
		"skills/spec-driven/references/validate.md",
		"skills/contract-authoring/SKILL.md",
		"skills/contract-review/SKILL.md",
		"context/STACK.md",
		"context/ARCHITECTURE.md",
		"design",
		"tasks",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("expected %s to exist: %v", rel, err)
		}
	}
	b, err := os.ReadFile(filepath.Join(root, "skills", "spec-driven", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, expected := range []string{"Specify", "Design", "Tasks", "Execute", "Validate", ".harness/contracts/sprint-NNN.md"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected spec-driven skill to contain %q\n%s", expected, text)
		}
	}
}

func TestInstallContractSkillsDoesNotWriteSpecDrivenPack(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".harness")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := runInstallSkillsWithOptions(root, false, PlanningContract); err != nil {
		t.Fatal(err)
	}
	if !skillsInstalled(root) {
		t.Fatal("expected contract skills to be installed")
	}
	if specDrivenSkillsInstalled(root) {
		t.Fatal("did not expect spec-driven skills in contract-only mode")
	}
}
