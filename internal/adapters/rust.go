package adapters

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/dancampari/harness/internal/detect"
	"github.com/dancampari/harness/internal/sensors"
)

type CargoClippy struct{}
type CargoTest struct{}
type CargoAudit struct{}

func (CargoClippy) Name() string                 { return "clippy" }
func (CargoClippy) Dimension() sensors.Dimension { return sensors.DimCorrectness }
func (CargoClippy) Available(root string) bool {
	return detect.HasFile(root, "Cargo.toml") && hasProjectCommand(root, "cargo")
}

func (CargoTest) Name() string                 { return "cargo-test" }
func (CargoTest) Dimension() sensors.Dimension { return sensors.DimCorrectness }
func (CargoTest) Available(root string) bool {
	return detect.HasFile(root, "Cargo.toml") && hasProjectCommand(root, "cargo")
}

func (CargoAudit) Name() string                 { return "cargo-audit" }
func (CargoAudit) Dimension() sensors.Dimension { return sensors.DimSecurity }
func (CargoAudit) Available(root string) bool {
	return detect.HasFile(root, "Cargo.toml") && hasProjectCommand(root, "cargo-audit")
}

type cargoMessage struct {
	Reason  string `json:"reason"`
	Message struct {
		Message string `json:"message"`
		Code    *struct {
			Code string `json:"code"`
		} `json:"code"`
		Level string `json:"level"`
		Spans []struct {
			FileName    string `json:"file_name"`
			LineStart   int    `json:"line_start"`
			IsPrimary   bool   `json:"is_primary"`
			Label       string `json:"label"`
			Suggested   string `json:"suggested_replacement"`
			ColumnStart int    `json:"column_start"`
		} `json:"spans"`
	} `json:"message"`
}

func parseCargoMessages(output string) []sensors.Finding {
	var findings []sensors.Finding
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var msg cargoMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil || msg.Reason != "compiler-message" {
			continue
		}
		if msg.Message.Level != "error" && msg.Message.Level != "warning" {
			continue
		}
		sev := sensors.SeverityMedium
		if msg.Message.Level == "error" {
			sev = sensors.SeverityHigh
		}
		file := ""
		lineNo := 0
		for _, span := range msg.Message.Spans {
			if span.IsPrimary {
				file = span.FileName
				lineNo = span.LineStart
				break
			}
		}
		rule := "clippy"
		if msg.Message.Code != nil && msg.Message.Code.Code != "" {
			rule = msg.Message.Code.Code
		}
		findings = append(findings, finding(
			sensors.DimCorrectness,
			sev,
			file,
			lineNo,
			rule,
			msg.Message.Message,
		))
	}
	return findings
}

func (c CargoClippy) Run(ctx context.Context, root string) sensors.Result {
	start := time.Now()
	res := sensors.Result{SensorName: c.Name(), Dimension: c.Dimension()}
	cmd := exec.CommandContext(ctx, commandOrName(root, "cargo"), "clippy", "--all-targets", "--message-format=json", "--", "-D", "warnings")
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
	res.Findings = parseCargoMessages(string(out))
	res.RawScore = clampScore(100 - len(res.Findings)*7)
	return res
}

func (c CargoTest) Run(ctx context.Context, root string) sensors.Result {
	start := time.Now()
	res := sensors.Result{SensorName: c.Name(), Dimension: c.Dimension()}
	cmd := exec.CommandContext(ctx, commandOrName(root, "cargo"), "test", "--all", "--quiet")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	res.Duration = time.Since(start)
	if err == nil {
		res.RawScore = 100
		return res
	}
	if _, ok := err.(*exec.ExitError); !ok {
		res.Error = err.Error()
		return res
	}
	res.RawScore = 50
	res.Findings = append(res.Findings, finding(
		sensors.DimCorrectness,
		sensors.SeverityCritical,
		"",
		0,
		"cargo-test-failure",
		truncateMessage(string(out), 240),
	))
	return res
}

type cargoAuditReport struct {
	Vulnerabilities struct {
		Count int `json:"count"`
		List  []struct {
			Advisory struct {
				ID          string `json:"id"`
				Title       string `json:"title"`
				Description string `json:"description"`
			} `json:"advisory"`
			Package struct {
				Name string `json:"name"`
			} `json:"package"`
		} `json:"list"`
	} `json:"vulnerabilities"`
}

func (c CargoAudit) Run(ctx context.Context, root string) sensors.Result {
	start := time.Now()
	res := sensors.Result{SensorName: c.Name(), Dimension: c.Dimension()}
	cmd := exec.CommandContext(ctx, commandOrName(root, "cargo-audit"), "--json")
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
	var report cargoAuditReport
	if err := json.Unmarshal(out, &report); err != nil {
		res.Error = fmt.Sprintf("parse cargo audit output: %v", err)
		return res
	}
	for _, vuln := range report.Vulnerabilities.List {
		msg := vuln.Advisory.Title
		if msg == "" {
			msg = vuln.Advisory.Description
		}
		res.Findings = append(res.Findings, finding(
			sensors.DimSecurity,
			sensors.SeverityHigh,
			"Cargo.lock",
			0,
			nonEmpty(vuln.Advisory.ID, "cargo-audit"),
			fmt.Sprintf("%s: %s", vuln.Package.Name, truncateMessage(msg, 180)),
		))
	}
	count := report.Vulnerabilities.Count
	if count == 0 {
		count = len(res.Findings)
	}
	res.RawScore = clampScore(100 - count*20)
	return res
}
