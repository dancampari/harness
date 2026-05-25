// Package traceability persists the lifecycle of every requirement
// declared in a feature spec. TLC's specify.md describes a status
// progression — Pending → In Design → In Tasks → Implementing →
// Verified — and the harness commits that progression to disk in
// .harness/traceability.json so the agent (and the live TUI) can see
// where each REQ-ID is in the pipeline without having to re-parse the
// spec on every read.
//
// The ledger is intentionally minimal: a single JSON file keyed by
// `<slug>:<req-id>`, append-friendly, and safe to read concurrently
// because every write goes through Update (read → mutate → atomic
// write). Pre-Phase-3 projects without a ledger work transparently —
// the first Update creates the file.
package traceability

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Status is one stop on TLC's specify.md status progression. Strings
// match TLC verbatim so the ledger reads naturally in JSON dumps.
type Status string

const (
	StatusPending      Status = "Pending"
	StatusInDesign     Status = "In Design"
	StatusInTasks      Status = "In Tasks"
	StatusImplementing Status = "Implementing"
	StatusVerified     Status = "Verified"
)

// Entry is the persisted state of one requirement.
type Entry struct {
	Slug          string `json:"slug"`
	RequirementID string `json:"requirement_id"`
	Statement     string `json:"statement,omitempty"`
	Status        Status `json:"status"`
	UpdatedAt     string `json:"updated_at"`
}

// Ledger is the JSON payload at .harness/traceability.json.
type Ledger struct {
	SchemaVersion string  `json:"schema_version"`
	Entries       []Entry `json:"entries"`
}

const schemaVersion = "1"

// mu serialises writes within one process. Cross-process concurrent
// writes are not supported — the harness CLI is single-tenant per
// project, so a mutex here is enough.
var mu sync.Mutex

// Path returns the canonical location of the ledger inside harnessDir.
func Path(harnessDir string) string {
	return filepath.Join(harnessDir, "traceability.json")
}

// Load reads the ledger from disk. Missing files return an empty
// ledger so callers can treat "no file" and "empty file" identically.
func Load(harnessDir string) (*Ledger, error) {
	b, err := os.ReadFile(Path(harnessDir))
	if err != nil {
		if os.IsNotExist(err) {
			return &Ledger{SchemaVersion: schemaVersion}, nil
		}
		return nil, err
	}
	var ledger Ledger
	if err := json.Unmarshal(b, &ledger); err != nil {
		return nil, fmt.Errorf("parse traceability ledger: %w", err)
	}
	if ledger.SchemaVersion == "" {
		ledger.SchemaVersion = schemaVersion
	}
	return &ledger, nil
}

// Save writes the ledger atomically: temp file + rename so an
// interrupted run never leaves a partial JSON on disk. The entries are
// sorted by slug+REQ-ID before write so diffs stay stable.
func Save(harnessDir string, ledger *Ledger) error {
	if ledger == nil {
		return fmt.Errorf("nil ledger")
	}
	if ledger.SchemaVersion == "" {
		ledger.SchemaVersion = schemaVersion
	}
	sort.Slice(ledger.Entries, func(i, j int) bool {
		if ledger.Entries[i].Slug != ledger.Entries[j].Slug {
			return ledger.Entries[i].Slug < ledger.Entries[j].Slug
		}
		return ledger.Entries[i].RequirementID < ledger.Entries[j].RequirementID
	})
	if err := os.MkdirAll(harnessDir, 0o755); err != nil {
		return err
	}
	out, err := json.MarshalIndent(ledger, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	target := Path(harnessDir)
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, target)
}

// Update applies a status transition for one requirement and persists
// the ledger. Returns the previous status (empty for new entries) and
// the persisted entry. Concurrent calls in the same process serialise
// through mu; cross-process callers must coordinate externally.
func Update(harnessDir, slug, reqID, statement string, next Status) (Status, Entry, error) {
	if slug == "" || reqID == "" {
		return "", Entry{}, fmt.Errorf("slug and requirement_id are required")
	}
	mu.Lock()
	defer mu.Unlock()

	ledger, err := Load(harnessDir)
	if err != nil {
		return "", Entry{}, err
	}
	var previous Status
	now := time.Now().UTC().Format(time.RFC3339)
	found := false
	for i := range ledger.Entries {
		if ledger.Entries[i].Slug == slug && ledger.Entries[i].RequirementID == reqID {
			previous = ledger.Entries[i].Status
			ledger.Entries[i].Status = next
			ledger.Entries[i].UpdatedAt = now
			if statement != "" {
				ledger.Entries[i].Statement = statement
			}
			found = true
			break
		}
	}
	if !found {
		ledger.Entries = append(ledger.Entries, Entry{
			Slug:          slug,
			RequirementID: reqID,
			Statement:     statement,
			Status:        next,
			UpdatedAt:     now,
		})
	}
	if err := Save(harnessDir, ledger); err != nil {
		return "", Entry{}, err
	}

	for _, e := range ledger.Entries {
		if e.Slug == slug && e.RequirementID == reqID {
			return previous, e, nil
		}
	}
	return previous, Entry{}, nil
}

// BulkRegister bulk-creates Pending entries for every requirement in
// the provided list, skipping IDs that already exist in the ledger.
// Used by Propose to seed the ledger when a spec first lands.
func BulkRegister(harnessDir, slug string, reqs []Requirement) error {
	if len(reqs) == 0 {
		return nil
	}
	mu.Lock()
	defer mu.Unlock()
	ledger, err := Load(harnessDir)
	if err != nil {
		return err
	}
	existing := map[string]bool{}
	for _, e := range ledger.Entries {
		if e.Slug == slug {
			existing[e.RequirementID] = true
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	changed := false
	for _, r := range reqs {
		if r.ID == "" || existing[r.ID] {
			continue
		}
		ledger.Entries = append(ledger.Entries, Entry{
			Slug:          slug,
			RequirementID: r.ID,
			Statement:     r.Statement,
			Status:        StatusPending,
			UpdatedAt:     now,
		})
		changed = true
	}
	if !changed {
		return nil
	}
	return Save(harnessDir, ledger)
}

// Requirement mirrors planner.Requirement at the package boundary so
// callers can pass slices directly without importing planner here
// (which would create an import cycle through agreement).
type Requirement struct {
	ID        string
	Statement string
}

// ForSlug returns the entries for one feature, sorted by REQ-ID.
func (l *Ledger) ForSlug(slug string) []Entry {
	var out []Entry
	for _, e := range l.Entries {
		if e.Slug == slug {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RequirementID < out[j].RequirementID })
	return out
}
