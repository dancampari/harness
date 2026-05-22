package events

import (
	"path/filepath"
	"testing"
	"time"
)

func TestAppendAndRecentRoundTrip(t *testing.T) {
	dir := t.TempDir()
	Record(dir, "contract.created", PhaseContract, "sprint 001", "")
	Record(dir, "agent.edit", PhaseBuild, "src/feature.ts", "codex")
	Record(dir, "qa.finished", PhaseQA, "PASS", "")

	recent := Recent(dir, 10)
	if len(recent) != 3 {
		t.Fatalf("expected 3 events, got %d", len(recent))
	}
	// Newest first.
	if recent[0].Type != "qa.finished" {
		t.Fatalf("expected newest event qa.finished, got %q", recent[0].Type)
	}
	if recent[2].Type != "contract.created" {
		t.Fatalf("expected oldest event contract.created, got %q", recent[2].Type)
	}
	if recent[1].Phase != PhaseBuild || recent[1].Message != "src/feature.ts" {
		t.Fatalf("agent.edit event lost fields: %+v", recent[1])
	}
	if recent[1].Agent != "codex" {
		t.Fatalf("expected agent codex, got %q", recent[1].Agent)
	}
}

func TestRecentLimitKeepsNewest(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 20; i++ {
		Record(dir, "agent.edit", PhaseBuild, "file", "")
	}
	if got := Recent(dir, 5); len(got) != 5 {
		t.Fatalf("expected Recent to cap at 5, got %d", len(got))
	}
}

func TestRecentMissingLog(t *testing.T) {
	if got := Recent(t.TempDir(), 10); len(got) != 0 {
		t.Fatalf("expected no events for a missing log, got %d", len(got))
	}
}

func TestAppendSetsTimestamp(t *testing.T) {
	dir := t.TempDir()
	before := time.Now().Add(-time.Second)
	if err := Append(dir, Event{Type: "agent.edit", Phase: PhaseBuild, Message: "x"}); err != nil {
		t.Fatal(err)
	}
	recent := Recent(dir, 1)
	if len(recent) != 1 {
		t.Fatalf("expected 1 event, got %d", len(recent))
	}
	if recent[0].Timestamp.Before(before) {
		t.Fatalf("expected Append to stamp a recent time, got %v", recent[0].Timestamp)
	}
}

func TestAppendEmptyHarnessDirIsNoOp(t *testing.T) {
	if err := Append("", Event{Type: "agent.edit"}); err != nil {
		t.Fatalf("expected empty harnessDir to be a silent no-op, got %v", err)
	}
}

func TestEventsFileNameStable(t *testing.T) {
	// The TUI reads this exact filename; guard against accidental renames.
	if FileName != "events.jsonl" {
		t.Fatalf("events file name changed to %q; the TUI reader expects events.jsonl", FileName)
	}
	_ = filepath.Join("x", FileName)
}
