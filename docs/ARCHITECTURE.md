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

### Premature Victory

`harness sprint qa` runs configured sensors in an isolated evaluator process.
Config v2 is strict: a dimension is active only when both threshold and weight
are greater than zero, and every active dimension must have at least one real
configured sensor execute. Missing sensors become `missing-sensor` findings,
score `0`, and force `FAIL`.

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
New baselines require `harness sprint qa --accept-screenshots` after review.

Code: `internal/adapters/playwright.go`.

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
- `playwright` for E2E and screenshot baseline checks

Run `harness doctor` to inspect active dimensions, registered sensors, and
missing local tooling.

## Deterministic Boundary

Harness deliberately avoids LLM calls, API keys, and cloud review. An optional
inferential reviewer can be a separate tool later, but the production harness
remains reproducible: same code plus same local tools yields the same report.
