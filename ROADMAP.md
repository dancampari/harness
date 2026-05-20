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
