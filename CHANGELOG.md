# Changelog

All notable changes to this project are documented here. Format loosely
follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the
project follows [Semantic Versioning](https://semver.org/) once
production-ready.

## [0.9.0] - Unreleased

The v0.6 → v0.9 train closes every P0 and P1 item in
`docs/IMPROVEMENT_PLAN.md`. The package version jumps from 0.5.5
directly to 0.9.0 because the eleven internal milestones below shipped
together and are validated by the same test suite.

### Added (v0.9 — Optional Inferential Reviewer)

- New `review` dimension wired across `internal/sensors`,
  `internal/config`, `internal/evaluator`, and `internal/adapters`.
  Disabled by default (threshold and weight = 0).
- `internal/adapters/external_reviewer.go` shells out to a configured
  reviewer CLI (Claude Code, Codex, custom Python script, etc.). I/O
  contract documented in `docs/INFERENTIAL_REVIEWER.md`.
- Reviewer suggestions populate the new `Finding.Hint` field so they
  surface in the repair brief under `## Suggested Fixes`.
- `--fast` and `harness watch` automatically exclude the review
  dimension; the LLM cost only happens during full `harness sprint qa`.
- `harness doctor` reports whether the reviewer is configured,
  partially configured, or active-without-command (FAIL).

### Added (v0.8.7 — Positive Prompt Injection)

- `internal/sensors.LLMHint(rule)` catalog of "Suggested fix / Do NOT"
  pairs for 30+ common rules (lint, complexity, e2e, contract,
  security).
- `sensors.Finding.Hint` field, omitted from compact TTY output and
  rendered in JSON reports and the deduplicated
  `## Suggested Fixes (LLM-optimized)` repair-brief section.

### Added (v0.8.5 — Adaptive Sprints + Context Budget)

- `planner.Contract.Size` field parsed from a new `## Size` section
  (`small | medium | large`). Empty Size preserves legacy behavior.
- `planning.RequiresTasks` / `RequiresDesign` predicates plus
  `ContractPolicyErrorsWith(mode, contract, presence)` so size and
  mode rules compose. Medium sprints must ship a tasks plan; large
  sprints additionally require a design doc.
- New `internal/budget` package and `harness context size [--format]`
  command estimate the agent-context cost of the harness memory
  bundle.
- `harness doctor` warns when the bundle crosses the 40k-token soft
  limit.

### Added (v0.8 — Continuous Drift Watch)

- `internal/sensors.IsAudit(name)` classifier covers `npm-audit`,
  `pip-audit`, `cargo-audit`, `govulncheck`.
- `evaluator.Options.IncludeAudits` and `Options.SkipContract` enable
  watch-style runs without a sprint contract.
- New `internal/watch` package: `RunOnce` writes
  `.harness/watch/<timestamp>.json` plus a `latest.json` pointer and
  computes delta versus the previous run.
- `harness watch once --fail-on-regression` propagates non-zero exit
  for CI cron schedules. `harness watch list` enumerates past reports.
- `docs/templates/harness-watch.yml.example` ships a ready-to-copy
  GitHub Actions schedule (6-hour cron).
- `harness doctor` surfaces whether drift watch has run.

### Added (v0.7 — Shift-Left)

- `internal/sensors.IsFast(name)` classifier covers eslint, ruff,
  mypy, go-vet, staticcheck, clippy, js-complexity, js-architecture.
- `evaluator.Options.Fast` filters sensors to fast-only and marks
  dimensions without a fast sensor as `Skipped` (excluded from
  verdict and weighted score).
- `harness sprint qa --fast` for shift-left informational checks,
  skipping the agreement gate and never overwriting full QA report
  artifacts.
- `harness install-hooks --pre-commit` writes a blocking git
  pre-commit hook that runs `harness sprint qa --fast`. Standard
  `git commit --no-verify` bypass still works.
- `harness doctor` reports pre-push and pre-commit hook status.

### Added (v0.6 — Verifiable Acceptance + Trusted Reports)

#### Verifiable acceptance and REQ-IDs (P0-1 + P0-2)

- 5-column acceptance table (`# | REQ | Criterion | Evidence |
  Threshold`) parsed alongside the legacy 3-column form.
- New `## Requirements` section in contracts; REQ-IDs cross-checked
  between deliverables and criteria (`Validate` rejects undefined
  references and orphan requirements).
- `Contract.CheckAgainstDiff` mechanically verifies declared evidence:
  `tests:<substring>` (searched across test files), `e2e:<path>`
  (file must exist), `fixture:<name>` (file in `.harness/fixtures/`).
  Legacy criteria without evidence preserve their pre-existing score.
- `harness sprint new` template uses the new format by default.

#### Contract hash covers design and tasks (P0-3)

- The agreement hash now includes `.harness/design/sprint-NNN.md` and
  `.harness/tasks/sprint-NNN.md` when present. Silent edits to those
  files invalidate an AGREED state.
- `Status.Hashed` lists which artifacts participated; `harness
  contract status` prints the list.

#### Reports pinned to workspace SHA (P0-4)

- New `internal/workspace` package computes a deterministic content
  hash of the working tree (ignoring `.git`, `.harness`,
  `node_modules`, build dirs).
- `EvaluationResult.Process.WorkspaceSHA` records the hash at QA time.
- `harness sprint score` refuses to consolidate when the current
  workspace SHA differs from the report's, blocking the silent
  "edit-after-PASS" bypass.

#### Planning policy actually gates QA (P0-5)

- New `internal/planning` package reads `.harness/setup.json` and
  enforces structural policies per mode.
- Spec-driven mode requires `## Requirements`, REQ-IDs on every
  criterion, and mechanical Evidence (`tests:` / `e2e:` / `fixture:`).
- Policy errors surface in `harness contract status` and block
  `harness contract propose` so the `--planning` flag is no longer
  cosmetic.

### Changed

- `package.json` version jumps from 0.5.5 to 0.9.0.
- Default contract template generated by `harness sprint new` uses
  neutral examples (`feature.spec.ts`, `invalid-input-400` fixture).
  No domain-specific references.
- README documents `--fast`, `--pre-commit`, `harness watch`, `harness
  context size`, and the optional `review` dimension.
- ROADMAP.md sections renumber Distribution Hardening to v0.9.1.
- `sprint.Manager` now exposes `Close()` and migrates the memory
  database automatically. Tests and long-running services should call
  Close to avoid leaking the SQLite file handle on Windows.

### Backwards compatibility

- Legacy contracts (3-column acceptance table, no `## Requirements`,
  no `## Size`) continue to parse and validate.
- Contract hash for sprints without `design/` and `tasks/` files
  matches v0.5.x exactly. Sprints that add those artifacts will see
  their hash invalidate once, requiring a re-propose. Documented in
  `MIGRATION.md`.
- The `review` dimension and external reviewer adapter are opt-in.
  Existing `.harness/config.yaml` files behave unchanged.
- The legacy `--skills on|off` flag remains supported as an alias for
  `--planning spec-driven|manual`.

## [0.5.5] and earlier

See `ROADMAP.md` for the v0.1 → v0.5 history (walking skeleton,
isolated evaluator, Node/TS production sensors, multi-agent agreement,
broader stack coverage, terminal UI refresh, doctor auto-fix).
