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

// Size classifies the sprint's scope so the planning policy can decide
// when design and task artifacts are mandatory. Small sprints stay
// lightweight; large sprints force explicit decomposition.
//
// Valid values: "small", "medium", "large". An empty Size defaults to
// "medium" at policy time.
type Size string

const (
	SizeSmall  Size = "small"
	SizeMedium Size = "medium"
	SizeLarge  Size = "large"
)

// Contract is the parsed structure of a contracts/sprint-NNN.md file.
type Contract struct {
	SprintNumber int
	Title        string
	Goal         string
	Size         Size
	Requirements []Requirement
	Deliverables []Deliverable
	Criteria     []AcceptanceCriterion
	Constraints  Constraints
	RawMarkdown  string
}

// Requirement is one entry in the optional `## Requirements` section.
// Requirements give acceptance criteria, deliverables, and tasks a stable
// identifier for traceability across the sprint.
type Requirement struct {
	ID        string // e.g. "REQ-001"
	Statement string
}

// Deliverable is a single artifact the sprint must produce.
type Deliverable struct {
	Path          string
	MustExport    []string
	RequirementID string // optional REQ-NNN linkage
}

// Evidence describes how an acceptance criterion is mechanically verified.
//
// Recognised kinds:
//   - "tests": Ref is a test name substring searched across test files.
//   - "e2e": Ref is a path (relative to repo root) that must exist.
//   - "fixture": Ref is the name of a JSON file under .harness/fixtures/.
//   - "inspection": Ref is a human description; the criterion is marked as
//     requiring manual review but does not fail mechanically.
//
// An empty Kind means the criterion declares no mechanical evidence; the
// contract dimension falls back to deliverable-only structural checks.
type Evidence struct {
	Kind string
	Ref  string
}

// AcceptanceCriterion is one row in the acceptance table.
type AcceptanceCriterion struct {
	Number        int
	RequirementID string // optional REQ-NNN linkage
	Statement     string
	Evidence      Evidence
	Threshold     int // 1-10
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
	criteriaRowLegacy := regexp.MustCompile(`^\|\s*(\d+)\s*\|\s*([^|]+?)\s*\|\s*(\d+)/10\s*\|`)
	criteriaRowFull := regexp.MustCompile(`^\|\s*(\d+)\s*\|\s*(REQ-\d+)?\s*\|\s*([^|]+?)\s*\|\s*([^|]*?)\s*\|\s*(\d+)/10\s*\|`)
	titleRow := regexp.MustCompile(`^#\s+Sprint\s+(\d+)\s*[—\-]\s*(.+)$`)
	delivPath := regexp.MustCompile("^[-*]\\s+`([^`]+)`")
	codeSpan := regexp.MustCompile("`([^`]+)`")
	requirementRow := regexp.MustCompile(`^[-*]\s+(REQ-\d+)\s*[:\-]\s*(.+)$`)
	delivRequirement := regexp.MustCompile(`\b(REQ-\d+)\b`)
	sizeLine := regexp.MustCompile(`(?i)^size\s*[:=]\s*(small|medium|large)\b`)
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
		case "size":
			if m := sizeLine.FindStringSubmatch(trimmed); m != nil {
				c.Size = Size(strings.ToLower(m[1]))
			} else if trimmed != "" && !strings.HasPrefix(trimmed, "<") && !strings.HasPrefix(trimmed, "#") {
				// Tolerate "small", "medium", "large" on its own line.
				lower := strings.ToLower(trimmed)
				switch lower {
				case "small", "medium", "large":
					c.Size = Size(lower)
				}
			}
		case "requirements":
			if m := requirementRow.FindStringSubmatch(trimmed); m != nil {
				c.Requirements = append(c.Requirements, Requirement{
					ID:        m[1],
					Statement: strings.TrimSpace(m[2]),
				})
			}
		case "deliverables":
			if m := delivPath.FindStringSubmatch(trimmed); m != nil {
				d := Deliverable{Path: m[1]}
				if idx := strings.Index(strings.ToLower(trimmed), "exports:"); idx >= 0 {
					for _, span := range codeSpan.FindAllStringSubmatch(trimmed[idx:], -1) {
						d.MustExport = append(d.MustExport, span[1])
					}
				}
				if rm := delivRequirement.FindStringSubmatch(trimmed); rm != nil {
					d.RequirementID = rm[1]
				}
				c.Deliverables = append(c.Deliverables, d)
			}
		case "acceptance criteria":
			if m := criteriaRowFull.FindStringSubmatch(trimmed); m != nil {
				var num, th int
				fmt.Sscanf(m[1], "%d", &num)
				fmt.Sscanf(m[5], "%d", &th)
				c.Criteria = append(c.Criteria, AcceptanceCriterion{
					Number:        num,
					RequirementID: m[2],
					Statement:     strings.TrimSpace(m[3]),
					Evidence:      parseEvidence(m[4]),
					Threshold:     th,
				})
			} else if m := criteriaRowLegacy.FindStringSubmatch(trimmed); m != nil {
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

// EffectiveSize returns the declared sprint size or an empty Size when
// the contract did not declare one. An empty Size disables the
// size-based policy gates so legacy contracts authored before v0.8.5
// continue to validate without changes. New contracts get a `medium`
// default through the template at sprint creation time.
func (c *Contract) EffectiveSize() Size {
	switch c.Size {
	case SizeSmall, SizeMedium, SizeLarge:
		return c.Size
	default:
		return ""
	}
}

// parseEvidence converts a cell value into an Evidence struct. The expected
// format is "<kind>:<ref>" with kind in {tests, e2e, fixture, inspection}.
// An empty cell yields zero-value Evidence and is treated as "no mechanical
// check declared" by CheckAgainstDiff.
func parseEvidence(raw string) Evidence {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Evidence{}
	}
	idx := strings.IndexByte(raw, ':')
	if idx < 0 {
		return Evidence{Kind: "inspection", Ref: raw}
	}
	kind := strings.ToLower(strings.TrimSpace(raw[:idx]))
	ref := strings.TrimSpace(raw[idx+1:])
	switch kind {
	case "tests", "test":
		return Evidence{Kind: "tests", Ref: ref}
	case "e2e":
		return Evidence{Kind: "e2e", Ref: ref}
	case "fixture", "fixtures":
		return Evidence{Kind: "fixture", Ref: ref}
	case "inspection", "manual":
		return Evidence{Kind: "inspection", Ref: ref}
	default:
		return Evidence{Kind: kind, Ref: ref}
	}
}

// Validate checks that the contract is structurally complete.
// It does NOT check semantics — that's outside the harness's scope.
//
// When `## Requirements` is present, Validate also enforces internal
// integrity: every REQ-ID referenced by a deliverable or criterion must
// resolve to a declared requirement, and every declared requirement must
// be referenced at least once. Contracts without `## Requirements` keep
// the legacy structural behavior so older sprints stay parseable.
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
	if len(c.Requirements) > 0 {
		known := map[string]bool{}
		for _, r := range c.Requirements {
			if r.ID == "" {
				continue
			}
			if known[r.ID] {
				errors = append(errors, fmt.Sprintf("duplicate requirement %s", r.ID))
				continue
			}
			known[r.ID] = true
		}
		referenced := map[string]bool{}
		for _, d := range c.Deliverables {
			if d.RequirementID == "" {
				continue
			}
			if !known[d.RequirementID] {
				errors = append(errors, fmt.Sprintf("deliverable %q references undefined %s",
					d.Path, d.RequirementID))
				continue
			}
			referenced[d.RequirementID] = true
		}
		for _, cr := range c.Criteria {
			if cr.RequirementID == "" {
				continue
			}
			if !known[cr.RequirementID] {
				errors = append(errors, fmt.Sprintf("criterion #%d references undefined %s",
					cr.Number, cr.RequirementID))
				continue
			}
			referenced[cr.RequirementID] = true
		}
		for _, r := range c.Requirements {
			if r.ID == "" {
				continue
			}
			if !referenced[r.ID] {
				errors = append(errors, fmt.Sprintf("requirement %s is declared but not referenced by any deliverable or criterion", r.ID))
			}
		}
	}
	return errors
}

func TemplatePlaceholderErrors(raw string) []string {
	if raw == "" {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		out = append(out, value)
	}
	angleRe := regexp.MustCompile(`(?s)<[^>]+>`)
	placeholderHints := []string{
		"expand", "small |", "optional", "required", "reference",
		"first requirement", "second requirement", "observable",
		"action", "input", "empty input", "network failure",
		"concurrent edits", "not in this sprint", "note any",
		"assumption", "path/to", "function",
	}
	for _, match := range angleRe.FindAllString(raw, -1) {
		lower := strings.ToLower(match)
		for _, hint := range placeholderHints {
			if strings.Contains(lower, hint) {
				add(match)
				break
			}
		}
	}
	for _, token := range []string{
		"`path/to/file.ts`",
		"`path/to/another.ts`",
		"`functionName`",
		"`anotherFn`",
		"feature-error-response",
	} {
		if strings.Contains(raw, token) {
			add(token)
		}
	}
	errs := make([]string, 0, len(out))
	for _, placeholder := range out {
		errs = append(errs, fmt.Sprintf("unresolved template placeholder %q", placeholder))
	}
	return errs
}

// CheckAgainstDiff verifies the contract is satisfied by the current
// state of the working tree. This is the "Contract" dimension of the
// evaluator: did the implementer actually deliver what was promised?
//
// Two layers run here:
//
//  1. Structural deliverables: file exists, expected exports present.
//     This has been the harness's contract check since v0.1.
//  2. Acceptance-criterion evidence: when a criterion declares an
//     Evidence cell, the harness mechanically verifies that the named
//     test, e2e spec, or approved fixture is reachable. Criteria
//     without evidence are skipped so legacy contracts keep their
//     pre-existing scores.
//
// Functional correctness still belongs to the deterministic sensors;
// this method only checks that the implementation maps to the promises
// the contract makes.
func (c *Contract) CheckAgainstDiff(root string) evaluator.ContractCheckResult {
	res := evaluator.ContractCheckResult{Status: "satisfied"}
	totalChecks := 0
	passedChecks := 0

	for _, d := range c.Deliverables {
		totalChecks++
		path, found := resolveDeliverablePath(root, d.Path)
		if !found {
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

	for _, cr := range c.Criteria {
		if cr.Evidence.Kind == "" {
			continue
		}
		totalChecks++
		if checkCriterionEvidence(root, cr) {
			passedChecks++
			continue
		}
		res.UnmetCriteria = append(res.UnmetCriteria, formatUnmetCriterion(cr))
	}

	if len(res.MissingDeliverables) > 0 || len(res.UnmetCriteria) > 0 {
		res.Status = "partial"
	}

	// Score: percentage of declared structural promises satisfied.
	// A deliverable path is one check; each declared export is another;
	// each criterion with declared evidence is another.
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

func resolveDeliverablePath(root, contractPath string) (string, bool) {
	path := contractPath
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	if _, err := os.Stat(path); err == nil {
		return path, true
	}

	pattern, ok := deliverablePlaceholderGlob(root, contractPath)
	if !ok {
		return path, false
	}
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return path, false
	}
	for _, match := range matches {
		if _, err := os.Stat(match); err == nil {
			return match, true
		}
	}
	return path, false
}

var deliverablePlaceholderToken = regexp.MustCompile(`<[A-Za-z0-9_-]+>`)

func deliverablePlaceholderGlob(root, contractPath string) (string, bool) {
	path := contractPath
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	path = filepath.Clean(path)
	if !deliverablePlaceholderToken.MatchString(path) {
		return "", false
	}
	return deliverablePlaceholderToken.ReplaceAllString(path, "*"), true
}
func formatUnmetCriterion(cr AcceptanceCriterion) string {
	prefix := fmt.Sprintf("criterion #%d", cr.Number)
	if cr.RequirementID != "" {
		prefix = fmt.Sprintf("%s (%s)", cr.RequirementID, prefix)
	}
	ref := cr.Evidence.Ref
	switch cr.Evidence.Kind {
	case "tests":
		return fmt.Sprintf("%s evidence `tests:%s` not found in any test file", prefix, ref)
	case "e2e":
		return fmt.Sprintf("%s e2e spec `%s` does not exist", prefix, ref)
	case "fixture":
		return fmt.Sprintf("%s approved fixture `%s` is missing from .harness/fixtures/", prefix, ref)
	case "inspection":
		return fmt.Sprintf("%s requires manual inspection: %s", prefix, ref)
	default:
		return fmt.Sprintf("%s has unrecognized evidence kind `%s`", prefix, cr.Evidence.Kind)
	}
}

// checkCriterionEvidence answers whether the criterion's declared evidence
// can be located in the repository. The check is intentionally loose: it
// confirms the artifact exists, not that it actually validates the
// behavior. Functional verification stays the job of the test/fixture
// itself, which runs under its sensor in the same QA pass.
//
// inspection evidence always fails the mechanical check so the user gets
// a visible reminder; the criterion can still pass once the evidence is
// upgraded to a mechanical kind or once the contract drops the row.
func checkCriterionEvidence(root string, cr AcceptanceCriterion) bool {
	switch cr.Evidence.Kind {
	case "tests":
		return findTestEvidence(root, cr.Evidence.Ref)
	case "e2e":
		if cr.Evidence.Ref == "" {
			return false
		}
		path := cr.Evidence.Ref
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		_, err := os.Stat(path)
		return err == nil
	case "fixture":
		if cr.Evidence.Ref == "" {
			return false
		}
		name := cr.Evidence.Ref
		if !strings.HasSuffix(name, ".json") {
			name += ".json"
		}
		path := filepath.Join(root, ".harness", "fixtures", name)
		_, err := os.Stat(path)
		return err == nil
	case "inspection":
		return false
	default:
		return false
	}
}

// findTestEvidence walks common test directories looking for a file that
// references the given test name as a substring. The search is bounded
// to a handful of recognised test-file suffixes so it stays cheap.
func findTestEvidence(root, ref string) bool {
	if ref == "" {
		return false
	}
	matched := false
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if matched {
			return filepath.SkipDir
		}
		if info.IsDir() {
			base := filepath.Base(path)
			switch base {
			case "node_modules", ".git", "dist", "build", "coverage", ".harness":
				return filepath.SkipDir
			}
			return nil
		}
		if !isTestFile(info.Name()) {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if strings.Contains(string(b), ref) {
			matched = true
		}
		return nil
	})
	return matched
}

func isTestFile(name string) bool {
	lower := strings.ToLower(name)
	for _, suffix := range []string{
		".test.ts", ".test.tsx", ".test.js", ".test.jsx",
		".spec.ts", ".spec.tsx", ".spec.js", ".spec.jsx",
		"_test.go", "_test.py",
	} {
		if strings.HasSuffix(lower, suffix) {
			return true
		}
	}
	if strings.HasPrefix(lower, "test_") && strings.HasSuffix(lower, ".py") {
		return true
	}
	return false
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

## Size
<small | medium | large. Sets the planning depth for this sprint.
- small: ≤3 deliverables, no design/tasks file required.
- medium: tasks plan in .specs/features/sprint-NNN/tasks.md is required.
- large: both design (.specs/features/sprint-NNN/design.md) and tasks plans required.
Default below; bump to medium/large when scope grows.>

small

## Requirements
<optional; required by spec-driven planning. Declare requirement IDs so
deliverables, criteria, and tasks can reference them.>

- REQ-001: <first requirement>
- REQ-002: <second requirement>

## Deliverables
<reference a REQ in the same line when the deliverable closes that
requirement. The reference is plain text; the parser picks it up.>

- ` + "`path/to/file.ts`" + ` exports: ` + "`functionName`" + ` (REQ-001)
- ` + "`path/to/another.ts`" + ` exports: ` + "`anotherFn`" + ` (REQ-002)

## Acceptance Criteria
<5-column form enforces traceability + mechanical evidence. Every
criterion must be in TLC's WHEN/THEN/SHALL form so the precondition,
action, and observable outcome are unambiguous. Evidence kinds:
tests:<name>, e2e:<path>, fixture:<name>, inspection:<note>.>

| # | REQ     | Criterion                                                                | Evidence                              | Threshold |
|---|---------|--------------------------------------------------------------------------|---------------------------------------|-----------|
| 1 | REQ-001 | WHEN <action> THEN the system SHALL <observable outcome>                 | tests:functionName handles edge case  | 8/10      |
| 2 | REQ-001 | WHEN <user-side action> THEN the UI SHALL <observable outcome>           | e2e:tests/e2e/feature.spec.ts         | 7/10      |
| 3 | REQ-002 | WHEN <input> THEN the system SHALL <observable outcome>                  | fixture:feature-error-response        | 9/10      |

## Edge Cases
<TLC-mandated section: list boundary and failure scenarios. Empty
"none" is allowed only for trivial sprints.>

- <empty input>
- <network failure>
- <concurrent edits>

## Out of Scope
<TLC-mandated section: deferred work made explicit so it does not
silently leak into implementation.>

- <not in this sprint>

## Constraints
- forbidden_imports: ` + "`src/domain/* → src/ui/*`" + `
- max_function_complexity: 10
- coverage_min: 80
`
