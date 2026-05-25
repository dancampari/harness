# Harness × TLC Spec-Driven — Unification Plan

**Status:** active. Replaces all earlier versions of this document.

## The reframing

The original concept of this project was *"harness engineering grounded
in TLC spec-driven"*. That never actually shipped. What shipped instead
was a parallel reshape — `cmd/harness/spec_driven_skills.go` embeds
roughly 430 lines of Go strings that re-narrate TLC's methodology in
shortened form. Those re-narrations were authored from the **summary
page** at agent-skills.techleads.club, not from the full skill content.
Reading the canonical TLC files (`.claude/skills/tlc-spec-driven/`,
3,400 lines across 18 files) makes the depth gap obvious:

| TLC reference | Canonical depth | Harness reshape |
|---|---:|---:|
| `tasks.md` | 419 lines, granularity matrix, parallelism rules, three mandatory cross-checks (Diagram-Definition, Test Co-location, Granularity), test co-location validator | ~50 lines: "create tasks file when sprint is medium/large; tasks must be atomic" |
| `implement.md` | 284 lines, mandatory pre-implementation block, TDD (RED → GREEN), hard test constraints, tiered gate checks, SPEC_DEVIATION markers, atomic conventional commits per task, explicit scope guardrail | ~30 lines: "implement only the agreed sprint" |
| `specify.md` | 155 lines, P1/P2/P3 priorities, independently-testable user stories, WHEN/THEN/SHALL acceptance, edge case section, requirement traceability with status progression (Pending → In Design → In Tasks → Implementing → Verified) | flat `REQ-NNN: <one-line statement>` |
| `validate.md` | 256 lines, multi-mode validation including interactive UAT | ~30 lines: "rerun QA on FAIL" |

The harness implements roughly 30–40% of TLC's depth, in *thinner words*
that the agent can read but cannot be held to.

The corrected mental model:

> **TLC is the methodology. The harness is the deterministic enforcement
> layer that lets a coding agent be held to TLC's method.**
>
> The agent reads TLC; the harness validates that the agent followed it.
> What TLC describes in prose, the harness measures in code: hashes,
> validators, sensors, locks, the activity log, the live progress
> snapshot. Together they are *one* system, not two.

The previous plan in this file treated TLC as "a thing to vendor
alongside our reshape". That misread the user intent. The real plan is
to delete the reshape and rebuild on the TLC foundation.

## What this is, and is not

This plan **replaces** the harness's adapted spec-driven content with
TLC verbatim, then upgrades the harness's deterministic gates to enforce
TLC's patterns at the levels of granularity TLC describes.

This plan is **not** about turning the harness into TLC. The harness
contributes things TLC does not and cannot:

- a deterministic agreement gate (hash + planner/tester roles + lock)
- a stack-aware QA pass with sensors, dimensions, and a verdict
- workspace SHA pinning so a report cannot survive code drift
- a process-isolated evaluator (no LLM in the core)
- a live `run-progress.json` snapshot and an append-only `events.jsonl`
- an edit guard before agreement
- a doctor with strict and auto-fix
- pre-commit shift-left
- an optional external inferential reviewer adapter

All of those stay. They are *the gates*. What changes is that the
content the agent reads to know what to do becomes TLC, and the
validators the harness applies become a deterministic mirror of TLC's
discipline.

## Architectural decisions (committed)

### Artifact tree

The agent-authored project memory lives at `.specs/` (versioned in git,
matches TLC verbatim). The harness's runtime state lives at `.harness/`
(mostly gitignored).

```
.specs/                              # versioned in git
├── project/
│   ├── PROJECT.md                   # vision and goals
│   ├── ROADMAP.md                   # features and milestones
│   └── STATE.md                     # persistent memory: decisions, blockers, todos, lessons
├── codebase/                        # brownfield mapping (7 files per TLC)
│   ├── STACK.md
│   ├── ARCHITECTURE.md
│   ├── CONVENTIONS.md
│   ├── STRUCTURE.md
│   ├── TESTING.md                   # Test Coverage Matrix + Gate Check Commands
│   ├── INTEGRATIONS.md
│   └── CONCERNS.md
├── features/                        # auto-sized: spec / (context / design / tasks)
│   └── <slug>/
│       ├── spec.md
│       ├── context.md               # only when Discuss is triggered
│       ├── design.md                # only Large / Complex
│       └── tasks.md                 # only Large / Complex
└── quick/                           # ad-hoc tasks (Quick mode)
    └── NNN-slug/
        ├── TASK.md
        └── SUMMARY.md

.harness/                            # mostly gitignored
├── skills/                          # vendored TLC + harness-gate
│   ├── tlc-spec-driven/             # verbatim TLC (18 files)
│   └── harness-gate/                # small: just the deterministic protocol
├── contracts/<slug>.lock.json       # agreement lock, references .specs/features/<slug>/spec.md
├── approvals/<slug>/{planner,tester}.json
├── reports/<slug>.json
├── evaluations/<slug>.md
├── repairs/latest.md
├── screenshots/, fixtures/
├── memory.db, run-progress.json, events.jsonl
├── traceability.json                # requirement-ID status progression
├── setup.json, config.yaml, agent-protocol.md
└── bin/
```

### Skill distribution

TLC is vendored into the binary via `go:embed` from
`internal/skills/tlc/`. The harness-gate skill (~150 lines) is also
embedded. `harness skills install` extracts both into
`.harness/skills/`. Bumping the vendored TLC is a release-time
decision; no runtime fetch.

### Vocabulary

External vocabulary is **feature** (TLC's term). The CLI keeps
`harness sprint <verb>` as an alias of `harness feature <verb>` for
backwards compatibility, marked deprecated, removed in v2.0.

## The six phases

### Phase 0 — Reframing (docs only, no behavior change)

- Delete the misleading `docs/SPEC_DRIVEN_SKILL_PACK.md` (it documents
  the reshape).
- Replace this `UNIFICATION_PLAN.md` with the corrected plan (done).
- Update `docs/ARCHITECTURE.md` to state the TLC-foundation framing.
- Update `README.md` "Spec Driven And Agent Agreement" section to point
  at TLC and describe the harness as the enforcement layer.

### Phase 1 — Vendor TLC, delete the reshape, ship the harness-gate skill

**This is the foundation. Without it every later phase keeps drifting
from TLC.**

- Create `internal/skills/tlc/tlc-spec-driven/` and copy
  `.claude/skills/tlc-spec-driven/*` verbatim (SKILL.md + README.md +
  16 references). Total ~3,400 lines.
- Create `internal/skills/gate/harness-gate/SKILL.md` (~150 lines)
  containing only what the harness uniquely provides:
  - the agreement gate workflow (propose → planner approve → tester
    approve → AGREED → implement → QA → score)
  - the events.jsonl contract (event types, phases)
  - the run-progress.json contract (snapshot shape, phases)
  - the edit guard (PreToolUse → ".harness/contracts not yet AGREED"
    rejection rule)
  - cross-references back to TLC ("follow TLC's specify.md, then
    propose the resulting spec.md through this gate")
- Embed both via `go:embed`.
- Delete `cmd/harness/spec_driven_skills.go` entirely (~430 lines).
- Delete the embedded `contractAuthoringSkill` and `contractReviewSkill`
  constants in `cmd/harness/skills.go` — TLC's specify.md + discuss.md
  covers contract authoring, and the tester role can be played by the
  harness-gate skill plus TLC's validate.md.
- `cmd/harness/skills.go` install logic rewritten:
  - extract TLC verbatim into `.harness/skills/tlc-spec-driven/`
  - extract harness-gate into `.harness/skills/harness-gate/`
  - remove (on upgrade) the legacy `.harness/skills/spec-driven/`,
    `contract-authoring/`, `contract-review/` directories with a
    one-time migration message
- Update `cmd/harness/install_hooks.go` generated AGENTS.md / CLAUDE.md
  / .cursor/rules content:
  - say "read TLC first" with the path to its SKILL.md
  - then "read harness-gate" for the gate workflow
  - keep the harness invocation list (propose/approve/qa/score/etc.)
- Update `cmd/harness/doctor.go` checks to verify
  `.harness/skills/tlc-spec-driven/SKILL.md` and
  `.harness/skills/harness-gate/SKILL.md` (instead of the legacy
  skill dirs).
- Update tests across `cmd/harness/install_hooks_test.go`,
  `doctor_test.go`, `skills_test.go`, `e2e_test.go` to assert the new
  layout.

### Phase 2 — Migrate artifacts to `.specs/`

- `internal/planner` reads features from
  `.specs/features/<slug>/spec.md`.
- `internal/agreement` keeps lock at
  `.harness/contracts/<slug>.lock.json`; lock body references the spec
  by relative path. Contract hash continues to span spec + design +
  tasks (already extended in v0.6).
- `internal/sprint` reads `.specs/features/<slug>/{design,tasks}.md`
  when present.
- `harness upgrade` migrates legacy projects:
  - `.harness/spec.md` → `.specs/project/PROJECT.md`
  - `.harness/progress.md` → `.specs/project/STATE.md`
  - `.harness/context/*.md` → `.specs/codebase/*.md`
    (adds `STRUCTURE.md` template when missing)
  - `.harness/contracts/sprint-NNN.md` →
    `.specs/features/sprint-NNN/spec.md`
  - `.harness/design/sprint-NNN.md` →
    `.specs/features/sprint-NNN/design.md`
  - `.harness/tasks/sprint-NNN.md` →
    `.specs/features/sprint-NNN/tasks.md`
  - lock files updated to point at the new paths
- TUI Doctor and Overview render `.specs/` paths.
- `.specs/` added to the *committed* set in install messaging; only
  `.harness/` artifacts stay in `.harness/.gitignore`.

### Phase 3 — Enforce TLC patterns as deterministic gates

This is where the harness earns the framing. Every TLC pattern that can
be enforced without LLM judgement becomes a validator, sensor, or
event. Implementation is incremental; each row below is a unit of work.

| TLC pattern (source) | Harness enforcement |
|---|---|
| `tasks.md` Granularity Check (one component / one function / one endpoint per task) | Pre-AGREED validator on `tasks.md` — rejects tasks that touch >1 file unless explicitly cohesive. |
| `tasks.md` Diagram-Definition Cross-Check | Validator parses the ASCII dependency diagram and every task's `Depends on:` line; rejects mismatches. |
| `tasks.md` Test Co-location Validation | Validator cross-references `.specs/codebase/TESTING.md`'s coverage matrix against every task's `Tests:` field. Hard gate before AGREED. |
| `tasks.md` Parallel-safe `[P]` flag rules | Validator: if a task has dependencies in its own phase, strip `[P]` and warn. |
| `implement.md` Tests First (RED → GREEN) | Sensor (Build phase): detects a task's implementation files modified without a corresponding test file modified in the same commit; reports `tdd-violation`. |
| `implement.md` Atomic commits per task | New CLI: `harness feature implement <task-id>` wraps `git commit` with the Conventional Commits format from TLC's matrix. Lock the agent into one commit per task. |
| `implement.md` SPEC_DEVIATION markers | Scanner sensor: greps for orphan SPEC_DEVIATION markers (without `Reason:`) → finding. Markers present → emitted in the report so the user reviews them. |
| `implement.md` Test count tracking | QA records test count per dimension; comparing run-over-run, a decrease without an explanation → finding `test-count-regression`. |
| `implement.md` Scope guardrail (touch only listed files) | Workspace-SHA diff against the task's `Where:` list — files touched outside the list → finding `scope-creep`. |
| `specify.md` P1/P2/P3 stories with WHEN/THEN/SHALL | Structural validator on `spec.md`: every story has a priority, an "Independent Test", at least one acceptance criterion in WHEN/THEN/SHALL form. |
| `specify.md` Requirement traceability with status progression | New `.harness/traceability.json` updated by the validators: each requirement transitions Pending → In Design → In Tasks → Implementing → Verified. The transitions emit events; the live panel can render the matrix. |
| `specify.md` Edge cases section | Validator: present, non-empty. |
| `specify.md` Out of Scope table | Validator: present (warning if empty). |
| Knowledge Verification Chain (Codebase → Docs → Context7 → Web → Flag uncertain) | Encoded as a mandatory instruction in the generated `AGENTS.md` / `CLAUDE.md`. Event types `verification.codebase`, `verification.docs`, `verification.context7`, `verification.web`, `verification.uncertain` — agents emit them via the existing event helper; the live panel renders the chain step. |
| Skill Integrations (mermaid-studio, codenavi) | Bootstrap-time detection in `harness init` / `upgrade` — generated `AGENTS.md` lists installed skills and instructs the agent to prefer them. Doctor reports detected skills. |
| Auto-sizing matrix (Small / Medium / Large / Complex with phase skipping) | The `Size:` field added in v0.8.5 maps to TLC's matrix; policy enforces design/tasks presence per row. Quick mode (Size: small) skips agreement entirely. |
| `state-management.md` structured STATE.md schema (decisions / blockers / todos / deferred / lessons) | `harness state record <kind>` writes structured entries to `.specs/project/STATE.md`. Doctor lints the file's structure. |

### Phase 4 — Add the TLC commands the harness never had

- `harness feature new <slug>` (primary; `sprint new` is alias)
- `harness feature qa | score | repair | status | list` (same)
- `harness feature propose | approve | reject` (aliases of `contract`
  verbs)
- `harness feature implement <task-id>` — wraps the atomic-commit step
  for one task, runs the tier-appropriate gate, updates traceability.
- `harness quick "<one-line>"` — TLC Quick mode for ≤3 files; creates
  `.specs/quick/NNN-slug/TASK.md`, runs `qa --fast`, bypasses agreement.
- `harness roadmap [view|update]` — opens `.specs/project/ROADMAP.md`
  per TLC's roadmap.md template.
- `harness state record <decision|blocker|todo|deferred|lesson> "<message>"`
  — appends a structured entry to `.specs/project/STATE.md`.
- `harness session pause` / `harness session resume` — TLC's
  session-handoff: writes a handoff note, captures current task state,
  produces a "where I left off" summary the next session reads.

### Phase 5 — Multi-agent delegation matched to TLC's matrix

- Rewrite `.claude/agents/harness-spec-planner.md` and
  `.codex/agents/harness_spec_planner.toml` to implement TLC's
  delegation matrix (`SKILL.md` lines 122–152):
  - Research / brownfield mapping → sub-agent
  - Implementing a task → sub-agent (one per task)
  - Parallel `[P]` tasks → one sub-agent per task, concurrent
  - Sequential tasks (no `[P]`) → sub-agent serially
  - Planning, task creation, validation reports → main context
  - Quick mode → main context
- Each delegation emits `agent.delegate.<reason>` and
  `agent.subagent.done` events. The live panel renders them.
- Generated `AGENTS.md` / `CLAUDE.md` include skill-detection
  instructions and the Knowledge Verification Chain.
- New doc `docs/MULTI_AGENT_PROTOCOL.md` codifies who delegates what.

### Phase 6 — Cleanup and retire

- README rewritten with the corrected narrative: TLC is the
  methodology; the harness is the deterministic enforcement.
- All examples updated to `feature` vocabulary.
- `sprint` CLI alias warns deprecated; doctor surfaces `cli.deprecated`
  on use.
- `IMPROVEMENT_PLAN.md` retires the items closed by this unification.
- Doctor `--strict` fails when the vendored TLC version drifts from
  the installed version in a project.

## What stays the same (intentionally)

The harness's original contributions are intact:

- agreement gate (hash + planner/tester + lock)
- 24 sensors / 8 dimensions / PASS-FAIL verdict
- workspace SHA pinning
- isolated evaluator subprocess
- `run-progress.json` and `events.jsonl`
- repair brief automation
- edit guard before AGREED
- 30+ LLM hints (Fowler's "positive prompt injection")
- doctor strict + auto-fix
- drift watch
- pre-commit shift-left
- external inferential reviewer adapter (opt-in)

## What stays out of scope

- The harness as orchestrator process spawning the coding CLI ("Path A"
  from the video's vision). Real possibility, real cost, real CLI
  dependency on headless modes. Out of scope for this unification.
- LLM inside the harness binary. Stays out forever.
- Interactive UAT conducted by the harness — TLC's `validate.md`
  includes a conversational UAT pattern; the harness produces the
  checklist and tracks completion but the conversation stays with the
  agent.
- Cloud sync, cross-project memory. Same.

## Effort estimate

| Phase | Effort | Risk |
|---|---|---|
| 0. Reframing docs | half a day | nil (docs only) |
| 1. Vendor TLC + delete reshape + harness-gate | ~3 days | low — additive in the binary, deletes only embedded text |
| 2. Migrate to `.specs/` | ~5 days | medium — migration must be lossless |
| 3. Enforce TLC patterns as gates | ~3–4 weeks | medium — each row is a small change; the total is the work |
| 4. New TLC commands (feature/quick/roadmap/state/session) | ~5 days | low |
| 5. Multi-agent matrix + skill integrations + verification chain | ~5–8 days | medium — depends on generated-doc clarity |
| 6. Cleanup + README + retire | ~2 days | low |

Total: roughly 6–8 weeks of focused work, shipped as a series of
versions. Phase 1 is the unblock for everything else.

## Going first

Phase 0 + Phase 1 are the foundation. Without them every later phase
keeps re-implementing TLC in Go strings. Start there, ship it, then
proceed sequentially through the rest.
