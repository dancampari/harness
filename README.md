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
npx github:dancampari/harness#v0.3.4
```

It detects the project, creates `.harness/`, installs references for Codex,
Claude Code, Cursor, or all three, runs `doctor`, and prints the command to
open the live Harness terminal.

For zero prompts:

```bash
cd your-project
npx github:dancampari/harness#v0.3.4 --yes
npx github:dancampari/harness#v0.3.4 --cli codex --yes
npx github:dancampari/harness#v0.3.4 --cli claude --yes
npx github:dancampari/harness#v0.3.4 --cli cursor --yes
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
npx github:dancampari/harness#v0.3.4 --yes
npx github:dancampari/harness#v0.3.4 sprint new "implement user auth"
```

Edit the generated contract:

```text
.harness/contracts/sprint-001.md
```

Let Codex, Claude Code, Cursor, or a human implement the feature, then run:

```bash
npx github:dancampari/harness#v0.3.4 sprint qa
npx github:dancampari/harness#v0.3.4 sprint qa --accept-screenshots
npx github:dancampari/harness#v0.3.4 sprint score
npx github:dancampari/harness#v0.3.4 run --resume
```

Use `--accept-screenshots` only after reviewing the first visual baseline. A
missing baseline is a failure by design.

## Does The User Interact?

Very little.

Default bootstrap behavior:

- If Codex, Claude Code, or Cursor markers already exist, Harness detects them
  and installs only the matching references.
- If no marker exists and the command is running in a terminal, Harness asks
  one question: Codex, Claude Code, Cursor, all, or none.
- If no marker exists and the command is non-interactive, `--yes` installs all
  references so the setup never stalls.

After bootstrap, the user normally interacts only to approve intent:

- fill or approve the sprint contract;
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

`harness run --resume` opens a full-screen Bubble Tea interface:

```text
harness - Autonomous Development Pipeline

╭──────────────────────────────────────────────────────────────╮
│ Sprints                                                      │
│ #    Goal                         Contract   Build   QA Score│
│ 001  validate harness demo        AGREED     DONE    PASS 98 │
╰──────────────────────────────────────────────────────────────╯

╭──────────────────────────────────────────────────────────────╮
│ Activity                                                     │
│ ### Sprint 001                                               │
│ - Verdict: PASS                                              │
│ - Score: 98/100                                              │
╰──────────────────────────────────────────────────────────────╯

active sprint 1/10   avg score 98   elapsed 2m   [q quit | r refresh]
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
  progress.md
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
harness setup [--yes] [--cli auto|codex|claude|cursor|all|none] [--start]
harness init [--force] [--install-hooks] [--cli auto|codex|claude|cursor|all|none]
harness install-hooks [--interactive] [--cli auto|codex|claude|cursor|all|none]
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

Deterministic adapters are welcome. Adapters must not call LLMs.

## License

MIT
