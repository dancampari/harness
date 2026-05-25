# Harness

> **TLC is the methodology. The harness is the deterministic enforcement
> layer that lets a coding agent be held to TLC's method.**

Harness sits next to a coding agent (Codex, Claude Code, Cursor) and turns
TLC's [spec-driven skill](https://agent-skills.techleads.club/skills/tlc-spec-driven/)
into binding gates: a hashed contract that planner + tester roles must both
approve, stack-aware QA sensors (lint, tests, coverage, audit, complexity,
architecture, Playwright E2E, screenshot baselines, SPEC_DEVIATION scanner,
scope-creep guardrail), workspace-SHA pinning so a report cannot survive
code drift, an isolated evaluator subprocess, and an edit guard that
refuses product-file writes before AGREED.

The harness vendors TLC verbatim inside the binary (`internal/skills/tlc/`)
and installs it into `.harness/skills/tlc-spec-driven/`. The agent reads
TLC; the harness validates that the agent followed it. What TLC describes
in prose, the harness measures in code: hashes, validators, sensors,
locks, the `.harness/events.jsonl` log, and the `.harness/run-progress.json`
snapshot.

The harness intentionally does not embed an LLM. It needs no API keys and
makes no model calls — the optional inferential reviewer adapter is the
single place where an external CLI may be shelled out to. Quality evidence
is deterministic, conservative, and reproducible.

## Install With npx

Current public GitHub install. This is the one-command bootstrap:

```bash
npx github:dancampari/harness#v0.10.4
```

It detects the project, creates `.harness/`, asks which coding CLI will drive
the work, asks which planning automation mode should be installed, asks whether
the command should be project-only or global, runs `doctor`, and prints the
command to open the live Harness terminal.

Interactive setup asks:

```text
Which coding CLI will implement code in this repo?
> Claude Code
  Codex
  Cursor IDE
  Auto / all references
  All three
  None

Planning automation mode:
> Spec-driven automation
  Contract automation only
  Manual contracts

Installation scope:
> Project only
  Global command + this project

Use Up/Down arrows and Enter to select. Esc cancels.
```

Global scope copies the resolved Harness binary into the npm global command
directory when available, with a user-local bin directory as fallback. Project
scope only writes repo-local `.harness/` files and agent references.

For zero prompts:

```bash
cd your-project
npx github:dancampari/harness#v0.10.4 --yes
npx github:dancampari/harness#v0.10.4 --cli codex --yes
npx github:dancampari/harness#v0.10.4 --cli claude --yes
npx github:dancampari/harness#v0.10.4 --cli cursor --yes
npx github:dancampari/harness#v0.10.4 --cli claude --planning spec-driven --scope project --yes
npx github:dancampari/harness#v0.10.4 --cli codex --planning manual --scope global --yes
```

`--skills on|off` remains supported as a legacy alias. New installs should use
`--planning spec-driven|contract|manual`.

## Upgrade Existing Project

Use one command to refresh Harness in a project that already has `.harness/`:

```bash
npx github:dancampari/harness#v0.10.4 upgrade --yes
```

The upgrade command preserves project memory and history:

```text
.harness/memory.db
.harness/progress.md
.harness/spec.md
.harness/contracts/
.harness/runs/
.harness/reports/
.harness/evaluations/
```

It refreshes generated Harness files from the current version:

```text
.harness/bin/harness
.harness/skills/
.harness/agent-protocol.md
AGENTS.md / CLAUDE.md / .cursor/rules/harness.mdc
.codex/agents/ / .claude/agents/
.codex/hooks.json / .claude/settings.json
.harness/.gitignore
safe .harness/config.yaml defaults via doctor --fix
```

When `upgrade --yes` detects an existing `harness` command on PATH, it also
refreshes that global command. On Windows this overwrites the old npm shims
`harness.cmd`, `harness.ps1`, and `harness` so plain `harness --version` uses
the newly installed binary instead of an older global package wrapper.

For the latest GitHub commit instead of a pinned release, use the default
branch:

```bash
npx github:dancampari/harness upgrade --yes
```

The GitHub form does not provide npm-style `@latest` semantics. After registry
publishing, the equivalent stable command will be:

```bash
npx @dancampari/harness@latest upgrade --yes
```

The package is also prepared for npm registry publishing as
`@dancampari/harness`. After the npm package is published, the stable user
command is:

```bash
npx @dancampari/harness@latest --version
npx @dancampari/harness@latest
```

The npm wrapper first looks for a local packaged binary, then tries to download
a prebuilt binary from the GitHub release matching the package version, then
falls back to building from source with Go when Go is installed.

## Quick Start

```bash
cd your-project
npx github:dancampari/harness#v0.10.4 --yes
npx github:dancampari/harness#v0.10.4 feature new "implement user auth"
```

With automated contract skills enabled, the coding CLI should create and fill
the sprint contract from the user's prompt. In manual mode, edit the generated
contract yourself:

```text
.specs/features/sprint-001/spec.md
```

Propose and approve the exact contract hash before implementation:

```bash
npx github:dancampari/harness#v0.10.4 feature propose
npx github:dancampari/harness#v0.10.4 feature approve --role planner
npx github:dancampari/harness#v0.10.4 feature approve --role tester
```

Let Codex, Claude Code, Cursor, or a human implement the agreed contract, then
run:

```bash
npx github:dancampari/harness#v0.10.4 feature qa
npx github:dancampari/harness#v0.10.4 feature qa --accept-screenshots
npx github:dancampari/harness#v0.10.4 feature qa --accept-fixtures
npx github:dancampari/harness#v0.10.4 feature repair
npx github:dancampari/harness#v0.10.4 feature score
npx github:dancampari/harness#v0.10.4 run --resume
```

Use `--accept-screenshots` only after reviewing the first visual baseline. Use
`--accept-fixtures` only after reviewing behavior fixture changes. Missing
baselines are failures by design.

If QA fails, Harness writes an actionable repair brief:

```text
.harness/repairs/latest.md
```

Agents must read that brief, fix findings, rerun `harness feature qa`, and
repeat until the verdict is `PASS`. `harness feature score` refuses to
consolidate `FAIL` by default. Use `--allow-fail` only for an explicit
abandoned-sprint audit trail.

## Does The User Interact?

Very little.

Default bootstrap behavior:

- If Codex, Claude Code, or Cursor markers already exist, Harness detects them
  and installs only the matching references.
- If the command is running in a terminal, Harness asks which CLI to configure,
  whether to install automated contract skills, and whether installation should
  be project-only or global.
- If no marker exists and the command is non-interactive, `--yes` installs all
  references and automated contract skills so the setup never stalls.

After bootstrap with contract skills enabled, the user normally interacts only
to approve intent:

- provide the original prompt to the coding CLI;
- answer small product questions only when the request is ambiguous;
- review and accept the first screenshot baseline;
- inspect reports when QA fails only if the agent cannot repair the issue or
  the repair brief asks for human approval.

Codex, Claude Code, and Cursor MUST call Harness functions autonomously after
bootstrap. The public function interface is the Harness CLI. In other words,
the agent function call is a shell command, because external coding CLIs cannot
call in-process Go functions inside another binary.

The generated `.harness/agent-protocol.md`, `AGENTS.md`, `CLAUDE.md`,
`.claude/settings.json`, and `.cursor/rules/harness.mdc` tell the coding tool
to call these functions itself. `CLAUDE.md` is the Claude Code project memory;
`.claude/settings.json` is used only for Claude Code hooks/settings.

When planning automation is enabled, Harness installs one of two skill levels.
The default and recommended mode is spec-driven automation:

```text
.harness/skills/spec-driven/SKILL.md
.harness/skills/spec-driven/references/
.harness/context/
.harness/design/
.harness/tasks/
```

Spec-driven automation adapts the TLC-style Specify, Design, Tasks, Execute,
Validate flow into Harness-native artifacts. It does not create `.specs/` by
default; `.harness/` remains the source of truth.

Contract-only automation installs the smaller author/reviewer pack:

```text
.harness/skills/contract-authoring/SKILL.md
.harness/skills/contract-authoring/references/
.harness/skills/contract-review/SKILL.md
```

The agent references instruct Codex, Claude Code, or Cursor to read that skill,
break the user's prompt into a small sprint, call `harness feature new`, fill the
contract Markdown automatically, and route the exact hash through planner/tester
agreement. In spec-driven mode, they may also create `.harness/design/` and
`.harness/tasks/` artifacts when the sprint needs more planning depth. Harness
remains the deterministic validator; the skills only guide agents toward viable
contracts.

When updating Harness in an existing project, rerun `harness install-hooks
--planning spec-driven` or `harness skills install --planning spec-driven
--force` to refresh the generated skill documents. This does not overwrite
contracts, reports, memory, screenshots, fixtures, approvals, or progress
history.

```bash
harness feature status
harness feature new "<goal>"
harness contract propose
harness contract approve --role planner
harness contract approve --role tester
harness contract status
harness feature qa --format=json
harness feature repair
harness feature score
harness doctor [--strict]
```

The user should not need to say "run Harness" after every task. The installed
agent references make that part of the coding agent's operating protocol.

Integration behavior:

| Tool | Installed reference | How Harness is triggered |
|---|---|---|
| Codex | `AGENTS.md`, `.codex/hooks.json`, `.codex/agents/*.toml` | Codex receives the Harness protocol, project custom agents, and a `PreToolUse` guard for `apply_patch/Edit/Write` |
| Claude Code | `CLAUDE.md`, `.claude/settings.json`, `.claude/agents/*.md` | Claude Code receives the autonomous protocol, project subagents, and a `PreToolUse` guard for `Edit/MultiEdit/Write` |
| Cursor | `.cursor/rules/harness.mdc` | Cursor receives an always-on rule to run Harness autonomously |
| Git | `.git/hooks/pre-push` | Safety-net report before push, non-blocking |

Harness does not block commits or pushes by default. The agreement gate does
block QA until the active contract is `AGREED`; sensor verdicts are still
reported as data for the agent or human to act on.

## TLC Spec-Driven And Agent Agreement

The harness is the gate; TLC is the method. The agent reads TLC at
session start; the harness validates that the agent followed it.

- `.specs/project/PROJECT.md` is the global product spec (TLC's
  `spec.md` at project scope).
- `.specs/project/STATE.md` records decisions, blockers, todos,
  deferred work, and lessons. `harness state record <kind> "<msg>"`
  appends structured entries.
- `.specs/project/ROADMAP.md` is the prioritised feature list per TLC's
  roadmap.md.
- `.specs/features/<slug>/spec.md` is the feature spec. Acceptance
  criteria use TLC's `WHEN <action> THEN system SHALL <outcome>` form;
  the harness rejects the propose step when criteria miss the pattern
  or the spec lacks `## Edge Cases` / `## Out of Scope` sections.
- `.specs/features/<slug>/design.md` is mandatory for `Size: large`
  features (architecture decisions).
- `.specs/features/<slug>/tasks.md` is mandatory for `medium` / `large`
  features. The granularity validator rejects tasks that touch more
  than 3 files unless `Cohesive: true` is set.
- `.specs/quick/NNN-slug/{TASK.md, SUMMARY.md}` records TLC Quick mode
  (≤3 files, no agreement gate).
- `.harness/skills/tlc-spec-driven/SKILL.md` ships the full TLC
  methodology — 18 reference files (specify.md, design.md, tasks.md,
  implement.md, validate.md, discuss.md, brownfield-mapping.md,
  coding-principles.md, state-management.md, session-handoff.md,
  roadmap.md, concerns.md, quick-mode.md, ...).
- `.harness/skills/harness-gate/SKILL.md` documents how the harness
  layers deterministic gates on top of TLC.
- `.harness/agent-protocol.md`, `AGENTS.md`, `CLAUDE.md`, and Cursor
  rules embed TLC's sub-agent delegation matrix and the Knowledge
  Verification Chain (`verification.codebase` → `.docs` → `.context7`
  → `.web` → `.uncertain`). See `docs/MULTI_AGENT_PROTOCOL.md` for the
  full specification.
- The evaluator runs in an isolated subprocess so the builder's context
  cannot leak into the judge's.

The PBQ-style agreement gate is deterministic:

- agreement states: `draft`, `proposed`, `agreed`, `changed`, and `rejected`;
- a stable contract hash for every revision;
- agent approvals recorded under `.harness/approvals/`;
- lock files under `.harness/contracts/sprint-NNN.lock.json`;
- commands `harness contract propose`, `harness contract approve`,
  `harness contract reject`, and `harness contract status`;
- QA blocked until the active contract has the required approvals;
- no LLM judgment inside Harness; agents may write approvals, Harness only
  verifies state, hashes, required roles, and sensor results.

For Codex installations, Harness also writes project-scoped custom agents:

- `.codex/agents/harness-spec-planner.toml` in spec-driven mode;
- `.codex/agents/harness-contract-author.toml`;
- `.codex/agents/harness-contract-reviewer.toml`;
- `.codex/agents/harness-task-worker.toml` in spec-driven mode;
- `.codex/hooks.json` with a `PreToolUse` guard.

The guard blocks `apply_patch`, `Edit`, `MultiEdit`, and `Write` against
product files until the active sprint contract is `AGREED`. Contract files and
Harness control files remain editable so the author/reviewer loop can repair
weak contracts. In Codex, project-local hooks run only when the `.codex/`
project layer is trusted by Codex.

For Claude Code installations, Harness writes equivalent project subagents
under `.claude/agents/` and a `PreToolUse` guard under `.claude/settings.json`.

By default the required roles are `planner` and `tester`. If the contract file
changes after approval, the hash changes and the contract state becomes
`CHANGED`; it must be proposed and approved again before QA.

Reports generated before the contract reached `AGREED` are treated as stale.
The TUI shows them as `STALE/BLOCKED`, `harness feature score` refuses to
consolidate them, and the agent must rerun `harness feature qa` after the
planner/tester approvals.

## Terminal Layout

### QA Report

`harness feature qa` renders a compact terminal card:

```text
â”Œâ”€ harness feature qa Â· sprint 001 â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
â”‚
â”‚  Verdict: PASS    Total Score: 98/100
â”‚
â”‚  â”Œâ”€ Dimension      Score   Threshold   Passed   Findings  â”€â”€â”
â”‚  â”‚  architecture    100      70         âœ“          0       â”‚
â”‚  â”‚  complexity      100      75         âœ“          0       â”‚
â”‚  â”‚  contract        100      80         âœ“          0       â”‚
â”‚  â”‚  correctness     100      80         âœ“          0       â”‚
â”‚  â”‚  coverage         87      70         âœ“          0       â”‚
â”‚  â”‚  e2e             100      70         âœ“          0       â”‚
â”‚  â”‚  security        100      85         âœ“          0       â”‚
â”‚  â””â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”˜
â”‚
â”‚  Evaluation: .harness/evaluations/sprint-001.md
â”‚  Report: .harness/reports/sprint-001.json
â””â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
```

In an interactive terminal, `harness feature qa` and `harness feature score`
open the markdown evaluation automatically after the report is written. Harness
tries `HARNESS_EDITOR`, then `cursor`, then `code`, then the OS default opener.
Set `HARNESS_OPEN_REPORT=0` to disable this behavior.

When a dimension fails, findings are printed underneath with rule, severity,
file, line, and fingerprint. The JSON report contains the same data plus sensor
status and process isolation metadata.

### Doctor

`harness doctor` shows what the project requires and what is installed:

```text
Harness doctor
  Project: harness-demo
  Stack: typescript
  Package manager: npm
  Frameworks: playwright

Active dimensions:
  correctness threshold=80 weight=20
    OK   eslint             available
    MISS jest               install jest and keep tests runnable with npx jest
    OK   vitest             available
  coverage threshold=70 weight=15
    OK   vitest-coverage    available
  e2e threshold=70 weight=10
    OK   playwright         available
```

Use `harness doctor --strict` in CI or before release. Strict mode exits
non-zero when an active dimension has no available real sensor, config is
ambiguous, generated agent references are stale, or contract automation skills
do not include the repair loop. Missing optional alternatives, such as Jest in a
Vitest project, remain warnings as long as the active dimension has another
available sensor.

Use `harness doctor --fix` when Doctor reports safe local config drift, such as
a TypeScript project whose `.harness/config.yaml` still has only the contract
gate and no adapters. The fix command restores detected stack defaults and
adapter lists, and refreshes `.harness/.gitignore`. It does not install package
dependencies, rewrite contracts, alter reports, or change project code.

## Stack Coverage

Harness defaults are strict for supported stacks. If a dimension is active,
at least one real configured sensor must execute.

| Stack | Correctness | Coverage | Security | Optional |
|---|---|---|---|---|
| Node/TypeScript | ESLint, Jest, Vitest | Jest/Vitest coverage | npm audit | JS complexity, import architecture, Playwright |
| Python | ruff, mypy, pytest | pytest-cov | pip-audit | approved fixtures |
| Go | go vet, staticcheck, go test | go test -cover | govulncheck | approved fixtures |
| Rust | clippy, cargo test | - | cargo audit | approved fixtures |
| Universal | - | - | semgrep when configured | requires local semgrep config |

Missing tools are actionable failures for active dimensions. Agents should run
`harness doctor --fix` autonomously when Doctor reports safe Harness config
drift, then install or request approval only for missing project dependencies
that actually change the application environment.

### Live TUI

`harness ui` or `harness run --resume` opens a full-screen Bubble Tea
interface. It refreshes `.harness/` artifacts every 750ms, so calls made
autonomously by Codex, Claude Code, or Cursor show up as soon as Harness writes
contracts, run state, reports, events, or progress.

The UI is organized as a terminal dashboard with six views:

- Overview: current run, quality gate, pipeline, run history, and activity.
- Runs: selectable run history with status, score, duration, agent, and report.
- Report: latest Markdown report preview, with a simple terminal renderer.
- Logs: recent `events.jsonl` and `commands.log` entries.
- Skills: active skills, suggested skills, categories, and adapters.
- Doctor: detected stack, package manager, scripts, validations, alerts, files,
  and risks.

The layout adapts to terminal width. Wide terminals show cards side by side,
medium terminals reduce columns, compact terminals stack cards, and very small
terminals fall back to a minimal status summary.

Controls inside the TUI:

- `1-6` switches views.
- `tab` switches to the next view.
- `enter` opens details for the selected run.
- `o` opens the latest report.
- `d` opens Doctor.
- `:` opens the command prompt.
- `qa`, `accept`, `score`, `status`, `doctor`, `propose`, `approve tester`,
  and `approve planner` run Harness commands without leaving the dashboard.
- `!<shell command>` runs a shell command from the project root.
- `Up/Down` navigates lists or scrolls the active view.
- `r` refreshes and `q` quits.

The TUI uses the terminal alternate screen, so native scrollbar visibility is
terminal-dependent. When content exceeds the viewport, Harness shows internal
range labels like `Report 1-12/40` or `Events 3-10/80`.

The dashboard requires an interactive terminal (TTY). If your IDE output panel
or integrated terminal does not render the full-screen alternate screen, run:

```bash
harness run --resume --no-alt-screen
```

```text
harness   Autonomous Development Pipeline   v0.10.4      Project: harness-demo   Agent: codex   Status: PASS

[1] Overview   [2] Runs   [3] Report   [4] Logs   [5] Skills   [6] Doctor

CURRENT RUN                         QUALITY GATE
Sprint 004                          Score
Exportar helpers formatados         98 /100
Status   : PASS                     ###################-
Agent    : codex                    Dimension       Score   Threshold   Status
Runtime  : 2.7s                     correctness     100     80          PASS

PIPELINE
Contract  ->  Build  ->  QA  ->  Report  ->  Accept
AGREED        DONE       PASS    DONE        DONE

[enter] Details   [o] Open Report   [d] Doctor   [1-6] Switch View   [r] Refresh   [q] Quit
```

## Strict Pass Policy

Harness prefers an explicit `FAIL` over a false `PASS`.

A dimension is active only when both `threshold > 0` and `weight > 0` in
`.harness/config.yaml`. To disable a dimension, set both values to `0`.

If an active dimension has no available sensor that executes, Harness emits a
`missing-sensor` finding, scores that dimension `0`, and returns `FAIL`.

## Quality Dimensions

| Dimension | What it measures | Node/TypeScript sensors |
|---|---|---|
| correctness | Lint and unit tests | `eslint`, `jest`, `vitest` |
| coverage | Test coverage | `jest-coverage`, `vitest-coverage` |
| complexity | Cyclomatic complexity, size, nesting | `js-complexity` |
| security | Dependency vulnerabilities | `npm-audit` |
| architecture | Import graph, cycles, forbidden imports | `js-architecture` |
| behavior | Approved command fixtures | `approved-fixtures` |
| contract | Declared deliverables, exports, and criterion evidence | built-in contract validator |
| e2e | Browser behavior and screenshot baselines | `playwright` |
| review | Optional inferential reviewer (LLM-backed CLI) | `external-reviewer` |

JSON reports include:

- `schema_version`;
- per-dimension scores and findings;
- configured sensor status: registered, available, executed, error, duration;
- isolated evaluator process metadata;
- workspace SHA captured at QA time so `harness feature score` refuses to consolidate stale reports.

Findings carry an optional `hint` field with an LLM-optimized
"Suggested fix / Do NOT" pair for well-known rules, surfaced in the
repair brief under `## Suggested Fixes (LLM-optimized)`.

## Shift-Left With `--fast`

`harness feature qa --fast` runs only the fast static-analysis sensors
(lint, type checks, complexity, architecture, contract structural).
Dimensions without a fast sensor are marked `SKIPPED` and do not block
the verdict. The agreement gate is bypassed so the same command works
during contract authoring. A non-zero exit code on `FAIL` lets git
hooks block the offending commit.

Fast QA is shift-left only: it does not write the full
`.harness/reports/sprint-NNN.json` artifact used by `harness feature
score`. To consolidate a sprint, run full `harness feature qa` after the
contract is agreed, then run `harness feature score`.

Install the blocking pre-commit hook with:

```bash
harness install-hooks --pre-commit
```

The hook respects `git commit --no-verify` for the rare cases where
the user must bypass shift-left.

## Drift Watch

`harness watch once` runs the fast sensor set plus configured audit
adapters (`npm-audit`, `pip-audit`, `govulncheck`, `cargo-audit`)
outside the sprint lifecycle. Reports go to
`.harness/watch/<timestamp>.json` with a `latest.json` pointer, and
deltas versus the previous run are reported so a cron schedule can
fail on regressions:

```bash
harness watch once --fail-on-regression --format=tty
```

A ready-to-copy GitHub Actions workflow lives at
`docs/templates/harness-watch.yml.example`.

## Acceptance Criteria With Evidence

Sprint contracts use a 5-column acceptance table that links each
criterion to a Requirement ID and mechanical Evidence:

```markdown
## Requirements
- REQ-001: Feature rejects invalid input
- REQ-002: Feature handles concurrent requests safely

## Acceptance Criteria
| # | REQ     | Criterion                  | Evidence                          | Threshold |
|---|---------|----------------------------|-----------------------------------|-----------|
| 1 | REQ-001 | Invalid input returns 400  | tests:handles invalid input       | 8/10      |
| 2 | REQ-002 | Concurrent calls serialise | e2e:tests/e2e/feature.spec.ts     | 7/10      |
| 3 | REQ-001 | Approved fixture confirms  | fixture:invalid-input-400         | 9/10      |
```

Evidence kinds: `tests:<substring>` (matched against test files),
`e2e:<path>` (file must exist), `fixture:<name>` (file must exist
under `.harness/fixtures/`). `inspection:<note>` marks a criterion as
requiring manual review.

The legacy 3-column form (`# | Criterion | Threshold`) still parses
for backwards compatibility.

## Sprint Size And Planning Policy

Contracts may declare a `## Size: small|medium|large` section.
Spec-driven planning composes mode rules with size rules:

- `spec-driven` mode: requires `## Requirements`, REQ-IDs on every
  criterion, and mechanical evidence.
- `medium` size: requires `.harness/tasks/sprint-NNN.md`.
- `large` size: also requires `.harness/design/sprint-NNN.md`.

`harness contract propose` and `harness contract status` enforce both
layers. Undeclared size keeps legacy behavior for older contracts.

## Context Budget

`harness context size` estimates the agent-context cost of the harness
memory bundle (`spec.md`, `progress.md`, `agent-protocol.md`,
`context/*.md`, active sprint `contract|design|tasks`). Doctor warns
when the bundle crosses 40k tokens — the soft limit for "useful
working window remaining" in a 200k-token model.

## Optional Inferential Reviewer

The `review` dimension is disabled by default. Opt in by configuring
an external reviewer CLI (anything from `claude code --agent ...` to a
custom Python script around the Anthropic SDK):

```yaml
# .harness/config.yaml
adapters:
  review: [external-reviewer]
thresholds:
  review: 70
weights:
  review: 10
review:
  command: ["claude", "code", "--agent", "harness-output-reviewer"]
  timeout_seconds: 600
```

The reviewer reads a JSON bundle on stdin and emits JSON findings on
stdout. Full I/O contract in `docs/INFERENTIAL_REVIEWER.md`. The
harness binary itself never embeds an LLM SDK.

Review is automatically excluded from `--fast` and from `harness
watch`, so the LLM cost only happens on full `harness feature qa`.

## Approved Fixtures

The optional `behavior` dimension validates stable input/output scenarios from
`.harness/fixtures/*.json`. It is disabled by default. Enable it by setting both
`thresholds.behavior` and `weights.behavior` above zero and keeping
`adapters.behavior: ["approved-fixtures"]`.

Example fixture:

```json
{
  "schema_version": "1",
  "name": "invoice summary",
  "command": "node",
  "args": ["scripts/fixture-invoice-summary.mjs"],
  "timeout_seconds": 10,
  "expect": {
    "exit_code": 0,
    "stdout": "Total: $42.00\n",
    "stderr": ""
  }
}
```

If a fixture has no approved expectation, or output changes, QA fails with
`fixture-baseline-missing` or `fixture-regression`. The agent must ask the user
to review the behavior change. Only after explicit approval should it run:

```bash
harness feature qa --accept-fixtures
```

## Process Isolation

`harness feature qa` runs in two processes:

1. The parent CLI starts an isolated child process with hidden `--internal`.
2. The child runs sensors and writes JSON/Markdown reports.
3. The parent reads the child JSON and renders the terminal output.

The child process gets an allowlisted environment. Sensitive variables from
agent sessions are stripped. Stdin is closed. Stdout is reserved for JSON.
Stderr is used for diagnostics.

## Project Files

Harness writes two sibling trees: `.specs/` carries the agent-authored
project memory and lives in git; `.harness/` carries runtime state and
is mostly gitignored.

```text
.specs/                              # versioned in git
  project/
    PROJECT.md                       # global product spec
    ROADMAP.md                       # prioritised feature list
    STATE.md                         # decisions, blockers, todos, lessons
    HANDOFF.md                       # session-handoff notes
  codebase/                          # brownfield mapping (TLC's 7-file pack)
    STACK.md
    ARCHITECTURE.md
    CONVENTIONS.md
    TESTING.md
    INTEGRATIONS.md
    CONCERNS.md
  features/
    sprint-001/
      spec.md                        # required, WHEN/THEN/SHALL criteria
      design.md                      # required for Size: large
      tasks.md                       # required for Size: medium | large
  quick/
    001-fix-navbar-overflow/
      TASK.md
      SUMMARY.md

.harness/                            # mostly gitignored (memory.db, reports, etc.)
  config.yaml
  agent-protocol.md
  setup.json
  skills/
    tlc-spec-driven/                 # vendored TLC, 18 reference files
      SKILL.md
      references/
    harness-gate/                    # deterministic gate protocol
      SKILL.md
  contracts/
    sprint-001.lock.json             # agreement lock (runtime state)
  approvals/
    sprint-001/
      planner.json
      tester.json
  evaluations/
  fixtures/
    invoice-summary.json
  reports/
  repairs/
    latest.md
  screenshots/
    baseline/
    current/
    diff/
  events.jsonl                       # every pipeline stage emits here
  run-progress.json                  # atomic live snapshot
  memory.db
```

`.specs/` is committed and travels with the repo. `memory.db`,
`reports/`, `repairs/`, and `screenshots/` are generated local state and
stay local. Legacy projects authored before v0.10 keep `.harness/spec.md`,
`.harness/progress.md`, and `.harness/{contracts,design,tasks,context}/`;
`harness upgrade` migrates them losslessly into the layout above.

## Commands

```text
harness                         one-command setup
harness setup [--yes] [--cli auto|codex|claude|cursor|all|none] [--planning auto|spec-driven|contract|manual] [--scope auto|project|global] [--start]
harness init [--force] [--install-hooks] [--cli auto|codex|claude|cursor|all|none] [--planning auto|spec-driven|contract|manual]
harness install-hooks [--interactive] [--cli auto|codex|claude|cursor|all|none] [--planning auto|spec-driven|contract|manual] [--pre-commit]
harness skills install [--force] [--planning auto|spec-driven|contract|manual]
harness skills status
harness doctor [--strict] [--fix]
harness spec

# Canonical feature surface (TLC vocabulary):
harness feature new <goal>
harness feature status
harness feature propose [--sprint N]
harness feature approve --role planner|tester [--sprint N]
harness feature reject  --role planner|tester --reason <text> [--sprint N]
harness feature qa [--format=tty|json] [--fast] [--accept-screenshots] [--accept-fixtures] [--allow-unagreed]
harness feature repair
harness feature score [--allow-fail]
harness feature list

# TLC project memory + ad-hoc:
harness quick "<one-line>"                                 # TLC Quick mode (.specs/quick/NNN-slug/)
harness roadmap                                            # print .specs/project/ROADMAP.md
harness roadmap append "<line>"                            # append a checklist entry
harness state record <decision|blocker|todo|deferred|lesson> "<msg>"   # append to STATE.md
harness session pause ["<note>"]                           # write HANDOFF.md
harness session resume [--clear]                           # read HANDOFF.md (optionally remove it)

# Deprecated aliases (removed in v2.0):
harness sprint new <goal>             # -> harness feature new
harness sprint status                 # -> harness feature status
harness sprint qa | repair | score | list
harness contract status | propose | approve | reject    # still supported as primary contract verbs

# Operational:
harness watch once [--fail-on-regression] [--format=tty|json]
harness watch list [--limit N]
harness context size [--format=tty|json]
harness run [--resume]
harness ui [--resume]
harness progress
harness trend
harness explain <finding-id>
```

## Development

```bash
go test ./...
go vet ./...
npm run build
npm pack
npm run smoke:package
```

Local Windows test before installing from npm/GitHub:

```powershell
cd harness
.\scripts\link-local.ps1
harness --version
harness doctor
```

If `harness` calls an older package, inspect command resolution with:

```powershell
Get-Command harness -All
```

The local link script removes only stale `@dancampari/agent-harness-kit`
shims, builds `dist/harness.exe`, and runs `npm link` so `harness` points to
this checkout.

Deterministic adapters are welcome. Adapters must not call LLMs.

## License

MIT
