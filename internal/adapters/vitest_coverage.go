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

type VitestCoverage struct{}

func (VitestCoverage) Name() string                 { return "vitest-coverage" }
func (VitestCoverage) Dimension() sensors.Dimension { return sensors.DimCoverage }

func (VitestCoverage) Available(root string) bool {
	return detect.HasFile(root, "package.json") && hasNodeBin(root, "vitest")
}

func (v VitestCoverage) Run(ctx context.Context, root string) sensors.Result {
	start := time.Now()
	res := sensors.Result{
		SensorName: v.Name(),
		Dimension:  v.Dimension(),
	}
	cmd := nodeToolCommand(ctx, root, "vitest", "run",
		"--coverage",
		"--coverage.reporter=json-summary",
		"--coverage.reporter=lcov")
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
			"Vitest did not produce coverage/coverage-summary.json",
		))
		return res
	}
	var summary jestCoverageSummary
	if err := json.Unmarshal(b, &summary); err != nil {
		res.Error = fmt.Sprintf("parse coverage summary: %v", err)
		return res
	}
	res.RawScore = clampScore(int(summary.Total.Lines.Pct))
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
