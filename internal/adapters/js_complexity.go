package adapters

import (
	"bufio"
	"context"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/dancampari/harness/internal/detect"
	"github.com/dancampari/harness/internal/sensors"
)

type JSComplexity struct{}

func (JSComplexity) Name() string                 { return "js-complexity" }
func (JSComplexity) Dimension() sensors.Dimension { return sensors.DimComplexity }

func (JSComplexity) Available(root string) bool {
	return detect.HasFile(root, "package.json")
}

var jsFunctionStart = regexp.MustCompile(`\b(function\s+\w+|\w+\s*=\s*(async\s*)?\([^)]*\)\s*=>|\w+\s*\([^)]*\)\s*\{)`)

func (j JSComplexity) Run(ctx context.Context, root string) sensors.Result {
	start := time.Now()
	res := sensors.Result{
		SensorName: j.Name(),
		Dimension:  j.Dimension(),
		RawScore:   100,
	}
	files, err := nodeSourceFiles(root)
	if err != nil {
		res.Error = err.Error()
		res.Duration = time.Since(start)
		return res
	}

	violations := 0
	for _, path := range files {
		select {
		case <-ctx.Done():
			res.Error = ctx.Err().Error()
			res.Duration = time.Since(start)
			return res
		default:
		}
		fileViolations := analyzeComplexityFile(root, path)
		violations += len(fileViolations)
		res.Findings = append(res.Findings, fileViolations...)
	}
	res.RawScore = clampScore(100 - violations*8)
	res.Duration = time.Since(start)
	return res
}

func analyzeComplexityFile(root, path string) []sensors.Finding {
	f, err := os.Open(path)
	if err != nil {
		return []sensors.Finding{finding(
			sensors.DimComplexity,
			sensors.SeverityMedium,
			slashRel(root, path),
			0,
			"complexity-read-error",
			err.Error(),
		)}
	}
	defer f.Close()

	var findings []sensors.Finding
	scanner := bufio.NewScanner(f)
	lineNo := 0
	inFn := false
	fnLine := 0
	fnComplexity := 1
	fnLines := 0
	braceDepth := 0
	maxDepth := 0
	for scanner.Scan() {
		lineNo++
		line := stripInlineComment(scanner.Text())
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !inFn && jsFunctionStart.MatchString(trimmed) {
			inFn = true
			fnLine = lineNo
			fnComplexity = 1
			fnLines = 0
			braceDepth = 0
			maxDepth = 0
		}
		if !inFn {
			continue
		}
		fnLines++
		fnComplexity += decisionCount(trimmed)
		braceDepth += strings.Count(trimmed, "{")
		if braceDepth > maxDepth {
			maxDepth = braceDepth
		}
		braceDepth -= strings.Count(trimmed, "}")
		if braceDepth <= 0 && strings.Contains(trimmed, "}") {
			if fnComplexity > 10 {
				findings = append(findings, finding(
					sensors.DimComplexity,
					sensors.SeverityHigh,
					slashRel(root, path),
					fnLine,
					"cyclomatic-complexity",
					"function complexity exceeds 10",
				))
			}
			if fnLines > 80 {
				findings = append(findings, finding(
					sensors.DimComplexity,
					sensors.SeverityMedium,
					slashRel(root, path),
					fnLine,
					"function-size",
					"function exceeds 80 non-empty lines",
				))
			}
			if maxDepth > 5 {
				findings = append(findings, finding(
					sensors.DimComplexity,
					sensors.SeverityMedium,
					slashRel(root, path),
					fnLine,
					"nesting-depth",
					"function nesting depth exceeds 5",
				))
			}
			inFn = false
		}
	}
	return findings
}

func stripInlineComment(line string) string {
	if idx := strings.Index(line, "//"); idx >= 0 {
		return line[:idx]
	}
	return line
}

func decisionCount(line string) int {
	count := 0
	tokens := []string{"if ", "for ", "while ", "case ", "catch ", "&&", "||", "?"}
	for _, token := range tokens {
		count += strings.Count(line, token)
	}
	return count
}
