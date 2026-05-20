package harness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dancampari/harness/internal/config"
)

func TestDoctorStrictFailsWhenActiveSensorsAreUnavailable(t *testing.T) {
	root := t.TempDir()
	writeDoctorFile(t, root, "package.json", `{"name":"demo","devDependencies":{}}`)
	writeDoctorHarnessConfig(t, root, config.DefaultFor("typescript"))
	writeDoctorFile(t, root, ".harness/agent-protocol.md", "harness.repair\nsprint repair\n")
	writeDoctorFile(t, root, ".harness/.gitignore", "memory.db\nreports/\nrepairs/\nscreenshots/\n")

	err := runDoctorWithOptions(root, doctorOptions{Strict: true})
	if err == nil || !strings.Contains(err.Error(), "doctor strict failed") {
		t.Fatalf("expected strict doctor failure, got %v", err)
	}
}

func TestDoctorStrictAllowsContractOnlyHarnessWithWarnings(t *testing.T) {
	root := t.TempDir()
	writeDoctorHarnessConfig(t, root, config.DefaultFor("unknown"))
	writeDoctorFile(t, root, ".harness/agent-protocol.md", "harness.repair\nsprint repair\n")
	writeDoctorFile(t, root, ".harness/.gitignore", "memory.db\nreports/\nrepairs/\nscreenshots/\n")

	if err := runDoctorWithOptions(root, doctorOptions{Strict: true}); err != nil {
		t.Fatalf("expected contract-only strict doctor to pass, got %v", err)
	}
}

func TestDoctorStrictFailsOnStaleInstalledSkills(t *testing.T) {
	root := t.TempDir()
	writeDoctorHarnessConfig(t, root, config.DefaultFor("unknown"))
	writeDoctorFile(t, root, ".harness/agent-protocol.md", "harness.repair\nsprint repair\n")
	writeDoctorFile(t, root, ".harness/.gitignore", "memory.db\nreports/\nrepairs/\nscreenshots/\n")
	writeDoctorFile(t, root, ".harness/setup.json", `{"contract_skills_enabled":true}`)
	writeDoctorFile(t, root, ".harness/skills/contract-authoring/SKILL.md", "old skill without repair loop\n")

	err := runDoctorWithOptions(root, doctorOptions{Strict: true})
	if err == nil || !strings.Contains(err.Error(), "doctor strict failed") {
		t.Fatalf("expected stale skills to fail strict doctor, got %v", err)
	}
}

func TestDoctorStrictPassesSpecDrivenPlanningArtifacts(t *testing.T) {
	root := t.TempDir()
	harnessDir := filepath.Join(root, ".harness")
	writeDoctorHarnessConfig(t, root, config.DefaultFor("unknown"))
	writeDoctorFile(t, root, ".harness/.gitignore", "memory.db\nreports/\nrepairs/\nscreenshots/\n")
	writeDoctorFile(t, root, ".harness/setup.json", `{"planning_mode":"spec-driven","contract_skills_enabled":true,"coding_cli":"none"}`)
	if err := runInstallSkillsWithOptions(harnessDir, false, PlanningSpecDriven); err != nil {
		t.Fatal(err)
	}

	if err := runDoctorWithOptions(root, doctorOptions{Strict: true}); err != nil {
		t.Fatalf("expected spec-driven doctor to pass, got %v", err)
	}
}

func TestDoctorStrictFailsOnStaleSpecDrivenSkill(t *testing.T) {
	root := t.TempDir()
	writeDoctorHarnessConfig(t, root, config.DefaultFor("unknown"))
	writeDoctorFile(t, root, ".harness/agent-protocol.md", "harness.repair\nsprint repair\n")
	writeDoctorFile(t, root, ".harness/.gitignore", "memory.db\nreports/\nrepairs/\nscreenshots/\n")
	writeDoctorFile(t, root, ".harness/setup.json", `{"planning_mode":"spec-driven","contract_skills_enabled":true,"coding_cli":"none"}`)
	writeDoctorFile(t, root, ".harness/skills/spec-driven/SKILL.md", "old skill\n")

	err := runDoctorWithOptions(root, doctorOptions{Strict: true})
	if err == nil || !strings.Contains(err.Error(), "doctor strict failed") {
		t.Fatalf("expected stale spec-driven skill to fail strict doctor, got %v", err)
	}
}

func writeDoctorHarnessConfig(t *testing.T, root string, cfg config.Config) {
	t.Helper()
	path := filepath.Join(root, ".harness", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
}

func writeDoctorFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
