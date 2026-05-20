package adapters

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dancampari/harness/internal/detect"
	"github.com/dancampari/harness/internal/sensors"
)

type Ruff struct{}
type Mypy struct{}
type Pytest struct{}
type PytestCoverage struct{}
type PipAudit struct{}

func (Ruff) Name() string                 { return "ruff" }
func (Ruff) Dimension() sensors.Dimension { return sensors.DimCorrectness }
func (Ruff) Available(root string) bool {
	return isPythonProject(root) && hasProjectCommand(root, "ruff")
}

func (Mypy) Name() string                 { return "mypy" }
func (Mypy) Dimension() sensors.Dimension { return sensors.DimCorrectness }
func (Mypy) Available(root string) bool {
	return isPythonProject(root) && hasProjectCommand(root, "mypy")
}

func (Pytest) Name() string                 { return "pytest" }
func (Pytest) Dimension() sensors.Dimension { return sensors.DimCorrectness }
func (Pytest) Available(root string) bool {
	return isPythonProject(root) && hasProjectCommand(root, "pytest")
}

func (PytestCoverage) Name() string                 { return "pytest-cov" }
func (PytestCoverage) Dimension() sensors.Dimension { return sensors.DimCoverage }
func (PytestCoverage) Available(root string) bool {
	return isPythonProject(root) && hasProjectCommand(root, "pytest")
}

func (PipAudit) Name() string                 { return "pip-audit" }
func (PipAudit) Dimension() sensors.Dimension { return sensors.DimSecurity }
func (PipAudit) Available(root string) bool {
	return isPythonProject(root) && hasProjectCommand(root, "pip-audit")
}

func isPythonProject(root string) bool {
	return detect.HasFile(root, "pyproject.toml") ||
		detect.HasFile(root, "requirements.txt") ||
		detect.HasFile(root, "setup.py")
}

type ruffIssue struct {
	Code     string `json:"code"`
	Filename string `json:"filename"`
	Message  string `json:"message"`
	Location struct {
		Row int `json:"row"`
	} `json:"location"`
}

func (r Ruff) Run(ctx context.Context, root string) sensors.Result {
	start := time.Now()
	res := sensors.Result{SensorName: r.Name(), Dimension: r.Dimension()}
	cmd := exec.CommandContext(ctx, commandOrName(root, "ruff"), "check", "--output-format=json", ".")
	cmd.Dir = root
	out, err := cmd.Output()
	res.Duration = time.Since(start)
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			res.Error = err.Error()
			return res
		}
	}
	var issues []ruffIssue
	if err := json.Unmarshal(out, &issues); err != nil {
		res.Error = fmt.Sprintf("parse ruff output: %v", err)
		return res
	}
	for _, issue := range issues {
		res.Findings = append(res.Findings, finding(
			sensors.DimCorrectness,
			sensors.SeverityMedium,
			slashRel(root, issue.Filename),
			issue.Location.Row,
			nonEmpty(issue.Code, "ruff"),
			issue.Message,
		))
	}
	res.RawScore = clampScore(100 - len(res.Findings)*4)
	return res
}

var mypyLineRE = regexp.MustCompile(`^(.+?):(\d+):(?:(\d+):)?\s*(error|note|warning):\s*(.*?)(?:\s+\[([^\]]+)\])?$`)

func parseMypyOutput(output string) []sensors.Finding {
	var findings []sensors.Finding
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		m := mypyLineRE.FindStringSubmatch(line)
		if len(m) == 0 || m[4] == "note" {
			continue
		}
		lineNo, _ := strconv.Atoi(m[2])
		rule := nonEmpty(m[6], "mypy")
		findings = append(findings, finding(
			sensors.DimCorrectness,
			sensors.SeverityHigh,
			filepath.ToSlash(m[1]),
			lineNo,
			rule,
			m[5],
		))
	}
	return findings
}

func (m Mypy) Run(ctx context.Context, root string) sensors.Result {
	start := time.Now()
	res := sensors.Result{SensorName: m.Name(), Dimension: m.Dimension()}
	cmd := exec.CommandContext(ctx, commandOrName(root, "mypy"), "--show-error-codes", "--no-color-output", ".")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	res.Duration = time.Since(start)
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			res.Error = err.Error()
			return res
		}
	}
	res.Findings = parseMypyOutput(string(out))
	res.RawScore = clampScore(100 - len(res.Findings)*8)
	return res
}

var pytestSummaryRE = regexp.MustCompile(`(?:(\d+)\s+failed)?(?:.*?(\d+)\s+passed)?(?:.*?(\d+)\s+skipped)?`)

func parsePytestSummary(output string, exitCode int) (passed, failed int) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.Trim(line, "= ")
		if !strings.Contains(line, "passed") && !strings.Contains(line, "failed") {
			continue
		}
		m := pytestSummaryRE.FindStringSubmatch(line)
		if len(m) == 0 {
			continue
		}
		if m[1] != "" {
			failed, _ = strconv.Atoi(m[1])
		}
		if m[2] != "" {
			passed, _ = strconv.Atoi(m[2])
		}
	}
	if exitCode != 0 && failed == 0 {
		failed = 1
	}
	return passed, failed
}

func (p Pytest) Run(ctx context.Context, root string) sensors.Result {
	start := time.Now()
	res := sensors.Result{SensorName: p.Name(), Dimension: p.Dimension()}
	cmd := exec.CommandContext(ctx, commandOrName(root, "pytest"), "-q")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	res.Duration = time.Since(start)
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			res.Error = err.Error()
			return res
		}
	}
	if exitCode == 5 {
		res.RawScore = 0
		res.Findings = append(res.Findings, finding(
			sensors.DimCorrectness,
			sensors.SeverityHigh,
			"",
			0,
			"no-tests-found",
			"pytest did not discover tests",
		))
		return res
	}
	passed, failed := parsePytestSummary(string(out), exitCode)
	total := passed + failed
	if total == 0 && exitCode == 0 {
		res.RawScore = 100
		return res
	}
	if total == 0 {
		total = 1
	}
	res.RawScore = int(float64(passed) / float64(total) * 100)
	if failed > 0 && res.RawScore > 50 {
		res.RawScore = 50
	}
	if failed > 0 {
		res.Findings = append(res.Findings, finding(
			sensors.DimCorrectness,
			sensors.SeverityCritical,
			"",
			0,
			"pytest-failure",
			truncateMessage(string(out), 240),
		))
	}
	return res
}

type pytestCoverageJSON struct {
	Totals struct {
		PercentCovered float64 `json:"percent_covered"`
	} `json:"totals"`
}

func (p PytestCoverage) Run(ctx context.Context, root string) sensors.Result {
	start := time.Now()
	res := sensors.Result{SensorName: p.Name(), Dimension: p.Dimension()}
	coveragePath := filepath.Join(root, "coverage", "coverage.json")
	_ = os.MkdirAll(filepath.Dir(coveragePath), 0o755)
	cmd := exec.CommandContext(ctx, commandOrName(root, "pytest"), "--cov=.", "--cov-report=json:coverage/coverage.json", "-q")
	cmd.Dir = root
	_, err := cmd.CombinedOutput()
	res.Duration = time.Since(start)
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			res.Error = err.Error()
			return res
		}
	}
	b, err := os.ReadFile(coveragePath)
	if err != nil {
		res.RawScore = 0
		res.Findings = append(res.Findings, finding(
			sensors.DimCoverage,
			sensors.SeverityHigh,
			"coverage/coverage.json",
			0,
			"coverage-missing",
			"pytest-cov did not produce coverage/coverage.json",
		))
		return res
	}
	var report pytestCoverageJSON
	if err := json.Unmarshal(b, &report); err != nil {
		res.Error = fmt.Sprintf("parse pytest coverage: %v", err)
		return res
	}
	res.RawScore = clampScore(int(report.Totals.PercentCovered))
	if res.RawScore < 70 {
		res.Findings = append(res.Findings, finding(
			sensors.DimCoverage,
			sensors.SeverityHigh,
			"coverage/coverage.json",
			0,
			"coverage-below-bar",
			fmt.Sprintf("line coverage is %.1f%%", report.Totals.PercentCovered),
		))
	}
	return res
}

type pipAuditReport struct {
	Dependencies []struct {
		Name  string `json:"name"`
		Vulns []struct {
			ID          string `json:"id"`
			Description string `json:"description"`
		} `json:"vulns"`
	} `json:"dependencies"`
}

func (p PipAudit) Run(ctx context.Context, root string) sensors.Result {
	start := time.Now()
	res := sensors.Result{SensorName: p.Name(), Dimension: p.Dimension()}
	cmd := exec.CommandContext(ctx, commandOrName(root, "pip-audit"), "-f", "json")
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
	var report pipAuditReport
	if err := json.Unmarshal(out, &report); err != nil {
		res.Error = fmt.Sprintf("parse pip-audit output: %v", err)
		return res
	}
	count := 0
	for _, dep := range report.Dependencies {
		for _, vuln := range dep.Vulns {
			count++
			res.Findings = append(res.Findings, finding(
				sensors.DimSecurity,
				sensors.SeverityHigh,
				"pyproject.toml",
				0,
				nonEmpty(vuln.ID, "pip-audit"),
				fmt.Sprintf("%s: %s", dep.Name, truncateMessage(vuln.Description, 180)),
			))
		}
	}
	res.RawScore = clampScore(100 - count*15)
	return res
}

func nonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
