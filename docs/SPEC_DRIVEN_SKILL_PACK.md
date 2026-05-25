# Harness-Native Spec-Driven Skill Pack Plan

This plan adapts the `tlc-spec-driven` skill model into Harness without adding a
parallel `.specs/` system. Harness remains the source of truth and the
deterministic gate. Agent skills guide planning and execution; Harness verifies
contracts, approvals, sensors, reports, and scoring.

Reference:

- https://agent-skills.techleads.club/skills/tlc-spec-driven/
- https://github.com/tech-leads-club/agent-skills/blob/main/packages/skills-catalog/skills/(development)/tlc-spec-driven/SKILL.md

## Design Decision

Do not install the external skill verbatim. Model the full workflow into
Harness-native artifacts:

| Spec-driven concept | Harness-native artifact |
|---|---|
| Project vision and goals | `.harness/spec.md` |
| Persistent state and session memory | `.harness/progress.md` |
| Brownfield codebase mapping | `.harness/context/*.md` |
| Feature spec | `.harness/contracts/sprint-NNN.md` |
| Design notes | `.harness/design/sprint-NNN.md` when needed |
| Atomic task plan | `.harness/tasks/sprint-NNN.md` when needed |
| Execute and verify | `harness feature qa`, `harness feature repair`, `harness feature score` |
| Validation report | `.harness/reports/latest.json` and `.harness/evaluations/sprint-NNN.md` |

This prevents conflicting instructions between `.specs/` and `.harness/`.

## User-Facing Setup Mode

The install questionnaire should evolve from a binary skill choice into a
professional automation mode selector:

```text
Planning automation mode:
> Spec-driven automation
  Contract automation only
  Manual contracts
```

Planned semantics:

- `spec-driven`: install full Harness-native Specify, Design, Tasks, Execute
  skills and provider agents.
- `contract`: install only the current lightweight contract author/reviewer
  skills.
- `manual`: install Harness commands and guards, but let the user write
  contracts.

Backward compatibility:

- Existing `--skills on` maps to `--planning spec-driven` for new setup calls.
- Existing `--skills off` maps to `--planning manual`.

## Phase Model

### Specify

The agent turns the user's prompt into a small, testable sprint contract.

Required Harness behavior:

- Read `.harness/spec.md`, `.harness/progress.md`, and current project context.
- Clarify only product-critical ambiguity.
- Write objective deliverables, acceptance criteria, constraints, and
  verification evidence into `.harness/contracts/sprint-NNN.md`.
- Assign requirement IDs inside the contract for traceability.
- Run `harness contract propose`.
- Planner approval is allowed only after the contract is structurally complete.

### Design

Design is optional and only appears when the sprint has architectural decisions,
new boundaries, data model changes, provider integrations, or security risk.

Required Harness behavior:

- Write `.harness/design/sprint-NNN.md` when needed.
- Link design decisions back to contract requirement IDs.
- Record rejected options and assumptions.
- Do not let design bypass the contract agreement gate.

### Tasks

Tasks are optional for small sprints and mandatory when the work is larger than
three obvious steps or has meaningful dependencies.

Required Harness behavior:

- Write `.harness/tasks/sprint-NNN.md` when needed.
- Each task must be atomic, independently verifiable, and mapped to requirement
  IDs.
- Tests must be co-located with the task that changes behavior.
- Mark parallel-safe tasks only when they do not touch overlapping files or
  shared fragile state.

### Execute

Implementation starts only after the agreement gate returns `AGREED`.

Required Harness behavior:

- Guards continue to block product-file edits while the contract is `DRAFT`,
  `PROPOSED`, `CHANGED`, `REJECTED`, `MISSING`, `BLOCKED`, or stale.
- Agents execute one atomic task at a time.
- After meaningful changes, agents run `harness feature qa --format=json`.
- On `FAIL`, agents run `harness feature repair`, read
  `.harness/repairs/latest.md`, fix findings, and repeat.
- `harness feature score` is allowed only after a non-stale `PASS`.

## Skill Pack Deliverables

Generated files:

```text
.harness/
  skills/
    spec-driven/
      SKILL.md
      references/
        specify.md
        design.md
        tasks.md
        execute.md
        validate.md
        context-loading.md
        brownfield-mapping.md
        state-management.md
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
  tasks/
```

The generated provider references should point to the spec-driven skill first
when `planning_mode=spec-driven`.

## Provider Integration

### Codex

Generated agents:

- `harness_spec_planner`: Specify/Design/Tasks only, no product-file edits.
- `harness_contract_reviewer`: independent tester approval/rejection.
- `harness_task_worker`: implementation after `AGREED`, constrained to one
  assigned task.

The existing edit guard remains mandatory.

### Claude Code

Generated subagents:

- `harness-spec-planner`
- `harness-contract-reviewer`
- `harness-task-worker`

Agent View can show the planner/tester/worker roles, but Harness must not
depend on a proprietary visual feature to enforce correctness.

### Cursor

Cursor receives repository rules and the same spec-driven protocol. Because
Cursor does not expose the same project-local subagent and pre-tool hook model,
Harness should document lower enforcement strength for Cursor and rely more on
`harness doctor --strict`, git hooks, and explicit TUI status.

## Deterministic Enforcement

The skill pack is advisory. The following remain deterministic:

- contract structure validation;
- stable contract hash;
- planner/tester approval state;
- edit guard before agreement;
- isolated QA process;
- active-dimension sensor policy;
- repair brief generation;
- stale report rejection;
- scoring only after PASS.

Do not add LLM judgement inside Harness. Agent-generated specs, designs, and
tasks must be treated as inputs that Harness validates where possible.

## Acceptance Criteria

This implementation is production-ready only when:

- `harness setup` offers the three planning modes.
- `harness skills install --planning spec-driven` writes the full skill pack.
- `harness doctor --strict` detects stale or missing spec-driven skill files.
- Generated Codex, Claude Code, and Cursor references all instruct agents to
  use the same Harness-native flow.
- Existing projects can upgrade without overwriting contracts, progress,
  reports, screenshots, fixtures, approvals, or local memory.
- A demo project can start with one prompt, produce a contract, pass
  planner/tester agreement, implement, run QA, repair failures, and score
  without the user manually writing the contract.
- The workflow does not create `.specs/` unless the user explicitly asks for
  compatibility export in a future version.

## Implementation Sequence

1. Add `planning_mode` to setup metadata while preserving `contract_skills_enabled`.
2. Extend setup prompts and flags with `--planning auto|spec-driven|contract|manual`.
3. Generate the spec-driven skill pack under `.harness/skills/spec-driven/`.
4. Generate Harness-native context, design, and task directories.
5. Update provider references to route contract creation through the
   spec-driven planner when enabled.
6. Extend `doctor --strict` to validate planning mode artifacts.
7. Add tests for setup migration, skill installation, stale skill detection,
   and provider reference content.
8. Validate with `harness-demo` and a real Node/TypeScript project.

## Follow-Up Track

After the skill pack lands:

- Add optional `.specs/` export for teams already using TLC-style folders.
- Add a `harness context refresh` command for brownfield mapping.
- Add a `harness tasks status` command for sprint task visibility in the TUI.
- Add task-level repair summaries that map QA findings back to requirement IDs.
- Add provider-specific launch helpers for Claude Agent View and Codex agent UI
  when reliable public command surfaces exist.
