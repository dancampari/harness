package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/dancampari/harness/internal/detect"
	"github.com/dancampari/harness/internal/sensors"
)

type JestCoverage struct{}

func (JestCoverage) Name() string                 { return "jest-coverage" }
func (JestCoverage) Dimension() sensors.Dimension { return sensors.DimCoverage }

func (JestCoverage) Available(root string) bool {
	return detect.HasFile(root, "package.json") && hasNodeBin(root, "jest")
}

type jestCoverageSummary struct {
	Total struct {
		Lines      coverageMetric `json:"lines"`
		Statements coverageMetric `json:"statements"`
		Functions  coverageMetric `json:"functions"`
		Branches   coverageMetric `json:"branches"`
	} `json:"total"`
}

type coverageMetric struct {
	Pct float64 `json:"pct"`
}

func (j JestCoverage) Run(ctx context.Context, root string) sensors.Result {
	start := time.Now()
	res := sensors.Result{
		SensorName: j.Name(),
		Dimension:  j.Dimension(),
	}
	cmd := exec.CommandContext(ctx, "npx", "--no-install", "jest",
		"--coverage",
		"--coverageReporters=json-summary",
		"--coverageReporters=lcov",
		"--passWithNoTests")
	cmd.Dir = root
	_, err := cmd.Output()
	res.Duration = time.Since(start)
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			res.Error = err.Error()
			return res
		}
	}

	path := filepath.Join(root, "coverage", "coverage-summary.json")
	b, err := os.ReadFile(path)
	if err != nil {
		res.RawScore = 0
		res.Findings = append(res.Findings, finding(
			sensors.DimCoverage,
			sensors.SeverityHigh,
			"",
			0,
			"coverage-missing",
			"Jest did not produce coverage/coverage-summary.json",
		))
		return res
	}
	var summary jestCoverageSummary
	if err := json.Unmarshal(b, &summary); err != nil {
		res.Error = fmt.Sprintf("parse coverage summary: %v", err)
		return res
	}
	score := int(summary.Total.Lines.Pct)
	res.RawScore = clampScore(score)
	if res.RawScore < 70 {
		res.Findings = append(res.Findings, finding(
			sensors.DimCoverage,
			sensors.SeverityHigh,
			"coverage/coverage-summary.json",
			0,
			"coverage-below-bar",
			fmt.Sprintf("line coverage is %.1f%%", summary.Total.Lines.Pct),
		))
	}
	return res
}
