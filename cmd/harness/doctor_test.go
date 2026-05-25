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
	writeDoctorFile(t, root, ".harness/agent-protocol.md", "harness.repair\nfeature repair\n")
	writeDoctorFile(t, root, ".harness/.gitignore", generatedHarnessIgnoreForTest)

	err := runDoctorWithOptions(root, doctorOptions{Strict: true})
	if err == nil || !strings.Contains(err.Error(), "doctor strict failed") {
		t.Fatalf("expected strict doctor failure, got %v", err)
	}
}

func TestDoctorStrictAllowsContractOnlyHarnessWithWarnings(t *testing.T) {
	root := t.TempDir()
	writeDoctorHarnessConfig(t, root, config.DefaultFor("unknown"))
	writeDoctorFile(t, root, ".harness/agent-protocol.md", "harness.repair\nfeature repair\n")
	writeDoctorFile(t, root, ".harness/.gitignore", generatedHarnessIgnoreForTest)

	if err := runDoctorWithOptions(root, doctorOptions{Strict: true}); err != nil {
		t.Fatalf("expected contract-only strict doctor to pass, got %v", err)
	}
}

func TestDoctorFixUpgradesContractOnlyTypeScriptConfig(t *testing.T) {
	root := t.TempDir()
	writeDoctorFile(t, root, "package.json", `{"name":"demo","devDependencies":{"vitest":"latest","eslint":"latest"}}`)
	writeDoctorFile(t, root, "tsconfig.json", `{}`)
	writeDoctorHarnessConfig(t, root, config.DefaultFor("unknown"))
	writeDoctorFile(t, root, ".harness/agent-protocol.md", "harness.repair\nfeature repair\n")

	if err := runDoctorWithOptions(root, doctorOptions{Fix: true}); err != nil {
		t.Fatalf("doctor --fix failed: %v", err)
	}

	cfg, err := config.Load(filepath.Join(root, ".harness", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Stack != "typescript" {
		t.Fatalf("expected stack to be fixed to typescript, got %q", cfg.Stack)
	}
	for _, dim := range []string{
		config.DimCorrectness,
		config.DimCoverage,
		config.DimComplexity,
		config.DimSecurity,
		config.DimArchitecture,
		config.DimContract,
		config.DimE2E,
	} {
		if cfg.ThresholdFor(dim) == 0 || cfg.WeightFor(dim) == 0 {
			t.Fatalf("expected %s to be active after doctor --fix", dim)
		}
	}
	for _, sensor := range []string{"eslint", "vitest", "vitest-coverage", "npm-audit", "js-complexity", "js-architecture", "playwright"} {
		if !containsString(cfg.AllAdapterNames(), sensor) {
			t.Fatalf("expected adapter %q after doctor --fix, got %#v", sensor, cfg.AllAdapterNames())
		}
	}
	if !hasGeneratedIgnore(filepath.Join(root, ".harness", ".gitignore")) {
		t.Fatal("expected doctor --fix to repair .harness/.gitignore")
	}
}

func TestDoctorFixCreatesMissingConfigWhenHarnessExists(t *testing.T) {
	root := t.TempDir()
	writeDoctorFile(t, root, "package.json", `{"name":"demo"}`)
	writeDoctorFile(t, root, "tsconfig.json", `{}`)
	if err := os.MkdirAll(filepath.Join(root, ".harness"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := runDoctorWithOptions(root, doctorOptions{Fix: true}); err != nil {
		t.Fatalf("doctor --fix failed: %v", err)
	}
	cfg, err := config.Load(filepath.Join(root, ".harness", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Stack != "typescript" || len(cfg.AllAdapterNames()) == 0 {
		t.Fatalf("expected generated TypeScript config with adapters, got stack=%q adapters=%#v", cfg.Stack, cfg.AllAdapterNames())
	}
}

func TestDoctorStrictFailsOnStaleInstalledSkills(t *testing.T) {
	root := t.TempDir()
	writeDoctorHarnessConfig(t, root, config.DefaultFor("unknown"))
	writeDoctorFile(t, root, ".harness/agent-protocol.md", "harness.repair\nfeature repair\n")
	writeDoctorFile(t, root, ".harness/.gitignore", generatedHarnessIgnoreForTest)
	writeDoctorFile(t, root, ".harness/setup.json", `{"contract_skills_enabled":true}`)
	writeDoctorFile(t, root, ".harness/skills/harness-gate/SKILL.md", "stale gate skill missing required sections\n")

	err := runDoctorWithOptions(root, doctorOptions{Strict: true})
	if err == nil || !strings.Contains(err.Error(), "doctor strict failed") {
		t.Fatalf("expected stale skills to fail strict doctor, got %v", err)
	}
}

func TestDoctorStrictPassesSpecDrivenPlanningArtifacts(t *testing.T) {
	root := t.TempDir()
	harnessDir := filepath.Join(root, ".harness")
	writeDoctorHarnessConfig(t, root, config.DefaultFor("unknown"))
	writeDoctorFile(t, root, ".harness/.gitignore", generatedHarnessIgnoreForTest)
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
	writeDoctorFile(t, root, ".harness/agent-protocol.md", "harness.repair\nfeature repair\n")
	writeDoctorFile(t, root, ".harness/.gitignore", generatedHarnessIgnoreForTest)
	writeDoctorFile(t, root, ".harness/setup.json", `{"planning_mode":"spec-driven","contract_skills_enabled":true,"coding_cli":"none"}`)
	writeDoctorFile(t, root, ".harness/skills/tlc-spec-driven/SKILL.md", "old skill missing phases\n")
	writeDoctorFile(t, root, ".harness/skills/harness-gate/SKILL.md", "old gate skill missing required sections\n")

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

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

const generatedHarnessIgnoreForTest = "memory.db\nreports/\nrepairs/\nscreenshots/\ntmp/\n"
