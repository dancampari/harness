# Harness

Deterministic QA harness for teams using coding agents such as Codex, Claude
Code, and Cursor.

Harness sits next to the coding agent and validates each sprint contract
against the real repository state using independent sensors: lint, tests,
coverage, dependency audit, complexity, architecture checks, Playwright E2E,
and visual screenshot baselines.

Harness is intentionally not an LLM reviewer. It does not call models, does not
need API keys, and does not replace human product judgment. Its job is to make
quality evidence visible and conservative.

## Install With npx

Current public GitHub install. This is the one-command bootstrap:

```bash
npx github:dancampari/harness#v0.5.5
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
npx github:dancampari/harness#v0.5.5 --yes
npx github:dancampari/harness#v0.5.5 --cli codex --yes
npx github:dancampari/harness#v0.5.5 --cli claude --yes
npx github:dancampari/harness#v0.5.5 --cli cursor --yes
npx github:dancampari/harness#v0.5.5 --cli claude --planning spec-driven --scope project --yes
npx github:dancampari/harness#v0.5.5 --cli codex --planning manual --scope global --yes
```

`--skills on|off` remains supported as a legacy alias. New installs should use
`--planning spec-driven|contract|manual`.

## Upgrade Existing Project

Use one command to refresh Harness in a project that already has `.harness/`:

```bash
npx github:dancampari/harness#v0.5.5 upgrade --yes
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
npx github:dancampari/harness#v0.5.5 --yes
npx github:dancampari/harness#v0.5.5 sprint new "implement user auth"
```

With automated contract skills enabled, the coding CLI should create and fill
the sprint contract from the user's prompt. In manual mode, edit the generated
contract yourself:

```text
.harness/contracts/sprint-001.md
```

Propose and approve the exact contract hash before implementation:

```bash
npx github:dancampari/harness#v0.5.5 contract propose
npx github:dancampari/harness#v0.5.5 contract approve --role planner
npx github:dancampari/harness#v0.5.5 contract approve --role tester
```

Let Codex, Claude Code, Cursor, or a human implement the agreed contract, then
run:

```bash
npx github:dancampari/harness#v0.5.5 sprint qa
npx github:dancampari/harness#v0.5.5 sprint qa --accept-screenshots
npx github:dancampari/harness#v0.5.5 sprint qa --accept-fixtures
npx github:dancampari/harness#v0.5.5 sprint repair
npx github:dancampari/harness#v0.5.5 sprint score
npx github:dancampari/harness#v0.5.5 run --resume
```

Use `--accept-screenshots` only after reviewing the first visual baseline. Use
`--accept-fixtures` only after reviewing behavior fixture changes. Missing
baselines are failures by design.

If QA fails, Harness writes an actionable repair brief:

```text
.harness/repairs/latest.md
```

Agents must read that brief, fix findings, rerun `harness sprint qa`, and
repeat until the verdict is `PASS`. `harness sprint score` refuses to
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
break the user's prompt into a small sprint, call `harness sprint new`, fill the
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
harness sprint status
harness sprint new "<goal>"
harness contract propose
harness contract approve --role planner
harness contract approve --role tester
harness contract status
harness sprint qa --format=json
harness sprint repair
harness sprint score
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

## Spec Driven And Agent Agreement

Harness is Spec Driven:

- `.harness/spec.md` is the project specification and persistent product bar.
- `.harness/contracts/sprint-NNN.md` turns one user request into a small,
  testable sprint contract.
- In spec-driven mode, agents use the Harness-native Specify, Design, Tasks,
  Execute, Validate flow from `.harness/skills/spec-driven/SKILL.md`.
- Optional planning depth lives under `.harness/context/`, `.harness/design/`,
  and `.harness/tasks/`; Harness does not create `.specs/` by default.
- `.harness/agent-protocol.md`, `AGENTS.md`, `CLAUDE.md`, and Cursor rules tell
  coding agents to create contracts, propose them, approve required roles, run
  QA, read findings, fix, and score.
- The evaluator is deterministic and isolated from the builder process.

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
The TUI shows them as `STALE/BLOCKED`, `harness sprint score` refuses to
consolidate them, and the agent must rerun `harness sprint qa` after the
planner/tester approvals.

## Terminal Layout

### QA Report

`harness sprint qa` renders a compact terminal card:

```text
â”Œâ”€ harness sprint qa Â· sprint 001 â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
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

In an interactive terminal, `harness sprint qa` and `harness sprint score`
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

```text
harness   Autonomous Development Pipeline   v0.5.5      Project: harness-demo   Agent: codex   Status: PASS

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
| contract | Declared deliverables and exports | built-in contract validator |
| e2e | Browser behavior and screenshot baselines | `playwright` |

JSON reports include:

- `schema_version`;
- per-dimension scores and findings;
- configured sensor status: registered, available, executed, error, duration;
- isolated evaluator process metadata.

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
harness sprint qa --accept-fixtures
```

## Process Isolation

`harness sprint qa` runs in two processes:

1. The parent CLI starts an isolated child process with hidden `--internal`.
2. The child runs sensors and writes JSON/Markdown reports.
3. The parent reads the child JSON and renders the terminal output.

The child process gets an allowlisted environment. Sensitive variables from
agent sessions are stripped. Stdin is closed. Stdout is reserved for JSON.
Stderr is used for diagnostics.

## Project Files

Harness creates this local directory:

```text
.harness/
  config.yaml
  spec.md
  agent-protocol.md
  setup.json
  progress.md
  skills/
    spec-driven/
      SKILL.md
      references/
    contract-authoring/
      SKILL.md
      references/
    contract-review/
      SKILL.md
  context/
    STACK.md
    ARCHITECTURE.md
    CONVENTIONS.md
    TESTING.md
    INTEGRATIONS.md
    CONCERNS.md
  design/
    sprint-001.md
  tasks/
    sprint-001.md
  contracts/
    sprint-001.md
    sprint-001.lock.json
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
  memory.db
```

`progress.md` is the narrative project memory and should be committed.
`memory.db`, `reports/`, `repairs/`, and `screenshots/` are generated local
state and should stay local.

## Commands

```text
harness                         one-command setup
harness setup [--yes] [--cli auto|codex|claude|cursor|all|none] [--planning auto|spec-driven|contract|manual] [--scope auto|project|global] [--start]
harness init [--force] [--install-hooks] [--cli auto|codex|claude|cursor|all|none] [--planning auto|spec-driven|contract|manual]
harness install-hooks [--interactive] [--cli auto|codex|claude|cursor|all|none] [--planning auto|spec-driven|contract|manual]
harness skills install [--force] [--planning auto|spec-driven|contract|manual]
harness skills status
harness doctor [--strict]
harness spec
harness sprint new <goal>
harness sprint status
harness contract status [--sprint N]
harness contract propose [--sprint N]
harness contract approve --role planner|tester [--sprint N]
harness contract reject --role planner|tester --reason <text> [--sprint N]
harness sprint qa [--format=tty|json] [--accept-screenshots] [--accept-fixtures] [--allow-unagreed]
harness sprint repair
harness sprint score [--allow-fail]
harness sprint list
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
go build -o dist/harness .
npm pack
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
