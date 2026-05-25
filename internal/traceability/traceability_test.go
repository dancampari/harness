package traceability

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUpdateCreatesAndUpdatesEntry(t *testing.T) {
	root := t.TempDir()
	prev, entry, err := Update(root, "sprint-001", "REQ-001", "ships", StatusPending)
	if err != nil {
		t.Fatal(err)
	}
	if prev != "" {
		t.Fatalf("expected empty previous for new entry, got %q", prev)
	}
	if entry.Status != StatusPending || entry.Statement != "ships" {
		t.Fatalf("unexpected entry: %+v", entry)
	}

	prev, entry, err = Update(root, "sprint-001", "REQ-001", "", StatusImplementing)
	if err != nil {
		t.Fatal(err)
	}
	if prev != StatusPending {
		t.Fatalf("expected previous Pending, got %q", prev)
	}
	if entry.Status != StatusImplementing || entry.Statement != "ships" {
		t.Fatalf("statement should be preserved when empty is passed; got %+v", entry)
	}
}

func TestUpdateRequiresKeys(t *testing.T) {
	root := t.TempDir()
	if _, _, err := Update(root, "", "REQ-001", "x", StatusPending); err == nil {
		t.Fatal("expected error for empty slug")
	}
	if _, _, err := Update(root, "sprint-001", "", "x", StatusPending); err == nil {
		t.Fatal("expected error for empty REQ-ID")
	}
}

func TestBulkRegisterSkipsExisting(t *testing.T) {
	root := t.TempDir()
	if _, _, err := Update(root, "sprint-001", "REQ-001", "ships", StatusImplementing); err != nil {
		t.Fatal(err)
	}
	err := BulkRegister(root, "sprint-001", []Requirement{
		{ID: "REQ-001", Statement: "ships (updated)"},
		{ID: "REQ-002", Statement: "exports"},
	})
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	entries := ledger.ForSlug("sprint-001")
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries after bulk, got %d", len(entries))
	}
	for _, e := range entries {
		if e.RequirementID == "REQ-001" && e.Status != StatusImplementing {
			t.Fatalf("BulkRegister overrode existing status: %+v", e)
		}
		if e.RequirementID == "REQ-002" && e.Status != StatusPending {
			t.Fatalf("BulkRegister should seed REQ-002 Pending, got %+v", e)
		}
	}
}

func TestSavePersistsAtomically(t *testing.T) {
	root := t.TempDir()
	if _, _, err := Update(root, "sprint-001", "REQ-001", "ships", StatusPending); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "traceability.json")); err != nil {
		t.Fatalf("ledger file missing after Update: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "traceability.json.tmp")); err == nil {
		t.Fatal("temp file should have been renamed away")
	}
}

func TestForSlugFilters(t *testing.T) {
	root := t.TempDir()
	for _, kv := range []struct{ slug, req string }{
		{"sprint-001", "REQ-001"},
		{"sprint-001", "REQ-002"},
		{"sprint-002", "REQ-001"},
	} {
		if _, _, err := Update(root, kv.slug, kv.req, "x", StatusPending); err != nil {
			t.Fatal(err)
		}
	}
	ledger, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := ledger.ForSlug("sprint-001"); len(got) != 2 {
		t.Fatalf("expected 2 entries for sprint-001, got %d", len(got))
	}
	if got := ledger.ForSlug("sprint-002"); len(got) != 1 {
		t.Fatalf("expected 1 entry for sprint-002, got %d", len(got))
	}
}
