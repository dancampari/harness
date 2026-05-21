package planner

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTempContract(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "sprint-001.md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseLegacyContractRemainsCompatible(t *testing.T) {
	path := writeTempContract(t, `# Sprint 001 — legacy demo

## Goal
Keep the legacy 3-column contract format working without regressions.

## Deliverables
- `+"`src/index.ts`"+` exports: `+"`run`"+`

## Acceptance Criteria
| # | Criterion                       | Threshold |
|---|---------------------------------|-----------|
| 1 | Index module exports run helper | 8/10      |

## Constraints
- max_function_complexity: 10
`)
	c, err := Parse(path)
	if err != nil {
		t.Fatalf("parse legacy contract: %v", err)
	}
	if errs := c.Validate(); len(errs) != 0 {
		t.Fatalf("expected legacy contract to validate, got %v", errs)
	}
	if len(c.Requirements) != 0 {
		t.Fatalf("legacy contract should not declare requirements, got %+v", c.Requirements)
	}
	if len(c.Criteria) != 1 {
		t.Fatalf("expected 1 criterion, got %d", len(c.Criteria))
	}
	if c.Criteria[0].Threshold != 8 {
		t.Fatalf("expected threshold 8, got %d", c.Criteria[0].Threshold)
	}
	if c.Criteria[0].Evidence.Kind != "" {
		t.Fatalf("legacy criterion should have no evidence, got %+v", c.Criteria[0].Evidence)
	}
	if c.Criteria[0].RequirementID != "" {
		t.Fatalf("legacy criterion should have no requirement id, got %q", c.Criteria[0].RequirementID)
	}
}

func TestParseFullContractCapturesRequirementsAndEvidence(t *testing.T) {
	path := writeTempContract(t, `# Sprint 002 — traceability demo

## Goal
Exercise the new spec-driven contract format end to end.

## Requirements
- REQ-001: Feature rejects invalid input.
- REQ-002: Feature handles concurrent requests safely.

## Deliverables
- `+"`src/feature.ts`"+` exports: `+"`compute`"+` (REQ-001)
- `+"`tests/e2e/feature.spec.ts`"+` (REQ-002)

## Acceptance Criteria
| # | REQ     | Criterion                                | Evidence                            | Threshold |
|---|---------|------------------------------------------|-------------------------------------|-----------|
| 1 | REQ-001 | Invalid input returns 400                | tests:handles invalid input         | 9/10      |
| 2 | REQ-002 | Concurrent calls are serialised          | e2e:tests/e2e/feature.spec.ts       | 7/10      |
| 3 | REQ-001 | Approved fixture verifies the 400 body   | fixture:invalid-input-400           | 8/10      |
`)
	c, err := Parse(path)
	if err != nil {
		t.Fatalf("parse full contract: %v", err)
	}
	if errs := c.Validate(); len(errs) != 0 {
		t.Fatalf("expected full contract to validate, got %v", errs)
	}
	if len(c.Requirements) != 2 {
		t.Fatalf("expected 2 requirements, got %d", len(c.Requirements))
	}
	if c.Requirements[0].ID != "REQ-001" || c.Requirements[1].ID != "REQ-002" {
		t.Fatalf("unexpected requirement ordering: %+v", c.Requirements)
	}
	if len(c.Deliverables) != 2 {
		t.Fatalf("expected 2 deliverables, got %d", len(c.Deliverables))
	}
	if c.Deliverables[0].RequirementID != "REQ-001" {
		t.Fatalf("expected deliverable 0 linked to REQ-001, got %q", c.Deliverables[0].RequirementID)
	}
	if c.Deliverables[1].RequirementID != "REQ-002" {
		t.Fatalf("expected deliverable 1 linked to REQ-002, got %q", c.Deliverables[1].RequirementID)
	}
	if len(c.Criteria) != 3 {
		t.Fatalf("expected 3 criteria, got %d", len(c.Criteria))
	}
	if got := c.Criteria[0]; got.RequirementID != "REQ-001" || got.Evidence.Kind != "tests" || got.Evidence.Ref != "handles invalid input" {
		t.Fatalf("criterion 0 unexpected: %+v", got)
	}
	if got := c.Criteria[1]; got.Evidence.Kind != "e2e" || got.Evidence.Ref != "tests/e2e/feature.spec.ts" {
		t.Fatalf("criterion 1 unexpected: %+v", got)
	}
	if got := c.Criteria[2]; got.Evidence.Kind != "fixture" || got.Evidence.Ref != "invalid-input-400" {
		t.Fatalf("criterion 2 unexpected: %+v", got)
	}
}

func TestValidateRejectsUnknownRequirementReference(t *testing.T) {
	path := writeTempContract(t, `# Sprint 003 — broken traceability

## Goal
Validation must reject contracts that reference an undeclared requirement.

## Requirements
- REQ-001: Only declared requirement.

## Deliverables
- `+"`src/index.ts`"+` exports: `+"`run`"+` (REQ-001)

## Acceptance Criteria
| # | REQ     | Criterion                          | Evidence            | Threshold |
|---|---------|------------------------------------|---------------------|-----------|
| 1 | REQ-999 | Refers to an undeclared requirement | tests:nope        | 6/10      |
`)
	c, err := Parse(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	errs := c.Validate()
	if len(errs) == 0 {
		t.Fatal("expected validation to fail on unknown REQ-999")
	}
	found := false
	for _, e := range errs {
		if e == "criterion #1 references undefined REQ-999" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected REQ-999 error message, got %v", errs)
	}
}

func TestValidateFlagsOrphanRequirement(t *testing.T) {
	path := writeTempContract(t, `# Sprint 004 — orphan requirement

## Goal
Every declared requirement must be referenced by at least one deliverable or criterion.

## Requirements
- REQ-001: Used requirement.
- REQ-002: Orphan requirement.

## Deliverables
- `+"`src/index.ts`"+` exports: `+"`run`"+` (REQ-001)

## Acceptance Criteria
| # | REQ     | Criterion              | Evidence            | Threshold |
|---|---------|------------------------|---------------------|-----------|
| 1 | REQ-001 | Used requirement check | tests:run-handles   | 6/10      |
`)
	c, err := Parse(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	errs := c.Validate()
	if len(errs) == 0 {
		t.Fatal("expected validation to flag orphan REQ-002")
	}
	found := false
	for _, e := range errs {
		if e == "requirement REQ-002 is declared but not referenced by any deliverable or criterion" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected orphan REQ-002 error, got %v", errs)
	}
}

func TestCheckAgainstDiffVerifiesCriterionEvidence(t *testing.T) {
	repo := t.TempDir()
	mkfile(t, filepath.Join(repo, "src", "feature.ts"),
		"export function compute() { return 'ok'; }\n")
	mkfile(t, filepath.Join(repo, "src", "feature.test.ts"),
		"it('handles invalid input', () => { /* ... */ });\n")
	mkfile(t, filepath.Join(repo, "tests", "e2e", "feature.spec.ts"),
		"// playwright spec\n")
	mkfile(t, filepath.Join(repo, ".harness", "fixtures", "invalid-input-400.json"),
		"{}\n")

	contractPath := filepath.Join(repo, ".harness", "contracts", "sprint-001.md")
	mkfile(t, contractPath, `# Sprint 001 — evidence verification

## Goal
Ensure the new criterion-evidence check finds tests, e2e, and fixtures.

## Requirements
- REQ-001: Feature rejects invalid input.

## Deliverables
- `+"`src/feature.ts`"+` exports: `+"`compute`"+` (REQ-001)

## Acceptance Criteria
| # | REQ     | Criterion                            | Evidence                            | Threshold |
|---|---------|--------------------------------------|-------------------------------------|-----------|
| 1 | REQ-001 | Tests cover invalid input            | tests:handles invalid input         | 8/10      |
| 2 | REQ-001 | E2E spec exists for feature flow     | e2e:tests/e2e/feature.spec.ts       | 7/10      |
| 3 | REQ-001 | Approved fixture proves 400 response | fixture:invalid-input-400           | 8/10      |
`)
	c, err := Parse(contractPath)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	res := c.CheckAgainstDiff(repo)
	if res.Status != "satisfied" {
		t.Fatalf("expected satisfied, got %+v", res)
	}
	if res.Score != 100 {
		t.Fatalf("expected score 100, got %d", res.Score)
	}
	if len(res.UnmetCriteria) != 0 {
		t.Fatalf("expected no unmet criteria, got %v", res.UnmetCriteria)
	}
}

func TestCheckAgainstDiffReportsMissingEvidence(t *testing.T) {
	repo := t.TempDir()
	mkfile(t, filepath.Join(repo, "src", "feature.ts"),
		"export function compute() { return 'ok'; }\n")
	// Note: no test file, no e2e spec, no fixture.

	contractPath := filepath.Join(repo, ".harness", "contracts", "sprint-001.md")
	mkfile(t, contractPath, `# Sprint 001 — missing evidence

## Goal
The contract dimension must fail when declared evidence is absent.

## Requirements
- REQ-001: Feature rejects invalid input.

## Deliverables
- `+"`src/feature.ts`"+` exports: `+"`compute`"+` (REQ-001)

## Acceptance Criteria
| # | REQ     | Criterion                            | Evidence                            | Threshold |
|---|---------|--------------------------------------|-------------------------------------|-----------|
| 1 | REQ-001 | Tests cover invalid input            | tests:handles invalid input         | 8/10      |
| 2 | REQ-001 | E2E spec exists for feature flow     | e2e:tests/e2e/feature.spec.ts       | 7/10      |
| 3 | REQ-001 | Approved fixture proves 400 response | fixture:invalid-input-400           | 8/10      |
`)
	c, err := Parse(contractPath)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	res := c.CheckAgainstDiff(repo)
	if res.Status == "satisfied" {
		t.Fatalf("expected partial/violated, got satisfied (%+v)", res)
	}
	if len(res.UnmetCriteria) != 3 {
		t.Fatalf("expected 3 unmet criteria, got %d (%v)", len(res.UnmetCriteria), res.UnmetCriteria)
	}
	expected := map[string]bool{
		"REQ-001 (criterion #1) evidence `tests:handles invalid input` not found in any test file":       false,
		"REQ-001 (criterion #2) e2e spec `tests/e2e/feature.spec.ts` does not exist":                     false,
		"REQ-001 (criterion #3) approved fixture `invalid-input-400` is missing from .harness/fixtures/": false,
	}
	for _, msg := range res.UnmetCriteria {
		if _, ok := expected[msg]; ok {
			expected[msg] = true
		}
	}
	for msg, found := range expected {
		if !found {
			t.Fatalf("expected unmet criterion message %q, got %v", msg, res.UnmetCriteria)
		}
	}
}

func TestCheckAgainstDiffSkipsLegacyCriteriaWithoutEvidence(t *testing.T) {
	repo := t.TempDir()
	mkfile(t, filepath.Join(repo, "src", "index.ts"),
		"export function run() { return 'ok'; }\n")

	contractPath := filepath.Join(repo, ".harness", "contracts", "sprint-001.md")
	mkfile(t, contractPath, `# Sprint 001 — legacy contract still scores 100

## Goal
Legacy contracts with no evidence must keep their pre-existing score.

## Deliverables
- `+"`src/index.ts`"+` exports: `+"`run`"+`

## Acceptance Criteria
| # | Criterion                       | Threshold |
|---|---------------------------------|-----------|
| 1 | Index module exports run helper | 8/10      |
`)
	c, err := Parse(contractPath)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	res := c.CheckAgainstDiff(repo)
	if res.Status != "satisfied" {
		t.Fatalf("expected legacy contract to remain satisfied, got %+v", res)
	}
	if res.Score != 100 {
		t.Fatalf("expected legacy score to stay at 100, got %d", res.Score)
	}
}

func TestParseSizeFromHashSection(t *testing.T) {
	cases := map[string]Size{
		"small":  SizeSmall,
		"medium": SizeMedium,
		"large":  SizeLarge,
	}
	for keyword, want := range cases {
		t.Run(keyword, func(t *testing.T) {
			path := writeTempContract(t, `# Sprint 005 — size declared

## Goal
Declare an explicit size in the contract.

## Size
`+keyword+`

## Deliverables
- `+"`x.ts`"+`

## Acceptance Criteria
| # | Criterion | Threshold |
|---|-----------|-----------|
| 1 | works     | 8/10      |
`)
			c, err := Parse(path)
			if err != nil {
				t.Fatal(err)
			}
			if c.Size != want {
				t.Fatalf("expected Size=%q, got %q", want, c.Size)
			}
			if c.EffectiveSize() != want {
				t.Fatalf("EffectiveSize mismatch: %q", c.EffectiveSize())
			}
		})
	}
}

func TestEffectiveSizeFallsBackToEmptyWhenUndeclared(t *testing.T) {
	path := writeTempContract(t, `# Sprint 006 — no size declared

## Goal
Skip the Size section to verify legacy behavior.

## Deliverables
- `+"`x.ts`"+`

## Acceptance Criteria
| # | Criterion | Threshold |
|---|-----------|-----------|
| 1 | works     | 8/10      |
`)
	c, err := Parse(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Size != "" {
		t.Fatalf("expected Size to be empty, got %q", c.Size)
	}
	if c.EffectiveSize() != "" {
		t.Fatalf("EffectiveSize should also be empty so legacy contracts skip size policy, got %q", c.EffectiveSize())
	}
}

func TestTemplateProducesValidContract(t *testing.T) {
	path := writeTempContract(t, Template(7, "ship feature x"))
	c, err := Parse(path)
	if err != nil {
		t.Fatalf("parse template: %v", err)
	}
	if errs := c.Validate(); len(errs) != 0 {
		t.Fatalf("expected fresh template to validate, got %v", errs)
	}
	if c.SprintNumber != 7 {
		t.Fatalf("expected sprint number 7, got %d", c.SprintNumber)
	}
	if c.Title != "ship feature x" {
		t.Fatalf("expected title 'ship feature x', got %q", c.Title)
	}
	if len(c.Requirements) != 2 {
		t.Fatalf("expected 2 requirements from template, got %d", len(c.Requirements))
	}
	if len(c.Deliverables) != 2 || c.Deliverables[0].RequirementID != "REQ-001" {
		t.Fatalf("template deliverables should reference REQ-001, got %+v", c.Deliverables)
	}
	if len(c.Criteria) != 3 {
		t.Fatalf("expected 3 criteria rows, got %d", len(c.Criteria))
	}
	for i, cr := range c.Criteria {
		if cr.Evidence.Kind == "" {
			t.Fatalf("template criterion %d should declare evidence, got %+v", i, cr.Evidence)
		}
	}
}

func mkfile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
