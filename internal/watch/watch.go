// Package watch implements `harness watch`, the periodic drift monitor.
//
// Watch is distinct from sprint QA:
//
//   - it runs OUTSIDE the sprint lifecycle (no contract required);
//   - it never gates anything (purely observational);
//   - it does not write to .harness/reports/ or memory.db (those slots
//     belong to full QA);
//   - it stores its reports under .harness/watch/<timestamp>.json and a
//     stable .harness/watch/latest.json pointer;
//   - it compares the current run against the previous one and surfaces
//     deltas so CI cron jobs can fail on regressions.
//
// The sensor set is fast static analysis plus configured audit adapters
// (npm-audit, pip-audit, govulncheck, cargo-audit). Tests and e2e are
// excluded because they are too slow for periodic monitoring.
package watch

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/dancampari/harness/internal/adapters"
	"github.com/dancampari/harness/internal/config"
	"github.com/dancampari/harness/internal/evaluator"
)

const schemaVersion = "1"

// Report captures one drift-watch run.
type Report struct {
	SchemaVersion string                  `json:"schema_version"`
	Timestamp     time.Time               `json:"timestamp"`
	Findings      int                     `json:"findings"`
	Dimensions    map[string]DimSummary   `json:"dimensions"`
	Sensors       []evaluator.SensorStatus `json:"sensors"`
	Delta         *Delta                  `json:"delta,omitempty"`
}

// DimSummary is a compact per-dimension projection of the underlying
// EvaluationResult, kept narrow so watch reports do not balloon.
type DimSummary struct {
	Score    int  `json:"score"`
	Findings int  `json:"findings"`
	Passed   bool `json:"passed"`
	Skipped  bool `json:"skipped,omitempty"`
}

// Delta compares the current Report to the previous one (latest.json).
// A non-zero Regressed count means the new run found dimensions or
// findings that were not present last time — exactly what cron is
// looking for.
type Delta struct {
	Compared          string         `json:"compared"` // path of previous report
	FindingsBefore    int            `json:"findings_before"`
	FindingsAfter     int            `json:"findings_after"`
	Regressed         int            `json:"regressed"` // positive when after > before
	DimensionDeltas   map[string]int `json:"dimension_deltas"`
}

// Result wraps the report with file paths so the CLI can show them.
type Result struct {
	Report       Report
	ReportPath   string
	LatestPath   string
	IsRegression bool
}

// RunOnce executes one watch pass against repoRoot, using the harness
// config at harnessDir/config.yaml. The caller is responsible for
// chosing when to run (cron, GitHub Actions, etc.); watch never spawns
// background workers itself.
func RunOnce(ctx context.Context, repoRoot, harnessDir string) (*Result, error) {
	cfg, err := config.Load(filepath.Join(harnessDir, "config.yaml"))
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	if errs := cfg.Validate(); len(errs) > 0 {
		return nil, fmt.Errorf("invalid config")
	}

	ev := evaluator.New(cfg, adapters.BuildRegistry())
	check := evaluator.ContractCheckResult{Status: "skipped"}
	evalCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	res, err := ev.EvaluateWith(evalCtx, repoRoot, 0, check, evaluator.Options{
		Fast:          true,
		IncludeAudits: true,
		SkipContract:  true,
	})
	if err != nil {
		return nil, err
	}

	report := Report{
		SchemaVersion: schemaVersion,
		Timestamp:     time.Now().UTC(),
		Dimensions:    make(map[string]DimSummary),
		Sensors:       res.Sensors,
	}
	for name, d := range res.Dimensions {
		report.Dimensions[name] = DimSummary{
			Score:    d.Score,
			Findings: len(d.Findings),
			Passed:   d.Passed,
			Skipped:  d.Skipped,
		}
		if !d.Skipped {
			report.Findings += len(d.Findings)
		}
	}

	delta := computeDelta(harnessDir, report)
	report.Delta = delta

	watchDir := filepath.Join(harnessDir, "watch")
	if err := os.MkdirAll(watchDir, 0o755); err != nil {
		return nil, err
	}
	reportPath := filepath.Join(watchDir, report.Timestamp.Format("20060102T150405Z")+".json")
	body, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, err
	}
	body = append(body, '\n')
	if err := os.WriteFile(reportPath, body, 0o644); err != nil {
		return nil, err
	}
	latestPath := filepath.Join(watchDir, "latest.json")
	if err := os.WriteFile(latestPath, body, 0o644); err != nil {
		return nil, err
	}

	return &Result{
		Report:       report,
		ReportPath:   reportPath,
		LatestPath:   latestPath,
		IsRegression: delta != nil && delta.Regressed > 0,
	}, nil
}

// computeDelta loads the most recent prior watch report (if any) and
// returns the diff between it and current. Watch keeps only the
// previous-latest pointer; per-run timestamped files are append-only
// archives the user/CI can browse separately.
func computeDelta(harnessDir string, current Report) *Delta {
	previousPath := filepath.Join(harnessDir, "watch", "latest.json")
	b, err := os.ReadFile(previousPath)
	if err != nil {
		return nil
	}
	var prev Report
	if err := json.Unmarshal(b, &prev); err != nil {
		return nil
	}
	delta := &Delta{
		Compared:        previousPath,
		FindingsBefore:  prev.Findings,
		FindingsAfter:   current.Findings,
		DimensionDeltas: map[string]int{},
	}
	if delta.FindingsAfter > delta.FindingsBefore {
		delta.Regressed = delta.FindingsAfter - delta.FindingsBefore
	}
	// Per-dimension delta: positive means more findings now than before.
	names := map[string]bool{}
	for n := range prev.Dimensions {
		names[n] = true
	}
	for n := range current.Dimensions {
		names[n] = true
	}
	for n := range names {
		before := prev.Dimensions[n].Findings
		after := current.Dimensions[n].Findings
		if before == after {
			continue
		}
		delta.DimensionDeltas[n] = after - before
	}
	return delta
}

// List returns past watch reports sorted by timestamp descending. Used
// by the TUI and `harness watch list`.
func List(harnessDir string, limit int) ([]string, error) {
	dir := filepath.Join(harnessDir, "watch")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if name == "latest.json" {
			continue
		}
		out = append(out, filepath.Join(dir, name))
	}
	sort.Sort(sort.Reverse(sort.StringSlice(out)))
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}
