# Architecture

Harness is a deterministic local control system for AI-assisted coding. It is
not an LLM reviewer. It supplies feedforward instructions, computational
sensors, isolated evaluation, and project memory so the coding model can be
steered by evidence instead of claims.

## Concept Mapping

### One-Shot Hero

Large unbounded tasks are contained by sprint contracts. A sprint has a small
goal, declared deliverables, acceptance criteria, and constraints under
`.harness/contracts/sprint-NNN.md`.

Code: `internal/sprint/sprint.go`, `internal/planner/contract.go`.

The Harness-native spec-driven skill pack keeps this boundary while adding
adaptive planning depth. The external TLC-style phases map into Harness
artifacts instead of creating a second `.specs/` source of truth:

- Specify -> `.harness/contracts/sprint-NNN.md`
- Design -> `.harness/design/sprint-NNN.md` when architectural decisions exist
- Tasks -> `.harness/tasks/sprint-NNN.md` when the sprint needs explicit atomic work items
- Execute/Validate -> `harness feature qa`, `harness feature repair`, and `harness feature score`

Plan: `docs/SPEC_DRIVEN_SKILL_PACK.md`.

### Divergent Agents

Contracts move through a deterministic agreement gate before implementation.
`harness contract propose` records a stable hash for the current contract.
Required roles, currently `planner` and `tester`, must approve that exact hash
with `harness contract approve --role ...`. If the contract changes after
approval, the hash changes and the state becomes `changed`; QA blocks until the
new hash is proposed and approved again.

Code: `internal/agreement/agreement.go`, `cmd/harness/contract.go`.

Provider integrations reinforce the same boundary. Codex installations get
`.codex/hooks.json` plus `harness_contract_author` and
`harness_contract_reviewer` custom agents; the hook denies product-file
`apply_patch/Edit/Write` calls before agreement. Claude Code installations get
equivalent `.claude/agents/` files and an edit guard in `.claude/settings.json`.
Cursor receives repository rules because it does not expose the same
project-local pre-tool hook surface.

### Premature Victory

`harness feature qa` runs configured sensors in an isolated evaluator process.
By default it first checks that the current sprint contract is `agreed`.
Config v2 is strict: a dimension is active only when both threshold and weight
are greater than zero, and every active dimension must have at least one real
configured sensor execute. Missing sensors become `missing-sensor` findings,
score `0`, and force `FAIL`.

When QA fails, the sprint is not complete. Harness writes
`.harness/repairs/latest.md` with the failed dimensions, findings, and required
next action. Agents must repair, rerun QA, and repeat until the verdict is
`PASS`. `harness feature score` refuses to consolidate `FAIL` unless the user
explicitly passes `--allow-fail` for an abandoned-sprint audit record.

Code: `internal/evaluator/evaluator.go`, `internal/config/config.go`.

### Session Amnesia

Harness keeps two stores:

- `.harness/progress.md`: narrative memory, versioned with the repo.
- `.harness/memory.db`: local SQLite index for runs, findings, recurrence, and
  trends.

Code: `internal/memory/memory.go`, `internal/reporter/reporter.go`.

### Fake Tests

The E2E dimension uses Playwright browser tests. Screenshot attachments are
copied into `.harness/screenshots/current`, compared against
`.harness/screenshots/baseline`, and visual differences become E2E findings.
New baselines require `harness feature qa --accept-screenshots` after review.

Code: `internal/adapters/playwright.go`.

Approved behavior fixtures cover deterministic input/output scenarios that do
not need a browser. JSON fixtures under `.harness/fixtures/` run a configured
command, compare exit code/stdout/stderr against approved expectations, and
fail with `fixture-baseline-missing` or `fixture-regression` when the behavior
has not been approved. Updating the approved output requires
`harness feature qa --accept-fixtures` after human review.

Code: `internal/adapters/approved_fixtures.go`.

### Same-Process Judgement

The parent CLI process re-executes itself with hidden `--internal`; that child
is the evaluator. The child has closed stdin, JSON-only stdout, stderr for
diagnostics, and an allowlisted environment. Builder context variables from
Claude Code, Codex, Cursor, or similar tools are stripped.

The JSON report includes safe process metadata so tests can verify distinct
PIDs and env stripping without exposing sensitive values.

Code: `internal/sprint/sprint.go`, `internal/evaluator/evaluator.go`.

### Accumulated Slop

Findings receive stable fingerprints. `harness trend` shows score history and
`harness explain <finding-id>` shows recurrence metadata. This makes repeated
issues visible across sprints.

Code: `internal/sensors/fingerprint.go`, `internal/memory/memory.go`.

## Node/TypeScript Sensors

The production Node/TypeScript profile includes:

- `eslint` for correctness
- `jest` for test correctness
- `jest-coverage` for coverage
- `npm-audit` for security
- `js-complexity` for cyclomatic complexity, function size, and nesting
- `js-architecture` for forbidden imports and import cycles
- `approved-fixtures` for optional approved behavior fixtures
- `playwright` for E2E and screenshot baseline checks

Run `harness doctor` to inspect active dimensions, registered sensors, and
missing local tooling. Run `harness doctor --strict` in CI or release checks to
fail when a dimension lacks an available real sensor, config is ambiguous, or
generated agent references/skills are stale.

## Deterministic Boundary

Harness deliberately avoids LLM calls, API keys, and cloud review. An optional
inferential reviewer can be a separate tool later, but the production harness
remains reproducible: same code plus same local tools yields the same report.
