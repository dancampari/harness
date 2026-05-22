package progress

import (
	"path/filepath"
	"testing"
	"time"
)

func TestNewWriterFlushesContractPhase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run-progress.json")
	NewWriter(path, 7)

	snap, ok := Read(path)
	if !ok {
		t.Fatal("expected run-progress.json to exist after NewWriter")
	}
	if snap.Phase != PhaseContract {
		t.Fatalf("expected initial phase %q, got %q", PhaseContract, snap.Phase)
	}
	if snap.SprintNumber != 7 {
		t.Fatalf("expected sprint number 7, got %d", snap.SprintNumber)
	}
	if snap.SchemaVersion != SchemaVersion {
		t.Fatalf("expected schema %q, got %q", SchemaVersion, snap.SchemaVersion)
	}
}

func TestWriterTracksPhaseAndSensorStates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run-progress.json")
	w := NewWriter(path, 1)

	w.RegisterSensors([]Sensor{
		{Name: "eslint", Dimension: "correctness", State: StatePending},
		{Name: "vitest", Dimension: "correctness", State: StatePending},
		{Name: "playwright", Dimension: "e2e", State: StateSkipped},
	})
	w.SetPhase(PhaseSensors)
	w.SetSensorState("eslint", StateRunning, 0)
	w.SetSensorState("eslint", StateDone, 1200*time.Millisecond)
	w.SetSensorState("vitest", StateRunning, 0)

	snap, ok := Read(path)
	if !ok {
		t.Fatal("expected snapshot on disk")
	}
	if snap.Phase != PhaseSensors {
		t.Fatalf("expected sensors phase, got %q", snap.Phase)
	}
	byName := map[string]Sensor{}
	for _, s := range snap.Sensors {
		byName[s.Name] = s
	}
	if byName["eslint"].State != StateDone {
		t.Fatalf("expected eslint done, got %q", byName["eslint"].State)
	}
	if byName["eslint"].Duration < 1.1 || byName["eslint"].Duration > 1.3 {
		t.Fatalf("expected eslint duration ~1.2s, got %v", byName["eslint"].Duration)
	}
	if byName["vitest"].State != StateRunning {
		t.Fatalf("expected vitest running, got %q", byName["vitest"].State)
	}
	if byName["playwright"].State != StateSkipped {
		t.Fatalf("expected playwright skipped, got %q", byName["playwright"].State)
	}
}

func TestActiveTrueWhileRunningFalseWhenDone(t *testing.T) {
	running := Snapshot{Phase: PhaseSensors, UpdatedAt: time.Now()}
	if !running.Active() {
		t.Fatal("expected a freshly updated sensors-phase snapshot to be active")
	}

	done := Snapshot{Phase: PhaseDone, UpdatedAt: time.Now(), Verdict: "PASS"}
	if done.Active() {
		t.Fatal("expected a done snapshot to be inactive")
	}

	stale := Snapshot{Phase: PhaseSensors, UpdatedAt: time.Now().Add(-2 * FreshWindow)}
	if stale.Active() {
		t.Fatal("expected a stale snapshot (process likely died) to be inactive")
	}

	empty := Snapshot{}
	if empty.Active() {
		t.Fatal("expected the zero snapshot to be inactive")
	}
}

func TestFinishRecordsVerdict(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run-progress.json")
	w := NewWriter(path, 3)
	w.SetPhase(PhaseSensors)
	w.Finish("PASS")

	snap, ok := Read(path)
	if !ok {
		t.Fatal("expected snapshot on disk")
	}
	if snap.Phase != PhaseDone {
		t.Fatalf("expected done phase, got %q", snap.Phase)
	}
	if snap.Verdict != "PASS" {
		t.Fatalf("expected verdict PASS, got %q", snap.Verdict)
	}
	if snap.Active() {
		t.Fatal("a finished snapshot must not report Active")
	}
}

func TestNilWriterIsNoOp(t *testing.T) {
	var w *Writer
	// None of these must panic on a nil receiver.
	w.SetPhase(PhaseSensors)
	w.RegisterSensors([]Sensor{{Name: "eslint"}})
	w.SetSensorState("eslint", StateRunning, 0)
	w.Finish("PASS")
}

func TestReadMissingFile(t *testing.T) {
	if _, ok := Read(filepath.Join(t.TempDir(), "absent.json")); ok {
		t.Fatal("expected Read of a missing file to return ok=false")
	}
}
