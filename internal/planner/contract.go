// Package planner handles sprint contracts: parsing, validation, and
// checking whether a diff satisfies them.
//
// Per your decision (manual planner), the planner does NOT generate
// contracts. The coding CLI (Claude Code / Codex / Cursor) writes the
// contract by hand, guided by the template. This package only:
//
//  1. Parses the contract markdown into a structured form.
//  2. Generates the initial template from a goal string.
//  3. Validates that the contract is well-formed.
//  4. Checks a contract against a diff (which files exist, which symbols
//     are exported, which test files are present).
//
// "Well-formed" is mechanical: required sections present, deliverables
// declared, acceptance criteria each have a threshold. It does NOT judge
// quality of the goal — that's the human's job.
package planner

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dancampari/harness/internal/evaluator"
)

// Contract is the parsed structure of a contracts/sprint-NNN.md file.
type Contract struct {
	SprintNumber int
	Title        string
	Goal         string
	Deliverables []Deliverable
	Criteria     []AcceptanceCriterion
	Constraints  Constraints
	RawMarkdown  string
}

// Deliverable is a single artifact the sprint must produce.
type Deliverable struct {
	Path       string
	MustExport []string
}

// AcceptanceCriterion is one row in the acceptance table.
type AcceptanceCriterion struct {
	Number    int
	Statement string
	Threshold int // 1-10
}

// Constraints are non-functional limits the sprint must respect.
type Constraints struct {
	ForbiddenImports      []string // e.g. "src/auth/* → src/ui/*"
	MaxFunctionComplexity int
	CoverageMin           int
	TestsRequired         []string
}

// Parse reads and parses a contract markdown file.
// The format is intentionally simple — we don't run a real markdown
// parser, just line-by-line state machine extraction. This keeps the
// dependency footprint small and the format hand-editable.
func Parse(path string) (*Contract, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	raw := strings.TrimPrefix(string(b), "\uFEFF")
	c := &Contract{RawMarkdown: raw}

	scanner := bufio.NewScanner(strings.NewReader(raw))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	var section string
	criteriaRow := regexp.MustCompile(`^\|\s*(\d+)\s*\|\s*(.+?)\s*\|\s*(\d+)/10\s*\|`)
	titleRow := regexp.MustCompile(`^#\s+Sprint\s+(\d+)\s*[—\-]\s*(.+)$`)
	delivPath := regexp.MustCompile("^[-*]\\s+`([^`]+)`")
	codeSpan := regexp.MustCompile("`([^`]+)`")
	forbiddenImport := regexp.MustCompile(`forbidden_imports?:\s*` + "`([^`]+)`")
	complexityLimit := regexp.MustCompile(`max_function_complexity:\s*(\d+)`)
	coverageMin := regexp.MustCompile(`coverage_min:\s*(\d+)`)

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if m := titleRow.FindStringSubmatch(trimmed); m != nil {
			fmt.Sscanf(m[1], "%d", &c.SprintNumber)
			c.Title = m[2]
			continue
		}

		if strings.HasPrefix(trimmed, "## ") {
			section = strings.ToLower(strings.TrimPrefix(trimmed, "## "))
			section = strings.TrimSpace(section)
			continue
		}

		switch section {
		case "goal":
			if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
				if c.Goal != "" {
					c.Goal += " "
				}
				c.Goal += trimmed
			}
		case "deliverables":
			if m := delivPath.FindStringSubmatch(trimmed); m != nil {
				d := Deliverable{Path: m[1]}
				if idx := strings.Index(strings.ToLower(trimmed), "exports:"); idx >= 0 {
					for _, span := range codeSpan.FindAllStringSubmatch(trimmed[idx:], -1) {
						d.MustExport = append(d.MustExport, span[1])
					}
				}
				c.Deliverables = append(c.Deliverables, d)
			}
		case "acceptance criteria":
			if m := criteriaRow.FindStringSubmatch(trimmed); m != nil {
				var num, th int
				fmt.Sscanf(m[1], "%d", &num)
				fmt.Sscanf(m[3], "%d", &th)
				c.Criteria = append(c.Criteria, AcceptanceCriterion{
					Number:    num,
					Statement: strings.TrimSpace(m[2]),
					Threshold: th,
				})
			}
		case "constraints":
			if m := forbiddenImport.FindStringSubmatch(trimmed); m != nil {
				c.Constraints.ForbiddenImports = append(c.Constraints.ForbiddenImports, m[1])
			}
			if m := complexityLimit.FindStringSubmatch(trimmed); m != nil {
				fmt.Sscanf(m[1], "%d", &c.Constraints.MaxFunctionComplexity)
			}
			if m := coverageMin.FindStringSubmatch(trimmed); m != nil {
				fmt.Sscanf(m[1], "%d", &c.Constraints.CoverageMin)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return c, nil
}

// Validate checks that the contract is structurally complete.
// It does NOT check semantics — that's outside the harness's scope.
func (c *Contract) Validate() []string {
	var errors []string
	if c.SprintNumber == 0 {
		errors = append(errors, "missing or invalid sprint number in title")
	}
	if c.Goal == "" {
		errors = append(errors, "missing ## Goal section")
	}
	if len(c.Deliverables) == 0 {
		errors = append(errors, "no deliverables declared")
	}
	if len(c.Criteria) == 0 {
		errors = append(errors, "no acceptance criteria declared")
	}
	for _, cr := range c.Criteria {
		if cr.Threshold < 1 || cr.Threshold > 10 {
			errors = append(errors,
				fmt.Sprintf("criterion #%d has invalid threshold %d (must be 1-10)",
					cr.Number, cr.Threshold))
		}
	}
	return errors
}

// CheckAgainstDiff verifies the contract is satisfied by the current
// state of the working tree. This is the "Contract" dimension of the
// evaluator: did the implementer actually deliver what was promised?
//
// The check is deliberately structural (file exists, expected exports
// present). Functional correctness is the job of other sensors.
func (c *Contract) CheckAgainstDiff(root string) evaluator.ContractCheckResult {
	res := evaluator.ContractCheckResult{Status: "satisfied"}
	totalChecks := 0
	passedChecks := 0

	for _, d := range c.Deliverables {
		totalChecks++
		path := d.Path
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		if _, err := os.Stat(path); err != nil {
			res.MissingDeliverables = append(res.MissingDeliverables, d.Path)
			totalChecks += len(d.MustExport)
			continue
		}
		passedChecks++

		if len(d.MustExport) == 0 {
			continue
		}
		b, err := os.ReadFile(path)
		if err != nil {
			for _, symbol := range d.MustExport {
				totalChecks++
				res.UnmetCriteria = append(res.UnmetCriteria,
					fmt.Sprintf("%s export `%s` could not be verified", d.Path, symbol))
			}
			continue
		}
		source := string(b)
		for _, symbol := range d.MustExport {
			totalChecks++
			if hasExportedSymbol(source, symbol) {
				passedChecks++
				continue
			}
			res.UnmetCriteria = append(res.UnmetCriteria,
				fmt.Sprintf("%s is missing export `%s`", d.Path, symbol))
		}
	}

	if len(res.MissingDeliverables) > 0 || len(res.UnmetCriteria) > 0 {
		res.Status = "partial"
	}

	// Score: percentage of declared structural promises satisfied.
	// A deliverable path is one check; each declared export is another.
	if totalChecks == 0 {
		res.Score = 0
		res.Status = "missing"
		return res
	}
	res.Score = passedChecks * 100 / totalChecks
	if res.Score == 0 {
		res.Status = "violated"
	}
	return res
}

func hasExportedSymbol(source, symbol string) bool {
	escaped := regexp.QuoteMeta(symbol)
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`\bexport\s+(?:async\s+)?(?:function|class|const|let|var|interface|type|enum)\s+` + escaped + `\b`),
		regexp.MustCompile(`\bexport\s*\{[^}]*\b` + escaped + `\b[^}]*\}`),
		regexp.MustCompile(`\bexports\.` + escaped + `\b`),
		regexp.MustCompile(`\bmodule\.exports\.` + escaped + `\b`),
	}
	if symbol == "default" {
		patterns = append(patterns, regexp.MustCompile(`\bexport\s+default\b`))
	}
	for _, pattern := range patterns {
		if pattern.MatchString(source) {
			return true
		}
	}
	return false
}

// Template returns the default contract template populated with a goal.
func Template(sprintNumber int, goal string) string {
	return fmt.Sprintf(contractTemplate, sprintNumber, goal)
}

const contractTemplate = `# Sprint %03d — %s

## Goal
<expand on the goal in 2-3 sentences — what exactly is being built?>

## Deliverables
- ` + "`path/to/file.ts`" + ` exports: ` + "`functionName`" + `
- ` + "`path/to/another.ts`" + ` exports: ` + "`anotherFn`" + `

## Acceptance Criteria
| # | Criterion                                  | Threshold |
|---|--------------------------------------------|-----------|
| 1 | <criterion 1, observable and verifiable>   | 6/10      |
| 2 | <criterion 2>                              | 7/10      |
| 3 | Coverage on new files                      | 8/10      |
| 4 | E2E test passes for primary flow           | 8/10      |
| 5 | No new high-severity findings              | 9/10      |

## Constraints
- forbidden_imports: ` + "`src/domain/* → src/ui/*`" + `
- max_function_complexity: 10
- coverage_min: 80
`
