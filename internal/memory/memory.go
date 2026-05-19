// Package memory is the persistent storage layer for the harness.
//
// Two stores coexist:
//   - memory.db (SQLite): indexed history for fast queries (trend, fingerprint
//     recurrence, score series). Local-only by default.
//   - progress.md (markdown): narrative brain, versioned in git, read by
//     CLIs as the first source of project context.
//
// This package handles only memory.db. progress.md is handled by the
// reporter package (it's a write-only narrative log).
package memory

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // sqlite driver
)

// DB wraps a sqlite connection with harness-specific queries.
type DB struct {
	conn *sql.DB
	path string
}

// Open opens or creates the harness memory database at path.
func Open(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := conn.Ping(); err != nil {
		return nil, err
	}
	return &DB{conn: conn, path: path}, nil
}

// Close releases the database handle.
func (db *DB) Close() error { return db.conn.Close() }

// Migrate creates the schema if missing. Safe to call repeatedly.
func (db *DB) Migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS runs (
			id INTEGER PRIMARY KEY,
			timestamp TEXT NOT NULL,
			git_sha TEXT,
			branch TEXT,
			sprint_number INTEGER,
			trigger TEXT,
			verdict TEXT,
			score_total INTEGER,
			scores_json TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_runs_ts ON runs(timestamp DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_runs_sprint ON runs(sprint_number)`,

		`CREATE TABLE IF NOT EXISTS findings (
			id TEXT PRIMARY KEY,
			run_id INTEGER REFERENCES runs(id) ON DELETE CASCADE,
			dimension TEXT,
			severity TEXT,
			file TEXT,
			line INTEGER,
			rule TEXT,
			message TEXT,
			fingerprint TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_findings_fp ON findings(fingerprint)`,
		`CREATE INDEX IF NOT EXISTS idx_findings_run ON findings(run_id)`,

		`CREATE TABLE IF NOT EXISTS decisions (
			id INTEGER PRIMARY KEY,
			timestamp TEXT,
			sprint_number INTEGER,
			category TEXT,
			summary TEXT,
			rationale TEXT
		)`,

		`CREATE TABLE IF NOT EXISTS contracts (
			sprint_number INTEGER PRIMARY KEY,
			created_at TEXT,
			goal TEXT,
			status TEXT,
			path TEXT
		)`,
	}
	for _, s := range stmts {
		if _, err := db.conn.Exec(s); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	return nil
}

// Run represents a single harness execution recorded in memory.
type Run struct {
	ID           int64
	Timestamp    time.Time
	GitSHA       string
	Branch       string
	SprintNumber int
	Trigger      string
	Verdict      string
	ScoreTotal   int
	Scores       map[string]int
}

// Finding is a single issue identified by a sensor.
type Finding struct {
	ID          string
	RunID       int64
	Dimension   string
	Severity    string
	File        string
	Line        int
	Rule        string
	Message     string
	Fingerprint string
	// Recurrence is filled in by FindingByID and similar — how many
	// previous runs contained a finding with the same fingerprint.
	Recurrence int
	FirstSeen  time.Time
}

// InsertRun records a new run and returns its id.
func (db *DB) InsertRun(r Run) (int64, error) {
	scoresJSON, _ := json.Marshal(r.Scores)
	res, err := db.conn.Exec(`
		INSERT INTO runs (timestamp, git_sha, branch, sprint_number, trigger, verdict, score_total, scores_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		r.Timestamp.UTC().Format(time.RFC3339), r.GitSHA, r.Branch,
		r.SprintNumber, r.Trigger, r.Verdict, r.ScoreTotal, string(scoresJSON))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// InsertFinding records a finding under a run.
func (db *DB) InsertFinding(f Finding) error {
	_, err := db.conn.Exec(`
		INSERT INTO findings (id, run_id, dimension, severity, file, line, rule, message, fingerprint)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		f.ID, f.RunID, f.Dimension, f.Severity, f.File, f.Line, f.Rule, f.Message, f.Fingerprint)
	return err
}

// RecentRuns returns the most recent N runs in descending timestamp order.
func (db *DB) RecentRuns(n int) ([]Run, error) {
	rows, err := db.conn.Query(`
		SELECT id, timestamp, COALESCE(git_sha, ''), COALESCE(branch, ''),
		       COALESCE(sprint_number, 0), COALESCE(trigger, ''),
		       COALESCE(verdict, ''), COALESCE(score_total, 0),
		       COALESCE(scores_json, '{}')
		FROM runs ORDER BY timestamp DESC LIMIT ?`, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Run
	for rows.Next() {
		var r Run
		var ts, scoresJSON string
		if err := rows.Scan(&r.ID, &ts, &r.GitSHA, &r.Branch, &r.SprintNumber,
			&r.Trigger, &r.Verdict, &r.ScoreTotal, &scoresJSON); err != nil {
			return nil, err
		}
		r.Timestamp, _ = time.Parse(time.RFC3339, ts)
		_ = json.Unmarshal([]byte(scoresJSON), &r.Scores)
		out = append(out, r)
	}
	return out, rows.Err()
}

// FingerprintRecurrence reports how many distinct runs contain a finding
// with the given fingerprint, and when it was first seen. This is what
// powers "this finding appeared in 4 PRs" recurrence detection.
func (db *DB) FingerprintRecurrence(fp string) (count int, firstSeen time.Time, err error) {
	row := db.conn.QueryRow(`
		SELECT COUNT(DISTINCT run_id), MIN(r.timestamp)
		FROM findings f JOIN runs r ON r.id = f.run_id
		WHERE f.fingerprint = ?`, fp)
	var ts sql.NullString
	if err := row.Scan(&count, &ts); err != nil {
		return 0, time.Time{}, err
	}
	if ts.Valid {
		firstSeen, _ = time.Parse(time.RFC3339, ts.String)
	}
	return count, firstSeen, nil
}

// FindingByID fetches a single finding with recurrence metadata attached.
func (db *DB) FindingByID(id string) (Finding, error) {
	var f Finding
	row := db.conn.QueryRow(`
		SELECT id, run_id, dimension, severity, file, line, rule, message, fingerprint
		FROM findings WHERE id = ?`, id)
	if err := row.Scan(&f.ID, &f.RunID, &f.Dimension, &f.Severity,
		&f.File, &f.Line, &f.Rule, &f.Message, &f.Fingerprint); err != nil {
		return Finding{}, err
	}
	f.Recurrence, f.FirstSeen, _ = db.FingerprintRecurrence(f.Fingerprint)
	return f, nil
}

// RecordDecision logs an architectural or process decision. Surfaced in
// progress.md and queryable later — answers "why did we choose X?"
func (db *DB) RecordDecision(sprintNum int, category, summary, rationale string) error {
	_, err := db.conn.Exec(`
		INSERT INTO decisions (timestamp, sprint_number, category, summary, rationale)
		VALUES (?, ?, ?, ?, ?)`,
		time.Now().UTC().Format(time.RFC3339), sprintNum, category, summary, rationale)
	return err
}

// UpsertContract records sprint contract metadata.
func (db *DB) UpsertContract(sprintNum int, goal, status, path string) error {
	_, err := db.conn.Exec(`
		INSERT INTO contracts (sprint_number, created_at, goal, status, path)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(sprint_number) DO UPDATE SET status = excluded.status`,
		sprintNum, time.Now().UTC().Format(time.RFC3339), goal, status, path)
	return err
}
