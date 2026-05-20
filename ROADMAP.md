# Roadmap

## v0.1 - Walking Skeleton

- [x] CLI command tree: init, spec, sprint, run, progress, trend, explain, install-hooks
- [x] Stack detection and stack-aware config defaults
- [x] SQLite memory and progress.md narrative memory
- [x] Sensor interface, registry, evaluator, weighted scoring
- [x] Sprint lifecycle: Contract -> Build -> QA -> Score
- [x] TTY and JSON reports
- [x] Bubble Tea TUI

## v0.2 - Isolated Evaluator

- [x] `harness sprint qa --internal` hidden subprocess worker
- [x] Parent process collects child JSON only
- [x] Allowlisted evaluator environment
- [x] Closed stdin, JSON-only stdout, stderr diagnostics
- [x] Evaluation markdown and JSON written by the child process
- [x] Integration test for distinct PID and stripped builder env

## v0.3 - Production Node/TypeScript Harness

- [x] Config v2 strict active-dimension policy
- [x] `missing-sensor` findings for active dimensions without executed sensors
- [x] Structured sensor status in JSON and markdown reports
- [x] Node/TS sensors: ESLint, Jest, Vitest, coverage, npm audit
- [x] Node/TS static sensors: complexity and architecture
- [x] Playwright screenshot current/baseline comparison
- [x] `harness sprint qa --accept-screenshots`
- [x] `harness doctor`
- [x] `harness doctor --strict` for CI-safe harness coverage checks
- [x] npx package wrapper and install docs
- [x] Interactive setup choices for CLI, contract skills, and install scope
- [x] Arrow-key setup prompts with Enter confirmation
- [x] Agent contract-authoring skill pack in `.harness/skills/`
- [x] Animated live pipeline dashboard with automatic QA verdict panel
- [x] Live TUI header shows the packaged release version
- [x] Faster spinner animation with unchanged artifact polling cadence
- [x] Interactive QA/score opens the markdown evaluation report automatically
- [x] CI smoke test for vet, tests, build, npm pack, and npm exec

## v0.4 - Spec-Driven Multi-Agent Agreement

- [x] Contract agreement states: draft, proposed, agreed, changed, rejected
- [x] Stable contract hash per revision
- [x] Agent approval records for planner and tester roles
- [x] CLI commands: `harness contract propose`, `harness contract approve`, `harness contract reject`, `harness contract status`
- [x] QA gate blocks until required agents agree on the same contract hash
- [x] Agreement history in `.harness/` lock and approval artifacts
- [x] Contract review skill for independent tester/reviewer role
- [x] Keep Harness deterministic: agents write approvals, Harness verifies state
- [x] Codex project hooks and custom agents for contract author/reviewer flow
- [x] Claude Code project subagents and edit guard for contract-first flow
- [x] Repair brief loop: failed QA writes `.harness/repairs/latest.md`
- [x] `harness sprint score` refuses FAIL by default
- [x] TUI command prompt, internal scrolling, and resize-safe rendering
- [ ] Optional provider-specific launch commands for opening Claude Agent View / Codex agent UI

## v0.4.5 - Approved Behaviour Fixtures

- [x] Optional `behavior` dimension
- [x] `approved-fixtures` sensor for `.harness/fixtures/*.json`
- [x] `harness sprint qa --accept-fixtures`
- [x] Repair brief actions for missing or changed approved fixtures
- [x] Human approval boundary for fixture baseline updates

## v0.4.6 - Harness-Native Spec-Driven Skill Pack

Reference plan: `docs/SPEC_DRIVEN_SKILL_PACK.md`.

- [x] Add setup planning modes: spec-driven, contract automation only, manual
- [x] Preserve backward compatibility for `--skills on|off`
- [x] Generate `.harness/skills/spec-driven/` with Specify, Design, Tasks, Execute, Validate references
- [x] Map TLC-style project memory to `.harness/spec.md`, `.harness/progress.md`, and `.harness/context/*.md`
- [x] Add optional `.harness/design/` and `.harness/tasks/` sprint artifacts
- [x] Require requirement IDs and traceability in generated spec-driven instructions
- [x] Update Codex agents for spec planner, contract reviewer, and task worker roles
- [x] Update Claude Code subagents for spec planner, contract reviewer, and task worker roles
- [x] Update Cursor rules for the spec-driven protocol and document enforcement limits
- [x] Extend `doctor --strict` to detect missing or stale spec-driven skill artifacts
- [x] Add tests for setup migration, skill generation, stale detection, and provider reference content
- [x] Validate end-to-end in `harness-demo` from prompt -> contract -> agreement -> implementation -> QA -> score

## v0.4.7 - Git Hook Quiet Mode

- [x] Git pre-push hook skips repositories that do not have `.harness/config.yaml`
- [x] Generated hook resolves the repository root before checking Harness state
- [x] Regression test covers hook generation for repos without Harness config

## v0.4.8 - Terminal UI Dashboard

- [x] Add `harness ui` as a dedicated TUI entrypoint
- [x] Replace the monolithic sprint/debug panel with six navigable views
- [x] Add Overview cards for current run, quality gate, pipeline, history, and activity
- [x] Add Runs, Report, Logs, Skills, and Doctor views
- [x] Add responsive full, medium, compact, and tiny layouts
- [x] Load new run artifacts from `.harness/current-run.json` and `.harness/runs/*`
- [x] Preserve fallback support for legacy contracts/reports/progress artifacts

## v0.4.9 - Terminal UI Polish

- [x] Remove lateral truncation ellipses from dashboard rendering
- [x] Tighten terminal-width clipping and padding
- [x] Refine Quality Gate score bar with green filled and muted empty segments
- [x] Align Pipeline stages and statuses in fixed columns
- [x] Keep footer focused on shortcuts and move notices into Latest Activity
- [x] Reduce aggressive truncation in Runs History

## v0.4.10 - Terminal UI Visual Refinement

- [x] Simplify the dashboard chrome with section rules instead of heavy card boxes
- [x] Rebalance wide, medium, compact, and tiny terminal breakpoints
- [x] Refine Overview, Runs, Report, Logs, Skills, and Doctor view spacing
- [x] Improve selected-row and status styling without full-row color flooding
- [x] Keep the release header version aligned with packaged builds

## v0.5 - Broader Stack Coverage

- [ ] Python: ruff, mypy, pytest, pytest-cov, pip-audit
- [ ] Go: go vet, staticcheck, go test -cover, govulncheck
- [ ] Rust: clippy, cargo test, cargo audit
- [ ] Universal: optional semgrep adapter

## v0.6 - Distribution Hardening

- [x] GitHub Actions CI for formatting, vet, tests, build, npm pack, and npm exec smoke
- [x] GitHub Actions release job to cross-compile Linux/macOS/Windows for amd64/arm64
- [ ] Publish npm package with prebuilt binaries in `dist/`
- [ ] Homebrew tap
- [ ] Signed binaries
- [ ] Stable JSON schema documentation and migration notes

## Deliberately Out Of Scope

- LLM-based review inside the deterministic harness
- Cross-project memory
- Blocking exit codes
- LLM-based contract generation inside Harness itself
- Cloud sync
