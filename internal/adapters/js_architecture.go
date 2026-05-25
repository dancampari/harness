package adapters

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/dancampari/harness/internal/agreement"
	"github.com/dancampari/harness/internal/detect"
	"github.com/dancampari/harness/internal/sensors"
)

type JSArchitecture struct{}

func (JSArchitecture) Name() string                 { return "js-architecture" }
func (JSArchitecture) Dimension() sensors.Dimension { return sensors.DimArchitecture }

func (JSArchitecture) Available(root string) bool {
	return detect.HasFile(root, "package.json")
}

type importEdge struct {
	From string
	To   string
	Line int
}

type forbiddenImportRule struct {
	From string
	To   string
}

var importPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\bimport\s+(?:.+?\s+from\s+)?["']([^"']+)["']`),
	regexp.MustCompile(`\brequire\(["']([^"']+)["']\)`),
	regexp.MustCompile(`\bexport\s+.+?\s+from\s+["']([^"']+)["']`),
}

func (j JSArchitecture) Run(ctx context.Context, root string) sensors.Result {
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
	fileSet := map[string]bool{}
	for _, path := range files {
		fileSet[slashRel(root, path)] = true
	}

	var edges []importEdge
	for _, path := range files {
		select {
		case <-ctx.Done():
			res.Error = ctx.Err().Error()
			res.Duration = time.Since(start)
			return res
		default:
		}
		edges = append(edges, parseImports(root, path, fileSet)...)
	}

	rules := readForbiddenImportRules(root)
	for _, edge := range edges {
		for _, rule := range rules {
			if pathMatches(rule.From, edge.From) && pathMatches(rule.To, edge.To) {
				res.Findings = append(res.Findings, finding(
					sensors.DimArchitecture,
					sensors.SeverityHigh,
					edge.From,
					edge.Line,
					"forbidden-import",
					fmt.Sprintf("%s imports %s, violating %s -> %s", edge.From, edge.To, rule.From, rule.To),
				))
			}
		}
	}

	for _, cycle := range findImportCycles(edges) {
		res.Findings = append(res.Findings, finding(
			sensors.DimArchitecture,
			sensors.SeverityHigh,
			cycle[0],
			0,
			"import-cycle",
			"import cycle: "+strings.Join(cycle, " -> "),
		))
	}

	res.RawScore = clampScore(100 - len(res.Findings)*10)
	res.Duration = time.Since(start)
	return res
}

func parseImports(root, path string, fileSet map[string]bool) []importEdge {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var edges []importEdge
	from := slashRel(root, path)
	scanner := bufio.NewScanner(f)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		for _, pattern := range importPatterns {
			for _, match := range pattern.FindAllStringSubmatch(line, -1) {
				if len(match) < 2 || !strings.HasPrefix(match[1], ".") {
					continue
				}
				if to, ok := resolveImport(root, filepath.Dir(path), match[1], fileSet); ok {
					edges = append(edges, importEdge{From: from, To: to, Line: lineNo})
				}
			}
		}
	}
	return edges
}

func resolveImport(root, fromDir, spec string, fileSet map[string]bool) (string, bool) {
	base := filepath.Clean(filepath.Join(fromDir, spec))
	candidates := []string{base}
	for _, ext := range []string{".ts", ".tsx", ".js", ".jsx"} {
		candidates = append(candidates, base+ext, filepath.Join(base, "index"+ext))
	}
	for _, candidate := range candidates {
		rel := slashRel(root, candidate)
		if fileSet[rel] {
			return rel, true
		}
	}
	return "", false
}

func readForbiddenImportRules(root string) []forbiddenImportRule {
	mgr := agreement.NewManager(filepath.Join(root, ".harness"))
	number, err := mgr.CurrentSprintNumber()
	if err != nil || number == 0 {
		return nil
	}
	path := mgr.ContractPath(number)
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	rulePattern := regexp.MustCompile("forbidden_imports?:\\s*`([^`]+)`")
	var rules []forbiddenImportRule
	for _, match := range rulePattern.FindAllStringSubmatch(string(b), -1) {
		parts := splitForbiddenRule(match[1])
		if len(parts) == 2 {
			rules = append(rules, forbiddenImportRule{From: parts[0], To: parts[1]})
		}
	}
	return rules
}

func splitForbiddenRule(rule string) []string {
	rule = strings.ReplaceAll(rule, "→", "->")
	rule = strings.ReplaceAll(rule, "â†’", "->")
	parts := strings.Split(rule, "->")
	if len(parts) != 2 {
		return nil
	}
	return []string{strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])}
}

func pathMatches(pattern, path string) bool {
	pattern = filepath.ToSlash(strings.TrimSpace(pattern))
	path = filepath.ToSlash(path)
	if strings.HasSuffix(pattern, "/*") {
		return strings.HasPrefix(path, strings.TrimSuffix(pattern, "*"))
	}
	ok, err := filepath.Match(pattern, path)
	return err == nil && ok
}

func findImportCycles(edges []importEdge) [][]string {
	graph := map[string][]string{}
	nodes := map[string]bool{}
	for _, edge := range edges {
		graph[edge.From] = append(graph[edge.From], edge.To)
		nodes[edge.From] = true
		nodes[edge.To] = true
	}

	index := 0
	stack := []string{}
	onStack := map[string]bool{}
	indexes := map[string]int{}
	lowlink := map[string]int{}
	var cycles [][]string
	var strongConnect func(string)

	strongConnect = func(v string) {
		indexes[v] = index
		lowlink[v] = index
		index++
		stack = append(stack, v)
		onStack[v] = true

		for _, w := range graph[v] {
			if _, seen := indexes[w]; !seen {
				strongConnect(w)
				if lowlink[w] < lowlink[v] {
					lowlink[v] = lowlink[w]
				}
			} else if onStack[w] && indexes[w] < lowlink[v] {
				lowlink[v] = indexes[w]
			}
		}

		if lowlink[v] != indexes[v] {
			return
		}
		var component []string
		for {
			w := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			onStack[w] = false
			component = append(component, w)
			if w == v {
				break
			}
		}
		if len(component) > 1 {
			sort.Strings(component)
			cycles = append(cycles, component)
		}
	}

	var ordered []string
	for node := range nodes {
		ordered = append(ordered, node)
	}
	sort.Strings(ordered)
	for _, node := range ordered {
		if _, seen := indexes[node]; !seen {
			strongConnect(node)
		}
	}
	return cycles
}
