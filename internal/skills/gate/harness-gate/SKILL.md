---
name: harness-gate
description: The deterministic enforcement layer for TLC spec-driven development. Read this AFTER tlc-spec-driven/SKILL.md. The harness binary validates that the agent followed TLC's method — hashing the contract, gating implementation behind planner+tester agreement, running stack-aware QA sensors, pinning reports to the workspace SHA, recording every phase as an event, and surfacing a live progress snapshot the TUI streams. Use when (1) the agent has authored a spec.md per TLC's specify.md, (2) the project needs deterministic gates the LLM cannot bypass, (3) state transitions must be visible across multi-agent sessions, (4) QA must be reproducible. Triggers on "propose contract", "approve contract", "run qa", "score sprint", "repair findings".
license: MIT
---

# Harness Gate — Deterministic Enforcement for TLC Spec-Driven

The harness is not a methodology. It is the deterministic enforcement
layer that makes TLC's method binding. Read TLC's `SKILL.md` first for
how to think, write specs, break tasks, and implement. Then read this
to know how the harness will hold you to it.

## What the harness owns

| TLC says | The harness adds |
|---|---|
| Write `spec.md` with WHEN/THEN/SHALL acceptance criteria | Hashes the spec; blocks implementation until planner AND tester roles approve the exact hash |
| Break into atomic tasks with cross-checks | Validators reject vague tasks, missing test co-location, diagram-definition mismatches BEFORE the agent starts coding |
| Tests-first, atomic commits per task | Sensors detect implementation-without-tests and scope-creep; the workspace SHA pins the QA report to the exact code state |
| Validate per the matrix | Stack-aware sensors run in an isolated subprocess and write a deterministic report |

The harness contributes:

- **Agreement gate**: planner + tester roles approve a stable hash of
  `spec.md` (+ `design.md`, `tasks.md` when present). Any later edit
  changes the hash and reverts the state to CHANGED — must re-propose
  and re-approve.
- **QA dimensions**: correctness, coverage, complexity, security,
  architecture, behavior, contract, e2e, review. Each has a sensor or
  set of sensors; a verdict is PASS only when every active dimension
  passes its threshold.
- **Sensors are deterministic**: no LLM inside the harness binary.
  External inferential review is opt-in and runs as a separate
  subprocess.
- **Workspace SHA pinning**: `harness sprint score` refuses to
  consolidate a report when the working tree changed since QA ran.
- **Events log** (`.harness/events.jsonl`): every pipeline stage
  appends — contract.created, contract.proposed, contract.agreed,
  agent.edit, agent.bash, qa.finished, sprint.scored, repair.briefed.
  The TUI's live panel renders this in real time.
- **Run-progress snapshot** (`.harness/run-progress.json`): rewritten
  atomically as the evaluator moves through contract → sensors →
  aggregate → done, with per-sensor state.
- **Edit guard**: the coding CLI's PreToolUse hook blocks edits to
  product files while the contract is not AGREED.
- **Doctor**: validates harness coverage, agent references, hooks,
  skills, drift watch, context budget, and the guard's own health.

## Workflow — how the harness gates intersect TLC's phases

```
TLC's Specify         → write .specs/features/<slug>/spec.md
The harness            → no gate; agent authors freely
                       
TLC's Design (opt)    → write .specs/features/<slug>/design.md
The harness            → no gate
                       
TLC's Tasks (opt)     → write .specs/features/<slug>/tasks.md
The harness            → validators run on Propose (granularity,
                         diagram-definition, test co-location);
                         reject before AGREED
                       
TLC's Execute         → implement per task with TDD + atomic commits
The harness            → edit guard before AGREED; sensors detect
                         tdd-violation, scope-creep, missing tests
                       
TLC's Validate        → verify per acceptance criteria, UAT for complex
The harness            → harness sprint qa runs every sensor in an
                         isolated subprocess; verdict PASS/FAIL is
                         deterministic; repair brief auto-generated on
                         FAIL with per-rule "Suggested fix / Do NOT"
                         (LLM-optimized hints)
                       
Project ROADMAP       → maintain .specs/project/ROADMAP.md
The harness            → harness roadmap reads/appends; emits
                         roadmap.updated event
                       
Project STATE         → maintain .specs/project/STATE.md
The harness            → harness state record <kind> writes structured
                         entries; emits state.recorded event
```

## CLI surface

```text
harness setup                       one-command bootstrap
harness init                        write .harness/ and .specs/ skeletons
harness skills install              extract vendored TLC + harness-gate
harness doctor [--strict] [--fix]   verify coverage; auto-fix safe drift
harness feature new <slug>          create .specs/features/<slug>/spec.md
harness feature propose             record the current hash for agreement
harness feature approve --role planner|tester
harness feature reject  --role planner|tester --reason "<why>"
harness feature qa [--fast]         run sensors in isolated subprocess
harness feature score [--allow-fail]
harness feature repair              print the repair brief
harness feature status              show contract + agreement state
harness feature list
harness quick "<one-line>"          TLC quick mode, bypasses agreement
harness roadmap                     open/append .specs/project/ROADMAP.md
harness state record <kind> "<msg>" append to .specs/project/STATE.md
harness session pause | resume      TLC session handoff
harness watch once                  drift monitor (audits + fast sensors)
harness context size                estimate agent-context cost
harness run [--resume]              live TUI
```

`harness sprint <verb>` remains as a deprecated alias for `feature` —
removed in v2.0.

## Required reading order at session start

The generated `AGENTS.md` / `CLAUDE.md` / `.cursor/rules/harness.mdc`
instructs the agent to read, in order:

1. `.harness/skills/tlc-spec-driven/SKILL.md` — methodology
2. `.harness/skills/harness-gate/SKILL.md` — this file
3. `.specs/project/PROJECT.md` (when it exists) — project vision
4. `.specs/project/STATE.md` (when it exists) — persistent memory
5. `.specs/project/ROADMAP.md` (when it exists) — feature roadmap
6. `.specs/codebase/*.md` (when working in existing project)

The harness's `agent-protocol.md` lists the function calls (the CLI
verbs above) the agent must invoke autonomously rather than asking the
user to run them.

## What the harness does NOT do

- It does not conduct conversations. TLC's `discuss.md` (gray-area
  resolution) and the interactive UAT in `validate.md` are done by the
  agent. The harness validates the structural result.
- It does not write code. The agent implements; the harness measures.
- It does not embed an LLM. External inferential review is an opt-in
  adapter (`docs/INFERENTIAL_REVIEWER.md`) that shells out to a
  user-configured CLI.

## Honest limitations

- Some TLC patterns are advisory and cannot be fully deterministic
  without crossing into LLM judgement (semantic deduplication,
  "would a senior engineer flag this as overcomplicated"). Those stay
  inferential; the harness ships the deterministic shell so the rest
  is enforced.
- The harness's "feature" replaces TLC's "feature" 1:1 in vocabulary
  and structure. The legacy "sprint" terminology remains as an alias
  during the v0.9 → v2.0 migration.
