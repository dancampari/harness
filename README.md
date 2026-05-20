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
npx github:dancampari/harness#v0.3.9
```

It detects the project, creates `.harness/`, asks which coding CLI will drive
the work, asks whether automated contract-authoring skills should be installed,
asks whether the command should be project-only or global, runs `doctor`, and
prints the command to open the live Harness terminal.

Interactive setup asks:

```text
Which coding CLI will implement code in this repo?
> Claude Code
  Codex
  Cursor IDE
  Auto / all references
  All three
  None

Install automated contract-authoring skills?
> Yes
  No

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
npx github:dancampari/harness#v0.3.9 --yes
npx github:dancampari/harness#v0.3.9 --cli codex --yes
npx github:dancampari/harness#v0.3.9 --cli claude --yes
npx github:dancampari/harness#v0.3.9 --cli cursor --yes
npx github:dancampari/harness#v0.3.9 --cli claude --skills on --scope project --yes
npx github:dancampari/harness#v0.3.9 --cli codex --skills off --scope global --yes
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
npx github:dancampari/harness#v0.3.9 --yes
npx github:dancampari/harness#v0.3.9 sprint new "implement user auth"
```

With automated contract skills enabled, the coding CLI should create and fill
the sprint contract from the user's prompt. In manual mode, edit the generated
contract yourself:

```text
.harness/contracts/sprint-001.md
```

Let Codex, Claude Code, Cursor, or a human implement the feature, then run:

```bash
npx github:dancampari/harness#v0.3.9 sprint qa
npx github:dancampari/harness#v0.3.9 sprint qa --accept-screenshots
npx github:dancampari/harness#v0.3.9 sprint score
npx github:dancampari/harness#v0.3.9 run --resume
```

Use `--accept-screenshots` only after reviewing the first visual baseline. A
missing baseline is a failure by design.

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
- inspect reports when QA fails.

Codex, Claude Code, and Cursor MUST call Harness functions autonomously after
bootstrap. The public function interface is the Harness CLI. In other words,
the agent function call is a shell command, because external coding CLIs cannot
call in-process Go functions inside another binary.

The generated `.harness/agent-protocol.md`, `AGENTS.md`, `CLAUDE.md`,
`.claude/settings.json`, and `.cursor/rules/harness.mdc` tell the coding tool
to call these functions itself. `CLAUDE.md` is the Claude Code project memory;
`.claude/settings.json` is used only for Claude Code hooks/settings.

When automated contract skills are enabled, Harness also installs:

```text
.harness/skills/contract-authoring/SKILL.md
.harness/skills/contract-authoring/references/
```

The agent references instruct Codex, Claude Code, or Cursor to read that skill,
break the user's prompt into a small sprint, call `harness sprint new`, and fill
the contract Markdown automatically before implementation. Harness remains the
deterministic validator; the skills only guide the agent toward viable
contracts.

```bash
harness sprint status
harness sprint new "<goal>"
harness sprint qa --format=json
harness sprint score
harness doctor
```

The user should not need to say "run Harness" after every task. The installed
agent references make that part of the coding agent's operating protocol.

Integration behavior:

| Tool | Installed reference | How Harness is triggered |
|---|---|---|
| Codex | `AGENTS.md` Harness Gate | Codex is instructed to run Harness after meaningful changes |
| Claude Code | `CLAUDE.md` + `.claude/settings.json` | `CLAUDE.md` gives Claude Code the autonomous protocol; hooks run Harness automatically on stop and before commits |
| Cursor | `.cursor/rules/harness.mdc` | Cursor receives an always-on rule to run Harness autonomously |
| Git | `.git/hooks/pre-push` | Safety-net report before push, non-blocking |

Harness reports only. It does not block commits or pushes by default; the agent
or human decides what to do with the result.

## Terminal Layout

### QA Report

`harness sprint qa` renders a compact terminal card:

```text
┌─ harness sprint qa · sprint 001 ──────────────────────────────
│
│  Verdict: PASS    Total Score: 98/100
│
│  ┌─ Dimension      Score   Threshold   Passed   Findings  ──┐
│  │  architecture    100      70         ✓          0       │
│  │  complexity      100      75         ✓          0       │
│  │  contract        100      80         ✓          0       │
│  │  correctness     100      80         ✓          0       │
│  │  coverage         87      70         ✓          0       │
│  │  e2e             100      70         ✓          0       │
│  │  security        100      85         ✓          0       │
│  └────────────────────────────────────────────────────────┘
│
│  Report: .harness/reports/sprint-001.json
└────────────────────────────────────────────────────────────────
```

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

### Live TUI

`harness run --resume` opens a full-screen Bubble Tea interface. It refreshes
every 750ms from `.harness/` artifacts, so calls made autonomously by Codex,
Claude Code, or Cursor show up as soon as `harness sprint new`,
`harness sprint qa`, or `harness sprint score` write contracts, reports, or
progress.

The status columns are fixed-width. Long sprint goals render on their own line
so the pipeline stages stay aligned and easy to scan.

```text
harness  Autonomous Development Pipeline

╭──────────────────────────────────────────────────────────────╮
│ Sprints                                                      │
│ #    Contract     Build     QA        Score   Time    Findings │
│ 001  AGREED       DONE      PASS      98      2.5s    0        │
│      Goal validate harness demo                               │
╰──────────────────────────────────────────────────────────────╯

╭──────────────────────────────────────────────────────────────╮
│ Activity                                                     │
│ QA PASS  sprint 001  score 98/100  runtime 2.5s              │
│ contract 100/80 pass  sensors: contract-validator            │
│ coverage 87/70 pass  sensors: vitest-coverage                │
│ e2e 100/70 pass  sensors: playwright                         │
╰──────────────────────────────────────────────────────────────╯

ready   project harness-demo   sprint 1/1   avg score 98   watch just now: qa report updated   elapsed 2m   [q quit | r refresh]
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
| contract | Declared deliverables and exports | built-in contract validator |
| e2e | Browser behavior and screenshot baselines | `playwright` |

JSON reports include:

- `schema_version`;
- per-dimension scores and findings;
- configured sensor status: registered, available, executed, error, duration;
- isolated evaluator process metadata.

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
    contract-authoring/
      SKILL.md
      references/
  contracts/
  evaluations/
  reports/
  screenshots/
    baseline/
    current/
    diff/
  memory.db
```

`progress.md` is the narrative project memory and should be committed.
`memory.db` is a local SQLite index and should stay local.

## Commands

```text
harness                         one-command setup
harness setup [--yes] [--cli auto|codex|claude|cursor|all|none] [--skills auto|on|off] [--scope auto|project|global] [--start]
harness init [--force] [--install-hooks] [--cli auto|codex|claude|cursor|all|none] [--skills on|off]
harness install-hooks [--interactive] [--cli auto|codex|claude|cursor|all|none] [--skills auto|on|off]
harness skills install
harness skills status
harness doctor
harness spec
harness sprint new <goal>
harness sprint status
harness sprint qa [--format=tty|json] [--accept-screenshots]
harness sprint score
harness sprint list
harness run [--resume]
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
