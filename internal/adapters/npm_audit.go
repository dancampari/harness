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

type NpmAudit struct{}

func (NpmAudit) Name() string                 { return "npm-audit" }
func (NpmAudit) Dimension() sensors.Dimension { return sensors.DimSecurity }

func (NpmAudit) Available(root string) bool {
	if !detect.HasFile(root, "package.json") {
		return false
	}
	_, err := exec.LookPath("npm")
	return err == nil
}

type npmAuditReport struct {
	Vulnerabilities map[string]struct {
		Name     string `json:"name"`
		Severity string `json:"severity"`
		Title    string `json:"title"`
		Via      any    `json:"via"`
	} `json:"vulnerabilities"`
	Metadata struct {
		Vulnerabilities map[string]int `json:"vulnerabilities"`
	} `json:"metadata"`
	Error *struct {
		Code    string `json:"code"`
		Summary string `json:"summary"`
		Detail  string `json:"detail"`
	} `json:"error,omitempty"`
}

func (n NpmAudit) Run(ctx context.Context, root string) sensors.Result {
	start := time.Now()
	res := sensors.Result{
		SensorName: n.Name(),
		Dimension:  n.Dimension(),
	}
	if !detect.HasFile(root, "package-lock.json") &&
		!detect.HasFile(root, "npm-shrinkwrap.json") {
		res.Duration = time.Since(start)
		res.RawScore = 0
		res.Findings = append(res.Findings, finding(
			sensors.DimSecurity,
			sensors.SeverityHigh,
			"package.json",
			0,
			"npm-audit-lockfile-missing",
			"npm audit requires package-lock.json or npm-shrinkwrap.json",
		))
		return res
	}

	cmd := exec.CommandContext(ctx, "npm", "audit", "--json")
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

	var report npmAuditReport
	if err := json.Unmarshal(out, &report); err != nil {
		res.Error = fmt.Sprintf("parse npm audit output: %v", err)
		return res
	}
	if report.Error != nil {
		res.RawScore = 0
		res.Findings = append(res.Findings, finding(
			sensors.DimSecurity,
			sensors.SeverityHigh,
			"package-lock.json",
			0,
			"npm-audit-error",
			report.Error.Summary,
		))
		return res
	}

	counts := report.Metadata.Vulnerabilities
	deduction := counts["critical"]*25 + counts["high"]*15 + counts["moderate"]*7 + counts["low"]*2
	res.RawScore = clampScore(100 - deduction)
	for name, vuln := range report.Vulnerabilities {
		sev := auditSeverity(vuln.Severity)
		msg := vuln.Title
		if msg == "" {
			msg = fmt.Sprintf("%s vulnerability in %s", vuln.Severity, name)
		}
		res.Findings = append(res.Findings, finding(
			sensors.DimSecurity,
			sev,
			"package-lock.json",
			0,
			"npm-audit-"+vuln.Severity,
			msg,
		))
	}
	return res
}

func auditSeverity(sev string) sensors.Severity {
	switch sev {
	case "critical":
		return sensors.SeverityCritical
	case "high":
		return sensors.SeverityHigh
	case "moderate":
		return sensors.SeverityMedium
	case "low":
		return sensors.SeverityLow
	default:
		return sensors.SeverityInfo
	}
}
