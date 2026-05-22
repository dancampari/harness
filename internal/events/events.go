// Package events is the harness activity log. Every stage of a sprint —
// contract authoring, the agent's build edits, QA, and scoring — appends
// a line to .harness/events.jsonl so the TUI (and anyone reading the
// file) can see what is happening across the whole pipeline, not just
// during QA.
//
// The log is append-only JSONL. Each Append issues a single small write
// to a file opened with O_APPEND, which the OS delivers atomically, so
// concurrent writers (the agent's hook subprocess and harness commands)
// never interleave a line.
package events

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FileName is the activity log inside the .harness directory.
const FileName = "events.jsonl"

// Pipeline phases an event can belong to.
const (
	PhaseContract = "contract"
	PhaseBuild    = "build"
	PhaseQA       = "qa"
	PhaseReport   = "report"
)

// Event is one entry in the activity log. The JSON shape is compatible
// with the per-run events the TUI already reads.
type Event struct {
	Timestamp time.Time `json:"timestamp"`
	Type      string    `json:"type"`
	Phase     string    `json:"phase,omitempty"`
	Message   string    `json:"message"`
	Agent     string    `json:"agent,omitempty"`
}

// Append writes one event line to <harnessDir>/events.jsonl. A zero
// Timestamp is set to now. Failures are returned but callers generally
// ignore them: activity logging is best-effort and must never break a
// harness command or block an agent's tool call.
func Append(harnessDir string, e Event) error {
	if harnessDir == "" {
		return nil
	}
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	if err := os.MkdirAll(harnessDir, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(harnessDir, FileName),
		os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(line)
	return err
}

// Record is the convenience form used across the CLI: it appends an
// event and swallows any error, because activity logging is advisory.
func Record(harnessDir, eventType, phase, message, agent string) {
	_ = Append(harnessDir, Event{
		Type:    eventType,
		Phase:   phase,
		Message: message,
		Agent:   agent,
	})
}

// Recent returns up to limit most-recent events, newest first. A
// missing log yields an empty slice.
func Recent(harnessDir string, limit int) []Event {
	path := filepath.Join(harnessDir, FileName)
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var all []Event
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e Event
		if json.Unmarshal([]byte(line), &e) != nil {
			continue
		}
		all = append(all, e)
	}
	// Newest first.
	for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
		all[i], all[j] = all[j], all[i]
	}
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all
}
