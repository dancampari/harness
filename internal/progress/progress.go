// Package progress publishes a live snapshot of an in-flight harness QA
// run to .harness/run-progress.json.
//
// The evaluator updates the snapshot as it moves through phases and as
// each sensor starts and finishes. The TUI polls the file so a human —
// or another agent watching the run — can see exactly which phase the
// harness is in and which sensor is executing right now, instead of
// waiting for a silent process to print a final verdict.
//
// The file is a single snapshot document, rewritten atomically on every
// change (write to a temp file, then rename). Readers therefore always
// observe a complete, consistent JSON document, never a partial write.
package progress

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

// SchemaVersion is bumped when the snapshot shape changes.
const SchemaVersion = "1"

// Phase names for a sprint QA run, in order.
const (
	PhaseContract  = "contract"  // parsing and checking the sprint contract
	PhaseSensors   = "sensors"   // running deterministic sensors
	PhaseAggregate = "aggregate" // combining sensor results into dimensions
	PhaseDone      = "done"      // finished; Verdict is populated
)

// Sensor execution states.
const (
	StatePending = "pending" // configured, not started
	StateRunning = "running" // executing now
	StateDone    = "done"    // executed successfully
	StateSkipped = "skipped" // tool unavailable; not executed
	StateError   = "error"   // execution failed
)

// FreshWindow bounds how long after the last update a non-done snapshot
// is still considered an in-flight run. Past this, a run that never
// reached PhaseDone is treated as abandoned (the process likely died).
const FreshWindow = 45 * time.Second

// Sensor is one row of the live progress snapshot.
type Sensor struct {
	Name      string  `json:"name"`
	Dimension string  `json:"dimension"`
	State     string  `json:"state"`
	Duration  float64 `json:"duration_seconds,omitempty"`
}

// Snapshot is the full document written to run-progress.json.
type Snapshot struct {
	SchemaVersion string    `json:"schema_version"`
	SprintNumber  int       `json:"sprint_number"`
	Phase         string    `json:"phase"`
	StartedAt     time.Time `json:"started_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	Verdict       string    `json:"verdict,omitempty"`
	Sensors       []Sensor  `json:"sensors"`
}

// Active reports whether the snapshot represents an in-flight run: it
// has not reached PhaseDone and was updated within FreshWindow.
func (s Snapshot) Active() bool {
	if s.Phase == "" || s.Phase == PhaseDone {
		return false
	}
	if s.UpdatedAt.IsZero() {
		return false
	}
	return time.Since(s.UpdatedAt) < FreshWindow
}

// Read loads a snapshot from path. A missing or unparseable file yields
// the zero Snapshot and false.
func Read(path string) (Snapshot, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Snapshot{}, false
	}
	var s Snapshot
	if err := json.Unmarshal(b, &s); err != nil {
		return Snapshot{}, false
	}
	return s, true
}

// Writer serialises a Snapshot to disk atomically on every mutation.
// All methods are safe for concurrent use and tolerate a nil receiver,
// so callers can pass a nil *Writer to disable progress reporting.
type Writer struct {
	path string
	mu   sync.Mutex
	snap Snapshot
}

// NewWriter creates a progress writer for path and flushes an initial
// snapshot in the contract phase.
func NewWriter(path string, sprintNumber int) *Writer {
	now := time.Now().UTC()
	w := &Writer{
		path: path,
		snap: Snapshot{
			SchemaVersion: SchemaVersion,
			SprintNumber:  sprintNumber,
			Phase:         PhaseContract,
			StartedAt:     now,
			Sensors:       []Sensor{},
		},
	}
	w.update(func() {})
	return w
}

// SetPhase records a coarse pipeline phase transition.
func (w *Writer) SetPhase(phase string) {
	w.update(func() { w.snap.Phase = phase })
}

// RegisterSensors seeds the sensor list. Callers typically register
// available sensors as pending and unavailable ones as skipped so the
// panel shows the full configured set from the start.
func (w *Writer) RegisterSensors(sensors []Sensor) {
	w.update(func() { w.snap.Sensors = append([]Sensor(nil), sensors...) })
}

// SetSensorState updates one sensor's state, matched by name. A zero
// duration leaves the recorded duration untouched.
func (w *Writer) SetSensorState(name, state string, dur time.Duration) {
	w.update(func() {
		for i := range w.snap.Sensors {
			if w.snap.Sensors[i].Name == name {
				w.snap.Sensors[i].State = state
				if dur > 0 {
					w.snap.Sensors[i].Duration = dur.Seconds()
				}
				return
			}
		}
	})
}

// Finish marks the run done and records its final verdict.
func (w *Writer) Finish(verdict string) {
	w.update(func() {
		w.snap.Phase = PhaseDone
		w.snap.Verdict = verdict
	})
}

// update applies mutate under the lock, refreshes UpdatedAt, and flushes
// the snapshot to disk. A nil writer is a no-op so progress reporting
// can be disabled by passing nil.
func (w *Writer) update(mutate func()) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	mutate()
	w.snap.UpdatedAt = time.Now().UTC()
	w.writeLocked()
}

// writeLocked serialises the snapshot and replaces the file atomically.
// Failures are swallowed: progress reporting is best-effort and must
// never break a QA run.
func (w *Writer) writeLocked() {
	body, err := json.MarshalIndent(w.snap, "", "  ")
	if err != nil {
		return
	}
	body = append(body, '\n')
	tmp := w.path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, w.path)
}
