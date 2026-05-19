package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/dancampari/harness/internal/detect"
	"github.com/dancampari/harness/internal/sensors"
)

type Vitest struct{}

func (Vitest) Name() string                 { return "vitest" }
func (Vitest) Dimension() sensors.Dimension { return sensors.DimCorrectness }

func (Vitest) Available(root string) bool {
	return detect.HasFile(root, "package.json") && hasNodeBin(root, "vitest")
}

type vitestReport struct {
	NumFailedTests int `json:"numFailedTests"`
	NumPassedTests int `json:"numPassedTests"`
	NumTotalTests  int `json:"numTotalTests"`
	TestResults    []struct {
		Name             string `json:"name"`
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

func (v Vitest) Run(ctx context.Context, root string) sensors.Result {
	start := time.Now()
	res := sensors.Result{
		SensorName: v.Name(),
		Dimension:  v.Dimension(),
	}
	cmd := exec.CommandContext(ctx, "npx", "--no-install", "vitest", "run", "--reporter=json")
	cmd.Dir = root
	out, err := cmd.Output()
	res.Duration = time.Since(start)
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			res.Error = err.Error()
			return res
		}
	}
	var report vitestReport
	if err := json.Unmarshal(out, &report); err != nil {
		res.Error = fmt.Sprintf("parse vitest output: %v", err)
		return res
	}
	for _, tr := range report.TestResults {
		for _, ar := range tr.AssertionResults {
			if ar.Status != "failed" {
				continue
			}
			msg := ar.FullName
			if len(ar.FailureMessages) > 0 {
				msg = ar.FailureMessages[0]
			}
			line := 0
			if ar.Location != nil {
				line = ar.Location.Line
			}
			res.Findings = append(res.Findings, finding(
				sensors.DimCorrectness,
				sensors.SeverityCritical,
				slashRel(root, tr.Name),
				line,
				"test-failure",
				truncateMessage(msg, 240),
			))
		}
	}
	if report.NumTotalTests == 0 {
		res.RawScore = 0
		res.Findings = append(res.Findings, finding(
			sensors.DimCorrectness,
			sensors.SeverityHigh,
			"",
			0,
			"no-tests-found",
			"no Vitest tests discovered in this project",
		))
		return res
	}
	res.RawScore = int(float64(report.NumPassedTests) / float64(safeTotal(report.NumTotalTests)) * 100)
	if report.NumFailedTests > 0 && res.RawScore > 50 {
		res.RawScore = 50
	}
	return res
}
