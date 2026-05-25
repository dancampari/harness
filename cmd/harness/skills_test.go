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

Run harness feature score before declaring the work complete.
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
	for _, expected := range []string{"harness.repair", "feature repair", ".harness/repairs/latest.md", "feature score` only after QA", ".harness/skills/tlc-spec-driven/SKILL.md", ".harness/skills/harness-gate/SKILL.md"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected refreshed protocol to contain %q\n%s", expected, text)
		}
	}
}

func TestInstallSpecDrivenSkillsWritesCanonicalPacks(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".harness")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := runInstallSkillsWithOptions(root, false, PlanningSpecDriven); err != nil {
		t.Fatal(err)
	}

	for _, rel := range []string{
		"skills/tlc-spec-driven/SKILL.md",
		"skills/tlc-spec-driven/references/specify.md",
		"skills/tlc-spec-driven/references/design.md",
		"skills/tlc-spec-driven/references/tasks.md",
		"skills/tlc-spec-driven/references/implement.md",
		"skills/tlc-spec-driven/references/validate.md",
		"skills/harness-gate/SKILL.md",
		"context",
		"design",
		"tasks",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("expected %s to exist: %v", rel, err)
		}
	}
	b, err := os.ReadFile(filepath.Join(root, "skills", "tlc-spec-driven", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(b)
	for _, expected := range []string{"Specify", "Design", "Tasks", "Execute"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected tlc-spec-driven skill to contain %q", expected)
		}
	}
	gate, err := os.ReadFile(filepath.Join(root, "skills", "harness-gate", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	gateText := string(gate)
	for _, expected := range []string{"Agreement gate", "QA dimensions", "Events log"} {
		if !strings.Contains(gateText, expected) {
			t.Fatalf("expected harness-gate skill to contain %q", expected)
		}
	}
}

func TestInstallContractModeInstallsSameCanonicalPacks(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".harness")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := runInstallSkillsWithOptions(root, false, PlanningContract); err != nil {
		t.Fatal(err)
	}
	if !skillsInstalled(root) {
		t.Fatal("expected canonical skill packs to be installed in contract mode")
	}
	if !specDrivenSkillsInstalled(root) {
		t.Fatal("expected tlc-spec-driven + harness-gate to be installed in contract mode")
	}
}

func TestInstallSkillsRemovesLegacyDirs(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".harness")
	for _, legacy := range []string{"spec-driven", "contract-authoring", "contract-review"} {
		if err := os.MkdirAll(filepath.Join(root, "skills", legacy), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "skills", legacy, "SKILL.md"), []byte("old\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := runInstallSkillsWithOptions(root, false, PlanningSpecDriven); err != nil {
		t.Fatal(err)
	}

	for _, legacy := range []string{"spec-driven", "contract-authoring", "contract-review"} {
		if _, err := os.Stat(filepath.Join(root, "skills", legacy)); !os.IsNotExist(err) {
			t.Fatalf("expected legacy skill dir %s to be removed", legacy)
		}
	}
}
