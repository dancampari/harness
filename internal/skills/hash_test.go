package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVendoredHashStableAcrossCalls(t *testing.T) {
	first, err := VendoredHash("tlc-spec-driven")
	if err != nil {
		t.Fatal(err)
	}
	second, err := VendoredHash("tlc-spec-driven")
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("VendoredHash is not stable: %q vs %q", first, second)
	}
	if len(first) != 64 {
		t.Fatalf("expected 64-char sha256 hex, got %d chars", len(first))
	}
}

func TestInstalledHashMatchesVendoredAfterInstall(t *testing.T) {
	root := t.TempDir()
	if err := Install(root); err != nil {
		t.Fatal(err)
	}
	for _, pack := range Packs() {
		vendored, err := VendoredHash(pack.Name)
		if err != nil {
			t.Fatal(err)
		}
		installed, err := InstalledHash(root, pack.Name)
		if err != nil {
			t.Fatal(err)
		}
		if installed != vendored {
			t.Fatalf("%s: post-install hash %s != vendored %s",
				pack.Name, installed, vendored)
		}
	}
}

func TestInstalledHashDetectsDrift(t *testing.T) {
	root := t.TempDir()
	if err := Install(root); err != nil {
		t.Fatal(err)
	}
	skillPath := filepath.Join(root, "skills", "tlc-spec-driven", "SKILL.md")
	if err := os.WriteFile(skillPath, []byte("tampered\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	vendored, err := VendoredHash("tlc-spec-driven")
	if err != nil {
		t.Fatal(err)
	}
	drifted, err := InstalledHash(root, "tlc-spec-driven")
	if err != nil {
		t.Fatal(err)
	}
	if drifted == vendored {
		t.Fatal("expected drift to change the installed hash")
	}
}

func TestInstalledHashEmptyWhenPackMissing(t *testing.T) {
	root := t.TempDir()
	got, err := InstalledHash(root, "tlc-spec-driven")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("expected empty hash for missing pack, got %q", got)
	}
}

func TestVendoredHashRejectsUnknownPack(t *testing.T) {
	if _, err := VendoredHash("nope"); err == nil {
		t.Fatal("expected error for unknown pack name")
	}
}
