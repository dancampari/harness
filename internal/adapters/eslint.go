// Package adapters contains concrete sensor implementations for each
// supported tool. Adapters parse tool output into the sensors.Result format.
//
// Adding a new tool means creating a new file here and registering it
// in registry.go. Adapters MUST be deterministic and never call out to
// LLMs — that is a hard rule of the harness design.
package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/dancampari/harness/internal/detect"
	"github.com/dancampari/harness/internal/sensors"
)

// ESLint is the lint sensor for Node/TS projects.
type ESLint struct{}

// Name implements sensors.Sensor.
func (ESLint) Name() string { return "eslint" }

// Dimension implements sensors.Sensor.
func (ESLint) Dimension() sensors.Dimension { return sensors.DimCorrectness }

// Available reports true when both eslint binary and a config exist.
func (ESLint) Available(root string) bool {
	if !detect.HasFile(root, "package.json") {
		return false
	}
	// Look for any ESLint config form.
	configs := []string{".eslintrc", ".eslintrc.js", ".eslintrc.json",
		".eslintrc.yaml", ".eslintrc.yml", "eslint.config.js", "eslint.config.mjs"}
	hasConfig := false
	for _, c := range configs {
		if detect.HasFile(root, c) {
			hasConfig = true
			break
		}
	}
	if !hasConfig {
		return false
	}
	return hasNodeBin(root, "eslint")
}

// eslintMessage is one item from ESLint's JSON formatter output.
type eslintMessage struct {
	RuleID   string `json:"ruleId"`
	Severity int    `json:"severity"` // 1=warn, 2=error
	Message  string `json:"message"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
}

type eslintFile struct {
	FilePath     string          `json:"filePath"`
	Messages     []eslintMessage `json:"messages"`
	ErrorCount   int             `json:"errorCount"`
	WarningCount int             `json:"warningCount"`
}

// Run executes eslint and parses its JSON output.
func (e ESLint) Run(ctx context.Context, root string) sensors.Result {
	start := time.Now()
	res := sensors.Result{
		SensorName: e.Name(),
		Dimension:  e.Dimension(),
	}

	// Use --format=json and --no-color. We scope to the repo root and let
	// eslint's config decide which files to include.
	// --no-error-on-unmatched-pattern avoids spurious failures when the
	// project hasn't set up file patterns.
	cmd := nodeToolCommand(ctx, root, "eslint",
		"--format", "json",
		"--no-error-on-unmatched-pattern",
		".")
	cmd.Dir = root
	out, err := cmd.Output()
	res.Duration = time.Since(start)
	// ESLint exits non-zero when it finds errors — that's expected.
	// We only treat parse failures as errors.
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			res.Error = err.Error()
			return res
		}
	}

	var files []eslintFile
	if err := json.Unmarshal(out, &files); err != nil {
		res.Error = fmt.Sprintf("parse eslint output: %v", err)
		return res
	}

	var errors, warnings int
	for _, f := range files {
		errors += f.ErrorCount
		warnings += f.WarningCount
		for _, m := range f.Messages {
			sev := sensors.SeverityMedium
			if m.Severity == 2 {
				sev = sensors.SeverityHigh
			} else if m.Severity == 1 {
				sev = sensors.SeverityLow
			}
			rel, _ := filepath.Rel(root, f.FilePath)
			finding := sensors.Finding{
				Dimension: sensors.DimCorrectness,
				Severity:  sev,
				File:      rel,
				Line:      m.Line,
				Rule:      m.RuleID,
				Message:   m.Message,
			}
			finding.Fingerprint = sensors.Fingerprint(
				finding.Dimension, finding.File, finding.Rule, finding.Message)
			res.Findings = append(res.Findings, finding)
		}
	}

	// Score: 100 minus weighted deductions. Errors hurt more than warnings.
	// Capped at 0. This formula is intentionally conservative — the
	// Evaluator can re-weight at aggregation time.
	deduction := errors*4 + warnings*1
	res.RawScore = 100 - deduction
	if res.RawScore < 0 {
		res.RawScore = 0
	}
	return res
}
