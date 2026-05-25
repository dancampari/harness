# Harness Improvement Plan

## Status

- v0.6 shipped: P0-1, P0-2, P0-3, P0-4, P0-5. See `ROADMAP.md` for the
  release notes and `internal/{planner,agreement,workspace,planning}` for
  the implementation. Backwards compatibility preserved.
- v0.7 shipped: P1-1 shift-left with `--fast` and pre-commit. See
  `internal/sensors.IsFast`, `evaluator.Options.Fast`,
  `cmd/harness/install_hooks.go` (`--pre-commit`).
- v0.8 shipped: P1-3 drift watch. See `internal/sensors.IsAudit`,
  `internal/watch`, `cmd/harness/watch.go`, and the GitHub Actions
  template in `docs/templates/harness-watch.yml.example`.
- v0.8.5 shipped: P1-4 sprint size policy and P1-5 context budget. See
  `planner.Contract.Size`, `planning.ContractPolicyErrorsWith`,
  `internal/budget`, and `cmd/harness/context.go`.
- v0.8.7 shipped: P1-6 positive prompt injection. See
  `internal/sensors.LLMHint`, `sensors.EnrichFinding`, and the
  `## Suggested Fixes` section in repair briefs.
- v0.9 shipped: P1-2 optional inferential reviewer. See
  `internal/adapters/external_reviewer.go`, the new `review` dimension
  in `internal/config` and `internal/evaluator`, and
  `docs/INFERENTIAL_REVIEWER.md` for the full I/O contract.
- **All P0 and P1 items closed.** v0.9+ work is the P2 polish backlog
  in this document.

## Closed by the TLC unification (v0.10)

The six-phase plan in `docs/UNIFICATION_PLAN.md` retired the following
items from the P2 backlog below. Cite this section instead of reopening
them:

- **TLC-as-foundation rewrite.** `internal/skills/tlc/tlc-spec-driven/`
  vendors the canonical TLC skill verbatim (~3,400 lines, 18 files);
  `internal/skills/gate/harness-gate/SKILL.md` documents only the
  deterministic gates. The 430-line `cmd/harness/spec_driven_skills.go`
  reshape was deleted.
- **`.specs/` artifact tree.** `.specs/project/{PROJECT,STATE,ROADMAP,
  HANDOFF}.md`, `.specs/codebase/*.md`, `.specs/features/<slug>/*.md`,
  `.specs/quick/NNN-slug/*` are now the source of truth.
  `agreement.Manager` dual-reads (`.specs/` canonical → `.harness/`
  legacy); `harness upgrade` migrates losslessly. See
  `cmd/harness/specs_migrate.go` and its test.
- **TLC spec.md structural validator.** Spec-driven mode rejects propose
  unless every criterion is in `WHEN/THEN/SHALL` form and the spec has
  `## Edge Cases` and `## Out of Scope` sections. See
  `internal/planning/policy.go::tlcSpecPatternErrors`.
- **TLC tasks.md granularity gate.** Tasks touching >3 files without
  `Cohesive: true` are rejected at propose. See
  `internal/planner/tasks.go` + `internal/planning/policy.go::
  tlcTaskGranularityErrors`.
- **SPEC_DEVIATION scanner sensor.** New deterministic sensor flags
  `SPEC_DEVIATION` markers missing a `Reason:` annotation within five
  lines. Contract dimension. See `internal/adapters/spec_deviation.go`.
- **Scope-creep sensor.** New deterministic sensor compares `git diff
  --name-only HEAD` against the union of every task's `Where:` paths
  and flags files modified outside the declared scope. Contract
  dimension. See `internal/adapters/scope_creep.go`.
- **Blocking TLC contract sensors.** `SPEC_DEVIATION` without `Reason:`,
  scope creep, TDD violations, and test-count regressions now hard-fail
  the contract gate instead of being diluted by score averages. See
  `internal/evaluator/evaluator.go` and the sensor tests.
- **TLC commands.** `harness feature {new,status,qa,repair,score,list,
  propose,approve,reject}`, `harness quick`, `harness roadmap`,
  `harness state record`, `harness session pause | resume`. See
  `cmd/harness/{feature,quick,roadmap,state,session}.go`.
- **Multi-agent delegation matrix.** Generated `AGENTS.md` / `CLAUDE.md`
  embed TLC's sub-agent delegation matrix and the Knowledge Verification
  Chain. New `harness-researcher` persona for Codex + Claude. Canonical
  spec at `docs/MULTI_AGENT_PROTOCOL.md`.
- **Skill-integrations detection.** `harness setup` / `install-hooks`
  detect `.claude/skills/mermaid-studio` and `codenavi`; the generated
  agent doc lists detected runtimes so the orchestrator prefers them.
  See `cmd/harness/skill_integrations.go`.
- **`harness sprint` deprecation.** Legacy `harness sprint <verb>` prints
  a one-line deprecation warning and emits `cli.deprecated`. Slated for
  removal in v2.0.
- **Vendored TLC drift gate.** `harness doctor [--strict]` hashes the
  embedded skill packs against the on-disk copy and fails when they
  differ. See `internal/skills/skills.go::VendoredHash` /
  `InstalledHash`.

Remaining open items below are P2 polish / quality-of-life work not
related to the TLC unification.

Forward-looking plan derived from a gap analysis against three canonical
references on harness engineering:

- Martin Fowler, "Harness Engineering"
  (https://martinfowler.com/articles/harness-engineering.html)
- TLC Spec-Driven skill
  (https://agent-skills.techleads.club/skills/tlc-spec-driven/ and the
  upstream `SKILL.md` in `tech-leads-club/agent-skills`)
- Video transcript on harness engineering provided in the planning conversation

`ROADMAP.md` records what has shipped. This document records what should
ship next and why. It is intentionally implementation-detailed so an agent or
a maintainer can pick an item up without rereading the entire conversation.

## How To Read This Plan

Each item has:

- **Status** — `bug` (CLI returns wrong answer or silently misleads),
  `gap` (canonical concept not implemented), or `enhancement` (quality
  improvement).
- **Why it matters** — concrete user-visible effect.
- **Files touched** — actual code paths.
- **Acceptance** — testable outcomes that close the item.
- **Depends on** — items that should land first.

Priority bands:

- **P0** — directly affects CLI correctness or the agreement gate being
  meaningful. Ship before claiming production readiness.
- **P1** — large coverage gap vs the canonical harness concept. Required
  to honor the framework's stated mission.
- **P2** — quality, UX, or follow-up work.

---

## P0 — Items that compromise CLI correctness today

### P0-1. Acceptance criteria are parsed but never verified

**Status**: bug

**Evidence**:

- `internal/planner/contract.go:120-130` parses
  `## Acceptance Criteria` rows into `Contract.Criteria`.
- `internal/planner/contract.go:181-239` `CheckAgainstDiff` iterates only
  `c.Deliverables`. `c.Criteria` is never read after parsing.
- `internal/evaluator/evaluator.go:77-82` declares `UnmetCriteria []string`
  but the only writers are missing exports (`contract.go:206`, `:218`).

**Effect**: a sprint can return `PASS` with every acceptance criterion
unaddressed as long as the declared files exist and export the listed
symbols. The "Acceptance Criteria" table is decorative.

**Fix**:

1. Extend the contract grammar to require evidence per criterion. New
   row format:

   ```markdown
   | # | REQ | Criterion | Evidence | Threshold |
   |---|-----|-----------|----------|-----------|
   | 1 | REQ-001 | Invalid input returns 400 | tests:Feature#rejects_invalid | 8/10 |
   | 2 | REQ-002 | Concurrent calls are serialised | e2e:tests/e2e/feature.spec.ts | 7/10 |
   | 3 | REQ-003 | No 5xx under contention | fixture:concurrent-no-5xx.json | 9/10 |
   ```

2. `AcceptanceCriterion` grows: `RequirementID string`, `Evidence Evidence`
   where `Evidence` is a tagged union `{Kind: tests|e2e|fixture|inspection, Ref: string}`.

3. New sensor `criteria-coverage` in `internal/adapters/criteria_coverage.go`
   that fails when a criterion has no matching:
   - test name in JUnit/Jest/Vitest/pytest output for `tests:` evidence,
   - Playwright spec for `e2e:` evidence,
   - approved fixture file existing and passing for `fixture:` evidence.

4. `Contract.Validate()` rejects criteria without a `Evidence` value or
   with malformed REQ-ID.

5. `ContractCheckResult.UnmetCriteria` is populated by the new sensor
   referencing REQ-IDs.

**Files touched**:

- `internal/planner/contract.go`
- `internal/adapters/criteria_coverage.go` (new)
- `internal/adapters/registry.go`
- `internal/evaluator/evaluator.go`
- `internal/sprint/sprint.go` (repair brief rule mapping)
- `cmd/harness/spec_driven_skills.go` (skill text for the new evidence
  field)
- contract template in `internal/planner/contract.go:265-287`

**Acceptance**:

- Parsing a contract with criteria but no Evidence column returns a
  structural error.
- A sprint with `REQ-001 → tests:foo#bar` and no test named `foo#bar`
  fails QA with `unmet-criterion` finding for `REQ-001`.
- A sprint where every REQ has matching evidence that actually executed
  and passed scores 100 in the `contract` dimension.

**Depends on**: P0-2 (REQ-ID parsing) lands together.

---

### P0-2. REQ-IDs are documented in skills but not enforced

**Status**: bug (skills lie to agents)

**Evidence**:

- `cmd/harness/spec_driven_skills.go:135` skill says "Use requirement IDs
  such as REQ-001".
- `cmd/harness/spec_driven_skills.go:219` tasks skill says "Each task
  maps to one or more requirement IDs".
- Parser does not extract REQ-IDs from anywhere
  (`internal/planner/contract.go`).
- `cmd/harness/doctor.go:478-507` strict mode only checks agent file
  presence, not contract/task REQ structure.

**Effect**: agents are told to use REQ-IDs and then the harness accepts
contracts without them. Doctor `--strict` claims spec-driven enforcement
without actually enforcing REQ traceability.

**Fix**:

1. New parser section in `internal/planner/contract.go` for
   `## Requirements`:

   ```markdown
   ## Requirements
   - REQ-001: Feature rejects invalid input
   - REQ-002: Feature handles concurrent requests safely
   ```

2. `Contract.Requirements []Requirement` with `{ID string, Statement string}`.

3. `Contract.Validate()` rejects:
   - any `Deliverable`, `Criterion`, or task row with a REQ-ID not
     declared in `## Requirements`,
   - any declared REQ-ID with zero criteria.

4. `internal/planner/tasks.go` (new) parses
   `.harness/tasks/sprint-NNN.md` task rows and validates REQ-ID
   coverage.

5. `cmd/harness/doctor.go` strict mode for `PlanningSpecDriven` checks:
   - active contract declares `## Requirements`,
   - every Deliverable and Criterion line references a declared REQ,
   - if `.harness/tasks/sprint-NNN.md` exists, every task references a
     declared REQ.

**Files touched**:

- `internal/planner/contract.go`
- `internal/planner/tasks.go` (new)
- `cmd/harness/doctor.go`
- `cmd/harness/spec_driven_skills.go` (template gets `## Requirements`)
- `internal/planner/contract.go:265-287` (contract template)

**Acceptance**:

- `harness sprint new "<goal>"` produces a template with a
  `## Requirements` section and Evidence column.
- `harness contract propose` rejects a contract with criteria referencing
  undefined REQ-IDs.
- `harness doctor --strict --planning spec-driven` fails on a
  spec-driven repo whose latest contract lacks `## Requirements`.

**Depends on**: none. Should land with P0-1.

---

### P0-3. Contract hash does not cover design or tasks

**Status**: bug (silent agreement bypass)

**Evidence**:

- `internal/agreement/agreement.go:331-340`:

  ```go
  canonical := strings.TrimSpace(strings.ReplaceAll(contract.RawMarkdown, "\r\n", "\n")) + "\n"
  sum := sha256.Sum256([]byte(canonical))
  ```

  Only the contract markdown is hashed.
- `docs/SPEC_DRIVEN_SKILL_PACK.md` and `cmd/harness/spec_driven_skills.go`
  treat `.harness/design/sprint-NNN.md` and `.harness/tasks/sprint-NNN.md`
  as part of the agreed scope.

**Effect**: design or task plan can be rewritten after `AGREED` without
producing a `CHANGED` state. The tester/reviewer role approves a snapshot
that did not include those files. The PreToolUse guard cannot detect this
because edit guards key off the hash.

**Fix**:

1. `Manager.contractHash` becomes
   `Manager.sprintHash(sprintNumber int) (string, *SprintBundle, error)`.

2. `SprintBundle` aggregates:
   - `.harness/contracts/sprint-NNN.md` (raw)
   - `.harness/design/sprint-NNN.md` if present
   - `.harness/tasks/sprint-NNN.md` if present

3. Hash input is a canonical YAML envelope, not concatenated markdown, so
   reordering is deterministic and missing files are explicit:

   ```yaml
   contract_sha256: ...
   design_sha256: ... | null
   tasks_sha256: ... | null
   ```

4. `Lock` keeps the same `contract_hash` field but its semantics widen.
   Schema version bumps to `2`. Migrate older locks lazily by recomputing.

5. CLI `harness contract status` adds an `Includes` block:

   ```text
   Sprint 003 AGREED
     contract  hash: ab12...
     design    hash: 9f44...
     tasks     hash: not present
   ```

**Files touched**:

- `internal/agreement/agreement.go`
- `internal/sprint/sprint.go` (Status text)
- `cmd/harness/contract.go`
- `cmd/harness/doctor.go` (strict mode validates schema_version)

**Acceptance**:

- A repo with an `AGREED` sprint transitions to `CHANGED` after any of
  `contract`, `design`, or `tasks` files is edited.
- `harness contract status` shows which of the three files contributed
  to the active hash.
- Lock files written under the new schema parse, and old `schema_version=1`
  lock files trigger a one-shot recompute with a warning.

**Depends on**: none.

---

### P0-4. Reports are not pinned to the implementation snapshot

**Status**: bug (stale-report check is time-based, not content-based)

**Evidence**:

- `internal/agreement/agreement.go:196-201` `ReportIsCurrent`:

  ```go
  return strings.EqualFold(s.State, "agreed") &&
      !s.AgreedAt.IsZero() &&
      !reportTime.IsZero() &&
      !reportTime.Before(s.AgreedAt)
  ```

  Reports are considered current as long as their timestamp is at or
  after `AgreedAt`. There is no link from the report to the actual code
  state that produced it.

**Effect**: a developer can edit product files after a `PASS`, leave the
contract untouched, and `harness sprint score` still consolidates the old
report because the agreement gate did not change. The "stale report"
heuristic only catches contract churn, not implementation churn.

**Fix**:

1. Evaluator records `workspace_sha` in `EvaluationResult.Process`. The
   value is the SHA-256 of a deterministic tree hash of tracked, non-
   ignored files. Use the same canonical algorithm as the sprint hash for
   consistency.

2. `harness sprint score` recomputes the workspace hash and refuses to
   consolidate if it differs from `EvaluationResult.Process.WorkspaceSHA`.
   Emit a clear message: `workspace changed after QA — rerun harness sprint qa`.

3. The TUI shows both the contract hash and workspace hash on the run.

**Files touched**:

- `internal/evaluator/evaluator.go` (`ProcessInfo` gains `WorkspaceSHA`)
- `internal/sprint/sprint.go` (`Consolidate` enforces match)
- `internal/tui/*` (display)
- Adapters may need to declare which files they read so the workspace
  hash can include them; default is the tracked working set.

**Acceptance**:

- After `harness sprint qa` returns `PASS`, editing any tracked file and
  running `harness sprint score` fails with the workspace-changed message.
- Running `harness sprint qa` again refreshes the workspace hash, after
  which `score` succeeds.

**Depends on**: P0-3 (shared hashing helper).

---

### P0-5. `planning_mode` enforcement is cosmetic for QA

**Status**: bug (mode flag misleads users)

**Evidence**:

- `cmd/harness/install_hooks.go:680-745` switches text protocols by mode.
- `cmd/harness/doctor.go:478-507` checks for spec-planner/task-worker
  agent files only when `planningMode == PlanningSpecDriven`.
- `internal/sprint/sprint.go` and `internal/agreement/agreement.go` do
  not branch on planning mode anywhere.

**Effect**: choosing `--planning manual` or `--planning contract` does
not change QA behavior. The CLI suggests modes are meaningfully different
but only the generated agent docs differ.

**Fix**:

1. `setup.json` (`internal/config` or a new file) becomes the source of
   truth for active planning mode. Currently the truth lives in setup
   state and is inferred elsewhere.

2. New `internal/planning/policy.go` exposes:

   ```go
   func RequireRequirements(mode string) bool
   func RequireTasksForSprintsOver(mode string, atomic int) bool
   func RequireDesignForArchSprints(mode string) bool
   ```

3. `Contract.Validate()` consults `policy.RequireRequirements(mode)`.
4. `Tasks.Validate()` consults `policy.RequireTasksForSprintsOver(mode, 3)`.
5. `harness sprint qa` rejects sprints that violate the policy with a
   clear "spec-driven requires X" finding, instead of allowing the run
   and writing reports that look valid.

**Files touched**:

- `internal/planning/policy.go` (new)
- `internal/planner/contract.go`
- `internal/planner/tasks.go` (new, from P0-2)
- `internal/sprint/sprint.go`
- `cmd/harness/doctor.go` (already mode-aware, becomes thinner)

**Acceptance**:

- A `spec-driven` repo with a contract missing `## Requirements` cannot
  reach `AGREED`.
- A `manual` repo with the same contract does reach `AGREED` and runs QA
  without complaining about requirements.
- `harness doctor --strict` prints which policies are active in the
  current mode.

**Depends on**: P0-2.

---

## P1 — Items that close large gaps vs the canon

### P1-1. Shift-left: pre-commit hook with fast sensors

**Status**: gap

**Evidence**: only `pre-push` is installed
(`install_hooks.go:481-505`) and it is intentionally non-blocking. Claude
Code receives a `PostToolUse Bash(git commit*)` hook that runs QA after
the commit, not before. Codex and Cursor get nothing on commit. Humans
without an agent get nothing.

**Fix**:

1. New `internal/sensors/sensor.go` interface tag: `Fast bool` (sensor
   self-declares whether it runs in seconds, not minutes).

2. New CLI flag `harness sprint qa --fast` runs only fast sensors and
   contract structural validation.

3. `harness install-hooks --pre-commit` writes
   `.git/hooks/pre-commit` that runs `harness sprint qa --fast` and
   exits non-zero on `FAIL`. The hook respects `--no-verify` like any
   other.

4. `harness install-hooks` default keeps the existing non-blocking
   `pre-push` and adds the blocking pre-commit, unless
   `--no-precommit` is passed.

5. The Doctor reports both hooks under "Continuous controls".

**Files touched**:

- `internal/sensors/sensor.go` (Fast tag)
- adapters (annotate fast vs slow)
- `cmd/harness/sprint.go` (`--fast` flag)
- `cmd/harness/install_hooks.go` (new pre-commit writer)
- `cmd/harness/doctor.go` (report)

**Acceptance**:

- After `harness install-hooks --pre-commit`, `git commit` runs
  `harness sprint qa --fast` and aborts on lint or type errors.
- `harness sprint qa --fast` finishes under 10s on a typical TS repo
  (lint + contract structural + typecheck).

**Depends on**: none.

---

### P1-2. Inferential output reviewer as an optional adapter

**Status**: gap (Fowler treats inferential sensors as half the harness;
the video treats a second LLM as core)

**Evidence**: `README.md:12-14` explicitly states "Harness is intentionally
not an LLM reviewer". No adapter calls an LLM. The
`harness-contract-reviewer` agent only judges the contract, not the diff.

**Decision required**: keep the deterministic core but allow an external
inferential reviewer behind an opt-in adapter.

**Fix**:

1. New adapter family `internal/adapters/external_reviewer.go` that
   invokes a configured CLI:

   ```yaml
   adapters:
     inferential:
       enabled: false
       command: ["claude", "code", "--agent", "harness-output-reviewer"]
       timeout_seconds: 600
       input: contract_and_diff
       output_schema: review.v1.json
   ```

2. Input bundle: structured JSON with the agreed contract, the diff
   restricted to tracked files, and the latest QA report. Written to a
   temp file passed via stdin or `--input-file`.

3. Output schema: array of findings with `requirement_id`, `severity`,
   `rule`, `file`, `line`, `message`. The harness validates the JSON
   shape and ignores anything that does not match.

4. Findings feed into a new dimension `review` with its own threshold
   and weight, defaulted to `0` so existing repos remain unchanged.

5. The reviewer adapter is allowed to fail (timeout, missing CLI) and
   degrades to a `missing-sensor` finding only when the dimension is
   active.

**Files touched**:

- `internal/adapters/external_reviewer.go` (new)
- `internal/config/config.go` (new `Inferential` adapter family,
  `DimReview`)
- `internal/evaluator/evaluator.go` (aggregate review findings)
- `cmd/harness/doctor.go` (audit external reviewer config)

**Acceptance**:

- A repo can opt in to the dimension and see review findings appear in
  reports.
- Without opt-in, the binary behaves exactly as today and never invokes
  an LLM.
- Doctor's `--strict` warns when the review dimension is active but the
  configured CLI is missing on PATH.

**Depends on**: P0-1 and P0-2 (so the reviewer can be told which REQ-IDs
to look at and check evidence linkage).

---

### P1-3. Drift watch outside the sprint loop

**Status**: gap (Fowler dedicates a section to continuous drift; Anthropic
case study highlights agents that lose ground between sprints)

**Evidence**: `npm audit`, `js-complexity`, and `js-architecture` only run
during `sprint qa`. There is no periodic execution. `harness trend` is
passive recurrence reporting; nothing surfaces new drift.

**Fix**:

1. New command `harness watch [--interval=60m]` runs the fast sensor set
   plus `npm audit`, dead-code detection (new sensor), and SBOM diff,
   writing to `.harness/watch/YYYY-MM-DDTHH.json`.

2. A GitHub Actions template at
   `.github/workflows/harness-watch.yml` invokes `harness watch --once`
   on a schedule, opens an issue when verdict regresses, and uploads the
   report.

3. New adapter `internal/adapters/dead_code.go` (TS/JS only at first via
   `ts-prune` or `knip`). Stubs for Python via `vulture`, Go via
   `staticcheck -unused`, Rust via `cargo machete`.

**Files touched**:

- `cmd/harness/watch.go` (new)
- `internal/adapters/dead_code.go` (new)
- `.github/workflows/harness-watch.yml` template (new)

**Acceptance**:

- `harness watch --once` produces a watch report file without requiring
  a sprint contract.
- Regressing the dead-code count or audit findings between two runs
  shows up in `harness trend` and `harness ui`.

**Depends on**: none.

---

### P1-4. Auto-sizing sprints

**Status**: gap (TLC Spec-Driven canon)

**Evidence**: setup offers `spec-driven|contract|manual` but does not
adjust per-sprint depth. Every sprint runs through the same
Specify/Design/Tasks expectations.

**Fix**:

1. Sprint contract grows a `Size:` field with values `small|medium|large`.

2. Heuristic in `harness sprint new`:
   - `small` when goal touches ≤3 files or has ≤3 deliverables.
   - `large` when the contract declares ≥3 modules, schema changes, or
     `forbidden_imports` constraints.
   - `medium` otherwise.

3. Policy in `internal/planning/policy.go` reads `Size:`:
   - `small`: design optional, tasks optional, no extra REQ overhead.
   - `medium`: tasks required.
   - `large`: design and tasks required, plus a security/perf checklist.

4. Doctor lists active policies for the current size.

**Files touched**:

- `internal/planner/contract.go` (Size parsing and default heuristic)
- `internal/planning/policy.go`
- `cmd/harness/doctor.go`

**Acceptance**:

- A new sprint with one deliverable defaults to `Size: small` and does
  not require `## Tasks`.
- A sprint declaring three modules in `Deliverables` defaults to
  `Size: large` and rejects the contract until design and tasks files
  exist.

**Depends on**: P0-5 (policy module).

---

### P1-5. Context budget visibility

**Status**: gap (TLC Spec-Driven canon mentions a 40k base / 160k working
budget)

**Fix**:

1. New command `harness context size`:
   - sums byte size of `.harness/spec.md`, `progress.md`,
     `agent-protocol.md`, `context/*.md`, and active sprint
     `contract|design|tasks`,
   - converts to estimated tokens using a constant ratio,
   - prints both raw bytes and tokens.

2. `harness doctor` warns when total context exceeds 40k tokens.

3. TUI Doctor view shows context size.

**Files touched**:

- `cmd/harness/context.go` (new)
- `cmd/harness/doctor.go`
- `internal/tui/*`

**Acceptance**:

- `harness context size` exits 0 with a clear table.
- Doctor warns above the threshold; `--strict` does not fail because
  this is a hint, not a contract violation.

**Depends on**: none.

---

### P1-6. Positive prompt injection for sensor messages

**Status**: gap (Fowler highlights "custom linter messages optimized for
LLM consumption" as a known pattern)

**Fix**:

1. Adapter base helper `internal/adapters/llm_hint.go` that rewraps
   default tool output into an agent-readable message:
   - prepend the requirement ID when known,
   - append a one-line "Suggested fix" derived from rule name when
     possible,
   - keep the original message intact.

2. Adapters opt in by calling the helper. ESLint, Ruff, Clippy, Go vet,
   and the new `dead_code` adapter all opt in.

**Acceptance**:

- An ESLint `no-unused-vars` finding in QA output contains both the
  original message and a suggested-action line.

**Depends on**: P0-1 (REQ linkage so the helper can prepend REQ-IDs).

---

## P2 — Quality and follow-up

### P2-1. Tasks status visibility

Add `harness tasks status` and a Tasks view in the TUI. Tracks atomic
task completion against the optional `tasks/sprint-NNN.md` file.

### P2-2. Cross-sprint hotspot view

Surface files touched in N>1 sprints, REQ-IDs that recurrently fail,
and findings older than N days. Pure SQL on the existing `memory.db`.

### P2-3. Optional `.specs/` export

For teams already using TLC Spec-Driven, ship a one-way exporter
`harness specs export` that writes a `.specs/` mirror from `.harness/`
artifacts. Mirror is regenerated, not authoritative.

### P2-4. Stable JSON schema documentation

The reports JSON already includes `schema_version`. Publish the schema
files under `docs/schemas/` and reference them from the agent protocol
so external tools can consume reports safely.

---

## Sequencing

A tight sequence that keeps the framework usable at every step:

1. P0-2 + P0-1 together (REQ-ID + criteria evidence). Releases as
   `v0.6 — Verifiable Acceptance`.
2. P0-3 + P0-4 (hash scope + workspace pinning). Releases as
   `v0.6.1 — Trusted Reports`.
3. P0-5 (policy module). Releases as `v0.6.2 — Real Planning Modes`.
4. P1-1 (pre-commit + fast sensors). Releases as
   `v0.7 — Shift-Left`.
5. P1-3 (drift watch). Releases as `v0.7.1 — Continuous Drift`.
6. P1-4 (auto-sizing). Releases as `v0.7.2 — Adaptive Sprints`.
7. P1-2 (external inferential reviewer). Releases as
   `v0.8 — Optional LLM Review`. Major because it changes the framing
   in `README.md`.
8. P1-5 + P1-6 + P2-*. Steady stream of polish.

Each P0 item has tests, a CHANGELOG entry, and a backwards-compat path
for existing `.harness/` directories. The legacy `schema_version=1`
locks and reports keep working through `v0.6.x`; `v0.7` may require a
one-shot upgrade.

---

## Out-Of-Scope For This Plan

The same items listed in `ROADMAP.md` under "Deliberately Out Of Scope"
remain out of scope:

- Cloud sync.
- Blocking exit codes for git push (kept opt-in via pre-commit).
- LLM contract generation inside the harness binary itself; only
  external adapters call models.
- Cross-project memory.

---

## Validation Notes

Every claim in this plan was verified against the source tree at the
time of writing. Specific file and line references are deliberate so an
agent picking up the work does not need to re-derive context.
