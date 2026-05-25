package harness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dancampari/harness/internal/memory"
)

func TestUpgradeRefreshesGeneratedFilesAndPreservesMemory(t *testing.T) {
	stubInstallableHarnessExecutable(t)
	root := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	}()

	if err := os.WriteFile("package.json", []byte(`{"name":"demo","devDependencies":{"vitest":"latest"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureHarnessSkeleton(".harness", PlanningSpecDriven); err != nil {
		t.Fatal(err)
	}
	progress := "# Project Progress\n\nkeep this narrative\n"
	if err := os.WriteFile(filepath.Join(".harness", "progress.md"), []byte(progress), 0o644); err != nil {
		t.Fatal(err)
	}
	contractPath := filepath.Join(".harness", "contracts", "sprint-001.md")
	if err := os.WriteFile(contractPath, []byte("# Sprint 001\n\nkeep this contract\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(".harness", "setup.json"), []byte(`{"coding_cli":"codex","planning_mode":"spec-driven","contract_skills_enabled":true,"install_scope":"project"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	staleLegacy := filepath.Join(".harness", "skills", "contract-authoring", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(staleLegacy), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(staleLegacy, []byte("old legacy skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := memory.Open(filepath.Join(".harness", "memory.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	if err := runUpgrade(upgradeOptions{Yes: true}, "test"); err != nil {
		t.Fatal(err)
	}

	// Phase 2: upgrade migrates legacy artifacts into the canonical
	// .specs/ tree. progress.md becomes STATE.md, contracts/sprint-NNN.md
	// becomes features/sprint-NNN/spec.md. Content must round-trip
	// losslessly; the legacy files must no longer exist.
	migratedState, err := os.ReadFile(filepath.Join(".specs", "project", "STATE.md"))
	if err != nil {
		t.Fatalf("expected progress.md to be migrated to .specs/project/STATE.md: %v", err)
	}
	if string(migratedState) != progress {
		t.Fatalf("STATE.md content drift after migration:\n%s", migratedState)
	}
	if _, err := os.Stat(filepath.Join(".harness", "progress.md")); !os.IsNotExist(err) {
		t.Fatalf("expected legacy .harness/progress.md to be removed, got err=%v", err)
	}
	migratedSpec, err := os.ReadFile(filepath.Join(".specs", "features", "sprint-001", "spec.md"))
	if err != nil {
		t.Fatalf("expected contract to be migrated to .specs/features/sprint-001/spec.md: %v", err)
	}
	if !strings.Contains(string(migratedSpec), "keep this contract") {
		t.Fatalf("migrated spec content drift:\n%s", migratedSpec)
	}
	if _, err := os.Stat(contractPath); !os.IsNotExist(err) {
		t.Fatalf("expected legacy .harness/contracts/sprint-001.md to be removed, got err=%v", err)
	}
	if _, err := os.Stat(staleLegacy); !os.IsNotExist(err) {
		t.Fatalf("expected legacy skill dir to be removed on upgrade, got err=%v", err)
	}
	gateSkill := filepath.Join(".harness", "skills", "harness-gate", "SKILL.md")
	gotGate, err := os.ReadFile(gateSkill)
	if err != nil {
		t.Fatalf("expected canonical harness-gate skill to be installed: %v", err)
	}
	if !strings.Contains(string(gotGate), "Agreement gate") {
		t.Fatalf("expected harness-gate skill to be refreshed, got:\n%s", gotGate)
	}
	tlcSkill := filepath.Join(".harness", "skills", "tlc-spec-driven", "SKILL.md")
	if _, err := os.Stat(tlcSkill); err != nil {
		t.Fatalf("expected canonical tlc-spec-driven skill to be installed: %v", err)
	}
	agents, err := os.ReadFile("AGENTS.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(agents), "## Harness Gate") || !strings.Contains(string(agents), "sprint repair") {
		t.Fatalf("expected Codex reference to be refreshed, got:\n%s", agents)
	}
	if _, err := os.Stat(filepath.Join(".harness", "memory.db")); err != nil {
		t.Fatalf("memory.db missing after upgrade: %v", err)
	}
}

func TestUpgradeAutoScopeUpdatesExistingGlobalHarness(t *testing.T) {
	original := lookPath
	lookPath = func(file string) (string, error) {
		return filepath.Join(t.TempDir(), "harness.cmd"), nil
	}
	defer func() {
		lookPath = original
	}()

	scope, err := resolveUpgradeScope("auto", setupState{InstallScope: "project"})
	if err != nil {
		t.Fatal(err)
	}
	if scope != "global" {
		t.Fatalf("expected existing PATH harness to force global upgrade, got %q", scope)
	}
}
