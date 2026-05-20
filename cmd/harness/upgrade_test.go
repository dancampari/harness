package harness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dancampari/harness/internal/memory"
)

func TestUpgradeRefreshesGeneratedFilesAndPreservesMemory(t *testing.T) {
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
	staleSkill := filepath.Join(".harness", "skills", "contract-authoring", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(staleSkill), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(staleSkill, []byte("old skill\n"), 0o644); err != nil {
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

	gotProgress, err := os.ReadFile(filepath.Join(".harness", "progress.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(gotProgress) != progress {
		t.Fatalf("progress.md was overwritten:\n%s", gotProgress)
	}
	gotContract, err := os.ReadFile(contractPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gotContract), "keep this contract") {
		t.Fatalf("contract was overwritten:\n%s", gotContract)
	}
	gotSkill, err := os.ReadFile(staleSkill)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gotSkill), "# Harness Contract Authoring") {
		t.Fatalf("expected generated skill to be refreshed, got:\n%s", gotSkill)
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
