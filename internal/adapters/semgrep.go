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

type Semgrep struct{}

func (Semgrep) Name() string                 { return "semgrep" }
func (Semgrep) Dimension() sensors.Dimension { return sensors.DimSecurity }

func (Semgrep) Available(root string) bool {
	return hasProjectCommand(root, "semgrep") && semgrepConfig(root) != ""
}

type semgrepReport struct {
	Results []struct {
		CheckID string `json:"check_id"`
		Path    string `json:"path"`
		Start   struct {
			Line int `json:"line"`
		} `json:"start"`
		Extra struct {
			Message  string `json:"message"`
			Severity string `json:"severity"`
		} `json:"extra"`
	} `json:"results"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func (s Semgrep) Run(ctx context.Context, root string) sensors.Result {
	start := time.Now()
	res := sensors.Result{SensorName: s.Name(), Dimension: s.Dimension()}
	configPath := semgrepConfig(root)
	if configPath == "" {
		res.Duration = time.Since(start)
		res.RawScore = 0
		res.Findings = append(res.Findings, finding(
			sensors.DimSecurity,
			sensors.SeverityHigh,
			"",
			0,
			"semgrep-config-missing",
			"semgrep requires .semgrep.yml, .semgrep.yaml, or .semgrep/",
		))
		return res
	}
	cmd := exec.CommandContext(ctx, commandOrName(root, "semgrep"), "--json", "--config", configPath, ".")
	cmd.Dir = root
	out, err := cmd.Output()
	res.Duration = time.Since(start)
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && len(out) == 0 {
			out = exitErr.Stderr
		} else if !ok {
			res.Error = err.Error()
			return res
		}
	}
	var report semgrepReport
	if err := json.Unmarshal(out, &report); err != nil {
		res.Error = fmt.Sprintf("parse semgrep output: %v", err)
		return res
	}
	if len(report.Errors) > 0 {
		for _, e := range report.Errors {
			res.Findings = append(res.Findings, finding(
				sensors.DimSecurity,
				sensors.SeverityMedium,
				configPath,
				0,
				"semgrep-error",
				e.Message,
			))
		}
	}
	for _, result := range report.Results {
		res.Findings = append(res.Findings, finding(
			sensors.DimSecurity,
			semgrepSeverity(result.Extra.Severity),
			filepath.ToSlash(result.Path),
			result.Start.Line,
			nonEmpty(result.CheckID, "semgrep"),
			result.Extra.Message,
		))
	}
	res.RawScore = clampScore(100 - len(report.Results)*12 - len(report.Errors)*5)
	return res
}

func semgrepConfig(root string) string {
	for _, name := range []string{".semgrep.yml", ".semgrep.yaml", ".semgrep"} {
		if detect.HasFile(root, name) {
			return name
		}
	}
	return ""
}

func semgrepSeverity(value string) sensors.Severity {
	switch value {
	case "ERROR":
		return sensors.SeverityHigh
	case "WARNING":
		return sensors.SeverityMedium
	default:
		return sensors.SeverityLow
	}
}
