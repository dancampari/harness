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

type GoVet struct{}
type GoTest struct{}
type GoTestCoverage struct{}
type Staticcheck struct{}
type Govulncheck struct{}

func (GoVet) Name() string                 { return "go-vet" }
func (GoVet) Dimension() sensors.Dimension { return sensors.DimCorrectness }
func (GoVet) Available(root string) bool {
	return detect.HasFile(root, "go.mod") && hasProjectCommand(root, "go")
}

func (GoTest) Name() string                 { return "go-test" }
func (GoTest) Dimension() sensors.Dimension { return sensors.DimCorrectness }
func (GoTest) Available(root string) bool {
	return detect.HasFile(root, "go.mod") && hasProjectCommand(root, "go")
}

func (GoTestCoverage) Name() string                 { return "go-test-coverage" }
func (GoTestCoverage) Dimension() sensors.Dimension { return sensors.DimCoverage }
func (GoTestCoverage) Available(root string) bool {
	return detect.HasFile(root, "go.mod") && hasProjectCommand(root, "go")
}

func (Staticcheck) Name() string                 { return "staticcheck" }
func (Staticcheck) Dimension() sensors.Dimension { return sensors.DimCorrectness }
func (Staticcheck) Available(root string) bool {
	return detect.HasFile(root, "go.mod") && hasProjectCommand(root, "staticcheck")
}

func (Govulncheck) Name() string                 { return "govulncheck" }
func (Govulncheck) Dimension() sensors.Dimension { return sensors.DimSecurity }
func (Govulncheck) Available(root string) bool {
	return detect.HasFile(root, "go.mod") && hasProjectCommand(root, "govulncheck")
}

func (g GoVet) Run(ctx context.Context, root string) sensors.Result {
	start := time.Now()
	res := sensors.Result{SensorName: g.Name(), Dimension: g.Dimension()}
	cmd := exec.CommandContext(ctx, commandOrName(root, "go"), "vet", "./...")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	res.Duration = time.Since(start)
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			res.Error = err.Error()
			return res
		}
	}
	res.Findings = parseTextToolFindings(sensors.DimCorrectness, "go-vet", string(out), sensors.SeverityHigh)
	res.RawScore = clampScore(100 - len(res.Findings)*10)
	return res
}

func (g GoTest) Run(ctx context.Context, root string) sensors.Result {
	start := time.Now()
	res := sensors.Result{SensorName: g.Name(), Dimension: g.Dimension()}
	cmd := exec.CommandContext(ctx, commandOrName(root, "go"), "test", "./...")
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
		"go-test-failure",
		truncateMessage(string(out), 240),
	))
	return res
}

var goCoverageTotalRE = regexp.MustCompile(`^total:\s+\(statements\)\s+([0-9.]+)%$`)

func parseGoCoverageTotal(output string) int {
	for _, line := range strings.Split(output, "\n") {
		m := goCoverageTotalRE.FindStringSubmatch(strings.TrimSpace(line))
		if len(m) == 2 {
			pct, _ := strconv.ParseFloat(m[1], 64)
			return int(pct)
		}
	}
	return 0
}

func (g GoTestCoverage) Run(ctx context.Context, root string) sensors.Result {
	start := time.Now()
	res := sensors.Result{SensorName: g.Name(), Dimension: g.Dimension()}
	coveragePath := filepath.Join(root, ".harness", "tmp", "go-coverage.out")
	_ = os.MkdirAll(filepath.Dir(coveragePath), 0o755)
	cmd := exec.CommandContext(ctx, commandOrName(root, "go"), "test", "./...", "-coverprofile", coveragePath)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			res.Duration = time.Since(start)
			res.Error = err.Error()
			return res
		}
		res.RawScore = 0
		res.Findings = append(res.Findings, finding(
			sensors.DimCoverage,
			sensors.SeverityHigh,
			"",
			0,
			"go-coverage-test-failure",
			truncateMessage(string(out), 240),
		))
		res.Duration = time.Since(start)
		return res
	}
	coverCmd := exec.CommandContext(ctx, commandOrName(root, "go"), "tool", "cover", "-func", coveragePath)
	coverCmd.Dir = root
	coverOut, err := coverCmd.Output()
	res.Duration = time.Since(start)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.RawScore = clampScore(parseGoCoverageTotal(string(coverOut)))
	if res.RawScore < 70 {
		res.Findings = append(res.Findings, finding(
			sensors.DimCoverage,
			sensors.SeverityHigh,
			".harness/tmp/go-coverage.out",
			0,
			"coverage-below-bar",
			fmt.Sprintf("go coverage is %d%%", res.RawScore),
		))
	}
	return res
}

type staticcheckIssue struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Location struct {
		File string `json:"file"`
		Line int    `json:"line"`
	} `json:"location"`
	Message string `json:"message"`
}

func (s Staticcheck) Run(ctx context.Context, root string) sensors.Result {
	start := time.Now()
	res := sensors.Result{SensorName: s.Name(), Dimension: s.Dimension()}
	cmd := exec.CommandContext(ctx, commandOrName(root, "staticcheck"), "-f", "json", "./...")
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
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var issue staticcheckIssue
		if err := json.Unmarshal([]byte(line), &issue); err != nil {
			continue
		}
		res.Findings = append(res.Findings, finding(
			sensors.DimCorrectness,
			sensors.SeverityMedium,
			slashRel(root, issue.Location.File),
			issue.Location.Line,
			nonEmpty(issue.Code, "staticcheck"),
			issue.Message,
		))
	}
	res.RawScore = clampScore(100 - len(res.Findings)*5)
	return res
}

func (g Govulncheck) Run(ctx context.Context, root string) sensors.Result {
	start := time.Now()
	res := sensors.Result{SensorName: g.Name(), Dimension: g.Dimension()}
	cmd := exec.CommandContext(ctx, commandOrName(root, "govulncheck"), "./...")
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
		sensors.DimSecurity,
		sensors.SeverityHigh,
		"go.mod",
		0,
		"govulncheck-finding",
		truncateMessage(string(out), 240),
	))
	return res
}

func parseTextToolFindings(dim sensors.Dimension, rule, output string, severity sensors.Severity) []sensors.Finding {
	var findings []sensors.Finding
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "# ") {
			continue
		}
		file, lineNo, msg := parseToolLine(line)
		findings = append(findings, finding(dim, severity, file, lineNo, rule, msg))
	}
	return findings
}
