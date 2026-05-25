package adapters

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dancampari/harness/internal/sensors"
)

// TestCountTracker counts every test file in the project tree and
// compares the result against the previous run's count, persisted at
// `.harness/test-count.json`. TLC implement.md treats a drop in the
// test count without an explanation as a regression worth surfacing
// — agents sometimes delete passing tests to make red builds green,
// and the harness needs to make that visible.
//
// The sensor is deterministic and offline: it walks the project,
// filters by test-file heuristics already used by the TDD-violation
// sensor, and records the count. The first run never reports a
// regression; subsequent runs compare against the persisted snapshot.
type TestCountTracker struct{}

func (TestCountTracker) Name() string                 { return "test-count-tracker" }
func (TestCountTracker) Dimension() sensors.Dimension { return sensors.DimContract }
func (TestCountTracker) Available(root string) bool   { return true }

type testCountSnapshot struct {
	SchemaVersion string `json:"schema_version"`
	Count         int    `json:"count"`
	UpdatedAt     string `json:"updated_at"`
}

const testCountSchema = "1"

func (t TestCountTracker) Run(ctx context.Context, root string) sensors.Result {
	start := time.Now()
	res := sensors.Result{
		SensorName: t.Name(),
		Dimension:  t.Dimension(),
		RawScore:   100,
	}

	current := countTestFiles(ctx, root)
	previous, hadPrev := loadTestCountSnapshot(root)

	if hadPrev && current < previous {
		dropped := previous - current
		res.Findings = append(res.Findings, finding(
			sensors.DimContract,
			sensors.SeverityMedium,
			"",
			0,
			"test-count-regression",
			"test file count dropped from "+itoa(previous)+" to "+itoa(current)+
				" ("+itoa(dropped)+" removed) — record a `harness state record decision \"<why>\"` if intentional, otherwise restore the missing tests (TLC implement.md treats undeclared test drops as regressions)",
		))
		// Dropping tests is a contract regression until the spec/tasks are
		// updated to justify it. Do not let this pass via score averaging.
		res.RawScore = 0
	}

	// Persist the snapshot AFTER comparing so a single run never
	// erases its own baseline.
	_ = saveTestCountSnapshot(root, current)
	res.Duration = time.Since(start)
	return res
}

func loadTestCountSnapshot(root string) (int, bool) {
	path := filepath.Join(root, ".harness", "test-count.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	var snap testCountSnapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return 0, false
	}
	return snap.Count, true
}

func saveTestCountSnapshot(root string, count int) error {
	dir := filepath.Join(root, ".harness")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	snap := testCountSnapshot{
		SchemaVersion: testCountSchema,
		Count:         count,
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
	}
	b, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(filepath.Join(dir, "test-count.json"), b, 0o644)
}

// countTestFiles walks the project and returns the number of test
// files, using the same heuristics as the TDD sensor's classifier.
// Build-output and dependency directories are skipped to keep the
// count meaningful.
func countTestFiles(ctx context.Context, root string) int {
	count := 0
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() {
			if shouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)
		if !isSourceFile(rel) {
			return nil
		}
		if !isTestFile(rel) {
			return nil
		}
		// Strip paths under .harness/skills/ TLC reference docs that
		// happen to mention "test" but are not actual tests.
		if strings.Contains(rel, ".harness/skills/") {
			return nil
		}
		count++
		return nil
	})
	return count
}
