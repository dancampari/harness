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

// Jest is a combined test+coverage sensor. We split scoring: failures
// affect Correctness, coverage % affects Coverage. The Evaluator routes
// each finding to its dimension based on its declared Dimension field.
type Jest struct{}

func (Jest) Name() string                 { return "jest" }
func (Jest) Dimension() sensors.Dimension { return sensors.DimCorrectness }

func (Jest) Available(root string) bool {
	if !detect.HasFile(root, "package.json") {
		return false
	}
	// Check jest is in package.json. We don't parse it — just look for jest
	// in node_modules/.bin or fall back to npx.
	return hasNodeBin(root, "jest")
}

// jestSummary is the shape we extract from --json output. Jest emits a
// rich structure; we keep only what we need.
type jestSummary struct {
	NumFailedTests int  `json:"numFailedTests"`
	NumPassedTests int  `json:"numPassedTests"`
	NumTotalTests  int  `json:"numTotalTests"`
	Success        bool `json:"success"`
	CoverageMap    json.RawMessage
	TestResults    []struct {
		Name             string `json:"name"`
		Status           string `json:"status"` // "passed" | "failed"
		FailureMessage   string `json:"failureMessage"`
		AssertionResults []struct {
			FullName        string   `json:"fullName"`
			Status          string   `json:"status"`
			FailureMessages []string `json:"failureMessages"`
			Location        *struct {
				Line int `json:"line"`
			} `json:"location"`
		} `json:"assertionResults"`
	} `json:"testResults"`
}

func (j Jest) Run(ctx context.Context, root string) sensors.Result {
	start := time.Now()
	res := sensors.Result{
		SensorName: j.Name(),
		Dimension:  j.Dimension(),
	}

	// We run with --json so we can parse, --coverage so we can extract
	// percentages, and --passWithNoTests to avoid spurious failures on
	// projects that haven't authored tests yet (the harness will catch
	// that via the Coverage dimension instead).
	cmd := exec.CommandContext(ctx, "npx", "--no-install", "jest",
		"--json",
		"--passWithNoTests")
	cmd.Dir = root
	out, err := cmd.Output()
	res.Duration = time.Since(start)
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			res.Error = err.Error()
			return res
		}
		// ExitError is expected on failing tests.
	}

	var summary jestSummary
	if err := json.Unmarshal(out, &summary); err != nil {
		res.Error = fmt.Sprintf("parse jest output: %v", err)
		return res
	}

	for _, tr := range summary.TestResults {
		for _, ar := range tr.AssertionResults {
			if ar.Status != "failed" {
				continue
			}
			msg := ar.FullName
			if len(ar.FailureMessages) > 0 {
				msg = ar.FailureMessages[0]
			}
			rel, _ := filepath.Rel(root, tr.Name)
			line := 0
			if ar.Location != nil {
				line = ar.Location.Line
			}
			f := sensors.Finding{
				Dimension: sensors.DimCorrectness,
				Severity:  sensors.SeverityCritical,
				File:      rel,
				Line:      line,
				Rule:      "test-failure",
				Message:   truncateMessage(msg, 240),
			}
			f.Fingerprint = sensors.Fingerprint(f.Dimension, f.File, f.Rule, ar.FullName)
			res.Findings = append(res.Findings, f)
		}
	}

	// Score: passing ratio. A single failure should hit hard — passing
	// "most" tests is not the bar.
	if summary.NumTotalTests == 0 {
		res.RawScore = 0
		res.Findings = append(res.Findings, sensors.Finding{
			Dimension: sensors.DimCorrectness,
			Severity:  sensors.SeverityHigh,
			Rule:      "no-tests-found",
			Message:   "no jest tests discovered in this project",
		})
	} else {
		ratio := float64(summary.NumPassedTests) / float64(safeTotal(summary.NumTotalTests))
		res.RawScore = int(ratio * 100)
		if summary.NumFailedTests > 0 && res.RawScore > 50 {
			// Hard cap when any test fails — passing "most" is not enough.
			res.RawScore = 50
		}
	}
	return res
}

// safeTotal avoids div-by-zero.
func safeTotal(n int) int {
	if n == 0 {
		return 1
	}
	return n
}

func truncateMessage(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
