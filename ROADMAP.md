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

## v0.4.11 - Doctor Auto-Fix

- [x] Add `harness doctor --fix` for safe Harness config drift repair
- [x] Restore detected stack defaults when a project is stuck in contract-only validation
- [x] Reconfigure missing adapter lists without touching contracts, reports, or project source
- [x] Repair generated `.harness/.gitignore` entries for local artifacts
- [x] Keep the TUI Doctor suggestion aligned with an actual CLI command

## v0.5 - Broader Stack Coverage

- [x] Python: ruff, mypy, pytest, pytest-cov, pip-audit
- [x] Go: go vet, staticcheck, go test -cover, govulncheck
- [x] Rust: clippy, cargo test, cargo audit
- [x] Universal: optional semgrep adapter
- [x] Stack defaults activate conservative quality gates for Python, Go, and Rust
- [x] Doctor and agent protocols support autonomous `harness doctor --fix`

## v0.6 - Verifiable Acceptance, Trusted Reports, Real Planning Modes

Forward-looking plan in `docs/IMPROVEMENT_PLAN.md`. The following P0 items
closed the bugs where the CLI returned PASS verdicts without verifying
what the contract promised.

### P0-1 + P0-2: Verifiable acceptance and REQ-IDs

- [x] Contract parser now accepts a 5-column acceptance table with REQ-ID
      and Evidence cells while preserving the legacy 3-column form.
- [x] New `## Requirements` section parsed when present; REQ-IDs cross-checked
      across deliverables and criteria.
- [x] `CheckAgainstDiff` mechanically verifies declared evidence (`tests:`,
      `e2e:`, `fixture:`) and reports unmet criteria with REQ-ID context.
- [x] `harness sprint new` template updated to use the new format by default;
      legacy contracts keep their pre-existing score.
- [x] `spec-driven/SKILL.md specify.md` documents the new skeleton with
      evidence kinds and traceability rules.

### P0-3: Contract hash covers design and tasks

- [x] `agreement` hashes `.harness/design/sprint-NNN.md` and
      `.harness/tasks/sprint-NNN.md` when present, so silent edits to those
      files invalidate an AGREED state.
- [x] `Status.Hashed` surfaces which artifacts contributed; `harness contract
      status` prints the list.
- [x] Backwards compatible: contract-only sprints hash exactly as in
      v0.5.x.

### P0-4: Reports pinned to workspace SHA

- [x] New `internal/workspace` package computes a deterministic content
      hash of the working tree (ignoring `.git`, `.harness`, `node_modules`,
      build dirs).
- [x] `EvaluationResult.Process.WorkspaceSHA` records the hash at QA time.
- [x] `harness sprint score` refuses to consolidate when the current
      workspace SHA differs from the report's, blocking the "edit after
      PASS, score the old report" silent-bypass.

### P0-5: Planning policy actually gates QA

- [x] New `internal/planning` package reads `.harness/setup.json` and
      enforces structural policies per mode.
- [x] Spec-driven mode requires `## Requirements`, REQ-IDs on every
      criterion, and mechanical Evidence (`tests:` / `e2e:` / `fixture:`).
- [x] Policy errors surface in `harness contract status` and block
      `harness contract propose` so the `--planning` flag is no longer
      cosmetic.

## v0.7 - Shift-Left

Closes the largest feedback-loop gap vs. Fowler's "Keep Quality Left"
guidance. Pre-commit is opt-in; pre-push remains non-blocking.

### P1-1: Pre-commit fast-feedback hook

- [x] `internal/sensors`: new `IsFast(name)` classifier; static-analysis
      adapters (eslint, ruff, mypy, go-vet, staticcheck, clippy,
      js-complexity, js-architecture) are tagged fast.
- [x] `evaluator.Options.Fast` filters configured sensors to fast-only
      and marks dimensions without a fast sensor as `Skipped`. Skipped
      dimensions do not contribute to the verdict or weighted score.
- [x] `harness sprint qa --fast` runs the new mode, skips the agreement
      gate (informational), and propagates `FAIL` as a non-zero exit so
      pre-commit can block.
- [x] Fast runs never overwrite the canonical `.harness/reports/` or
      `.harness/evaluations/` artifacts — those slots remain owned by
      full QA.
- [x] `harness install-hooks --pre-commit` installs a blocking git
      pre-commit that runs `harness sprint qa --fast`. Standard
      `git commit --no-verify` bypass still works.
- [x] `harness doctor` reports both pre-push and pre-commit hook status.

## v0.8 - Continuous Drift Watch

Closes Fowler's "continuous drift monitoring" gap. Reuses the fast
sensor infrastructure from v0.7, adds dependency audits, and runs
outside the sprint lifecycle so quality regressions between sprints are
visible.

### P1-3: Drift watch

- [x] `internal/sensors.IsAudit(name)` classifier covers npm-audit,
      pip-audit, govulncheck, cargo-audit.
- [x] `evaluator.Options.IncludeAudits` extends the fast filter to also
      include audit sensors; `Options.SkipContract` removes the contract
      dimension when there is no sprint to gate.
- [x] New `internal/watch` package: `RunOnce` returns a structured
      report, persists `.harness/watch/<timestamp>.json` plus a
      `latest.json` pointer, computes deltas vs. the previous run, and
      flags regressions.
- [x] `harness watch once` runs one drift pass without requiring an
      active sprint; `--fail-on-regression` propagates regressions as a
      non-zero exit for CI cron.
- [x] `harness watch list` enumerates past reports.
- [x] `harness doctor` surfaces whether `harness watch` has run; nudges
      users to schedule the included GitHub Actions template.
- [x] `docs/templates/harness-watch.yml.example` ships a ready-to-copy
      workflow that schedules a 6-hour cron and uploads reports as CI
      artifacts.

## v0.8.5 - Adaptive Sprints and Context Budget

Closes P1-4 (auto-sizing sprints) and P1-5 (context budget) together,
since both target "right-sized planning". Mode rules from v0.6 still
apply; size rules layer on top.

### P1-4: Sprint size policy

- [x] `planner.Contract` gains a `Size` field parsed from a new
      `## Size` section (values: small, medium, large).
- [x] `planner.Contract.EffectiveSize` returns empty when undeclared so
      legacy contracts skip size gating; the template ships with
      `small` as an explicit, friendly default.
- [x] `planning.RequiresTasks` and `RequiresDesign` predicates plus a
      new `ContractPolicyErrorsWith(mode, contract, presence)` that
      composes mode rules (spec-driven) with size rules (medium/large).
- [x] `agreement.Manager` passes real file presence into the policy so
      a medium sprint that has not yet written `.harness/tasks/sprint-NNN.md`
      cannot reach AGREED, and a large sprint additionally requires
      `.harness/design/sprint-NNN.md`.
- [x] Contract template explains size semantics inline so new sprints
      have ready-to-edit defaults.

### P1-5: Context budget visibility

- [x] New `internal/budget` package inspects the standard agent context
      bundle: `spec.md`, `progress.md`, `agent-protocol.md`,
      `context/*.md`, current sprint `contract|design|tasks`. Converts
      bytes to a token estimate using the 4-bytes/token heuristic.
- [x] `harness context size [--format=tty|json]` surfaces the
      breakdown, total bytes, token estimate, soft limit (40k), and
      OVER BUDGET state.
- [x] `harness doctor` reports the bundle size and warns when it
      crosses the soft limit, nudging users to prune progress.md
      before the agent's window fills up.

## v0.8.7 - Positive Prompt Injection

Closes Fowler's "custom linter messages optimized for LLM consumption"
pattern. Sensor findings carry an opt-in `Hint` field with a
"Suggested fix:" / "Do NOT:" pair so agents picking up a repair brief
do not reflexively choose the laziest fix that silences a rule without
addressing the underlying issue.

### P1-6: Sensor hint catalog

- [x] `internal/sensors.Finding` grows an optional `Hint` field, omitted
      from JSON when empty and not rendered in the compact TTY output.
- [x] `internal/sensors.LLMHint(rule)` catalogs hints for the common
      rules: lint (`no-unused-vars`, `no-explicit-any`, etc.),
      structural (`complexity-too-high`, `circular-import`,
      `forbidden-import`), coverage (`test-failure`,
      `coverage-below-threshold`), e2e (`screenshot-baseline-missing`,
      `visual-regression`), contract (`missing-deliverable`,
      `unmet-criterion`, `missing-sensor`), and security (`npm-audit`).
- [x] `EnrichFinding` is called automatically by the evaluator's
      aggregator and on `missing-sensor` synthetic findings.
- [x] Repair brief markdown renders a deduplicated
      `## Suggested Fixes (LLM-optimized)` section under the findings
      table so agents see each unique hint once per repair pass.

## v0.9 - Optional Inferential Reviewer

Closes the last P1 item and the biggest single gap versus Fowler's
canon: harness now supports inferential sensors as a first-class
dimension. Per the long-standing design rule, the harness binary still
never embeds an LLM; the new adapter is a deterministic shell-out to a
user-configured reviewer CLI.

### P1-2: External reviewer dimension

- [x] New `review` dimension across `sensors.Dimension`, `config`,
      and the evaluator. Active only when both `thresholds.review` and
      `weights.review` are greater than zero (default is zero/disabled).
- [x] `internal/adapters.ExternalReviewer` shells out to the configured
      command, pipes a JSON bundle on stdin, and parses a JSON findings
      array on stdout. Suggestions populate the LLM `Hint` field added
      in v0.8.7 so they appear in the repair brief.
- [x] Fast (pre-commit) and watch (drift cron) flows automatically
      exclude the review dimension — only `harness sprint qa` pays the
      LLM latency cost.
- [x] `harness doctor` reports whether the reviewer is configured,
      misconfigured (command without weight), or partially configured
      (weight without command).
- [x] `docs/INFERENTIAL_REVIEWER.md` documents the I/O contract,
      includes a minimal Python wrapper example, and explains why the
      adapter stays external instead of embedding an SDK.

## v0.9.1 - Realtime Observability

The harness was only live during QA; it sat idle while the coding CLI
authored a contract or implemented code. v0.9.1 makes every pipeline
stage observable in real time. Full notes in `CHANGELOG.md`.

- [x] `internal/events`: append-only `.harness/events.jsonl` activity log
- [x] PreToolUse guard records each agent edit; new PostToolUse hook
      records agent commands; contract/sprint subcommands record
      milestones (`contract.*`, `qa.finished`, `sprint.scored`)
- [x] `internal/progress`: evaluator publishes a live per-sensor QA
      snapshot to `.harness/run-progress.json`, rewritten atomically
- [x] TUI live panel: QA sensor checklist, agent-activity stream across
      Contract/Build, and streamed command output — visible whether the
      run was launched from the dashboard or by an agent
- [x] Guard fails open but never silent: undecodable hook payloads
      record a `guard.warn` event and surface in `harness doctor`;
      robust cwd resolution with a process-cwd fallback
- [x] TUI fixes: new sprint surfaces in Overview; Skills tab split into
      Active Skills and Sensor Adapters

## v0.10 - Distribution Hardening

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
