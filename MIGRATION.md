# Migration Guide

## 0.5.x → 0.9.0

The 0.9 release closes every P0 and P1 item from
`docs/IMPROVEMENT_PLAN.md`. Existing projects keep working with no
mandatory config edits, but a few one-time effects apply on the next
`harness sprint qa` run. Read this once before upgrading a production
installation.

## TL;DR

```bash
# Refresh the binary and generated agent files.
npx github:dancampari/harness#v0.9.0 upgrade --yes

# If you have an AGREED sprint with design/ or tasks/ files, re-approve.
harness contract status
harness contract propose
harness contract approve --role planner
harness contract approve --role tester

# (Optional) install the new shift-left pre-commit hook.
harness install-hooks --pre-commit

# (Optional) schedule drift watch.
cp docs/templates/harness-watch.yml.example .github/workflows/harness-watch.yml
```

## Required actions

### Re-approve sprints that have design/ or tasks/ files

v0.9 widens the contract hash to include
`.harness/design/sprint-NNN.md` and `.harness/tasks/sprint-NNN.md` when
those files exist. The intent is to prevent silent edits to those
files from bypassing tester approval.

Effect on existing projects:

- **Sprints with only `contracts/sprint-NNN.md`**: hash is identical to
  v0.5.x. No action needed.
- **Sprints that also have `design/sprint-NNN.md` or
  `tasks/sprint-NNN.md`**: the hash is now different. The first
  `harness contract status` call after upgrade reports the state as
  `CHANGED`. Run `propose`/`approve` once to re-anchor.

`harness contract status` now prints a `Hashed:` line so you can see
which artifacts contributed.

### Score consolidation refuses stale reports

`EvaluationResult.Process.WorkspaceSHA` is recorded at QA time.
`harness sprint score` now refuses to consolidate when the workspace
has changed since QA. Workflow change:

```bash
# Before v0.9: edit, sprint qa, edit again, sprint score → consolidated stale report.
# v0.9:        edit, sprint qa, edit again, sprint score → error.

# Correct workflow:
harness sprint qa          # records WorkspaceSHA
# (no edits)
harness sprint score       # consolidates if SHA still matches
```

If you genuinely changed nothing but the workspace hash drifted
(generated files, etc.), see "Tuning workspace hash" below.

## Optional actions

### Adopt the 5-column acceptance table

Existing 3-column tables keep working. To get mechanical evidence
checks, migrate one sprint at a time:

```markdown
## Requirements
- REQ-001: <statement>

## Acceptance Criteria
| # | REQ     | Criterion               | Evidence                          | Threshold |
|---|---------|-------------------------|-----------------------------------|-----------|
| 1 | REQ-001 | Observable outcome      | tests:handles edge case           | 8/10      |
```

Evidence kinds: `tests:<substring>`, `e2e:<path>`,
`fixture:<name>`, `inspection:<note>`. See README → "Acceptance
Criteria With Evidence".

### Declare sprint size when scope grows

Add a `## Size` section to flag medium or large sprints:

```markdown
## Size
medium
```

`medium` requires a tasks file; `large` additionally requires a
design file. Sprints without `## Size` keep legacy behavior (no
size-based requirements).

### Enable spec-driven enforcement

If `.harness/setup.json` has `planning_mode: spec-driven`, the
agreement gate now enforces structural rules: `## Requirements`
present, every criterion carries a REQ-ID, and Evidence is mechanical
(not `inspection:`).

If you opted into spec-driven mode but never adopted REQ-IDs, the
first `harness contract propose` after upgrade will fail with a clear
error. Either:

- Migrate the contract to the 5-column form with REQ-IDs, or
- Switch the mode to `contract` or `manual` in
  `.harness/setup.json` and rerun `harness install-hooks` to refresh
  the generated agent docs.

### Pre-commit shift-left

`harness install-hooks --pre-commit` writes a blocking git pre-commit
hook that runs `harness sprint qa --fast`. This is opt-in. Existing
hooks are not touched.

`--no-verify` still bypasses the hook for emergency commits.

### Drift watch

`harness watch once` runs the fast sensor set plus configured audit
adapters and writes a report to `.harness/watch/`. Copy
`docs/templates/harness-watch.yml.example` to
`.github/workflows/harness-watch.yml` to schedule a 6-hour cron with
artifact upload and regression-failing exit.

### Optional inferential reviewer

The `review` dimension is disabled by default. To enable an
LLM-backed reviewer, edit `.harness/config.yaml`:

```yaml
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

Full I/O contract in `docs/INFERENTIAL_REVIEWER.md`.

## Tuning workspace hash

If `harness sprint score` reports unexpected workspace drift,
inspect what changed:

```bash
# JSON output includes Process.WorkspaceSHA.
cat .harness/reports/latest.json | jq .process.workspace_sha
```

`internal/workspace` walks the working tree and skips a built-in list
of directories (`.git`, `.harness`, `node_modules`, `dist`, `build`,
`coverage`, `target`, `.next`, `.cache`, `.turbo`, `.pytest_cache`,
`__pycache__`, `venv`, `.venv`). Generated files outside those paths
do contribute to the hash; add them to your project's `.gitignore`
and to `internal/workspace.SkippedDirs` if you need them ignored by
harness too.

## Breaking changes (none)

There are no breaking changes to the public CLI surface. Every
addition listed above is opt-in or preserves legacy behavior.

The only effect that may surprise an existing user is the re-approval
required for sprints with design/tasks files (see "Required actions"
above), which is exactly what the change was meant to enforce.

## Need help?

- README → "Quick Start" walks through the new workflow.
- `harness doctor` (and `--strict` in CI) reports which features are
  active and surfaces inconsistencies.
- File an issue at the repository if a v0.5.x contract no longer
  parses as expected.
