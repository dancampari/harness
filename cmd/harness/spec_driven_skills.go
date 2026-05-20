package harness

import "path/filepath"

func specDrivenSkillFiles(root string) map[string]string {
	base := filepath.Join(root, "skills", "spec-driven")
	refs := filepath.Join(base, "references")
	return map[string]string{
		filepath.Join(base, "SKILL.md"):                    specDrivenSkill,
		filepath.Join(refs, "specify.md"):                  specDrivenSpecifyReference,
		filepath.Join(refs, "design.md"):                   specDrivenDesignReference,
		filepath.Join(refs, "tasks.md"):                    specDrivenTasksReference,
		filepath.Join(refs, "execute.md"):                  specDrivenExecuteReference,
		filepath.Join(refs, "validate.md"):                 specDrivenValidateReference,
		filepath.Join(refs, "context-loading.md"):          specDrivenContextLoadingReference,
		filepath.Join(refs, "brownfield-mapping.md"):       specDrivenBrownfieldReference,
		filepath.Join(refs, "state-management.md"):         specDrivenStateReference,
		filepath.Join(refs, "harness-artifact-mapping.md"): specDrivenArtifactMappingReference,
	}
}

func specDrivenContextTemplates(root string) map[string]string {
	base := filepath.Join(root, "context")
	return map[string]string{
		filepath.Join(base, "STACK.md"):        contextStackTemplate,
		filepath.Join(base, "ARCHITECTURE.md"): contextArchitectureTemplate,
		filepath.Join(base, "CONVENTIONS.md"):  contextConventionsTemplate,
		filepath.Join(base, "TESTING.md"):      contextTestingTemplate,
		filepath.Join(base, "INTEGRATIONS.md"): contextIntegrationsTemplate,
		filepath.Join(base, "CONCERNS.md"):     contextConcernsTemplate,
	}
}

const specDrivenSkill = `---
name: harness-spec-driven
description: Use when an agent receives a user request in a Harness repository and must run the Harness-native Specify, Design, Tasks, Execute, Validate flow before and during implementation.
---

# Harness Spec-Driven Automation

This skill adapts the TLC spec-driven workflow into Harness-native artifacts.
Do not create a parallel .specs/ tree. Harness files under .harness/ are the
source of truth.

## Source Of Truth

- Project spec: .harness/spec.md
- Persistent state: .harness/progress.md
- Brownfield context: .harness/context/*.md
- Sprint contract: .harness/contracts/sprint-NNN.md
- Optional design: .harness/design/sprint-NNN.md
- Optional task plan: .harness/tasks/sprint-NNN.md
- Validation: .harness/reports/latest.json and .harness/evaluations/sprint-NNN.md
- Repairs: .harness/repairs/latest.md

## Workflow

1. Context Load
   - Read .harness/spec.md, .harness/progress.md, .harness/agent-protocol.md,
     and relevant .harness/context/*.md files.
   - Inspect the existing codebase before deciding where work belongs.

2. Specify
   - Convert the user prompt into the smallest useful sprint.
   - Create or update .harness/contracts/sprint-NNN.md.
   - Use requirement IDs such as REQ-001 and map each acceptance criterion to
     observable evidence.
   - Run harness contract propose after the contract is complete.
   - Approve only the planner role after the contract is complete.

3. Design
   - Create .harness/design/sprint-NNN.md only when the sprint has meaningful
     architecture, data model, security, integration, or migration decisions.
   - Keep design decisions linked to requirement IDs.

4. Tasks
   - Create .harness/tasks/sprint-NNN.md when the sprint has more than three
     non-trivial implementation steps or parallel work.
   - Each task must be atomic, verify one outcome, and map back to requirement
     IDs.

5. Agreement
   - Route the proposed contract to the independent tester/reviewer.
   - If rejected, repair planning artifacts only, propose the new hash, and
     approve planner again.
   - Do not edit product files until harness contract status returns AGREED.

6. Execute
   - Implement only the agreed sprint.
   - Prefer one atomic task at a time.
   - Do not weaken the contract to make implementation pass.

7. Validate
   - Run harness sprint qa --format=json after meaningful changes.
   - If Doctor reports safe config drift or says to run doctor --fix, run
     harness doctor --fix before asking the user.
   - If FAIL, run harness sprint repair, read .harness/repairs/latest.md, fix
     findings, and rerun QA.
   - Repeat until PASS.
   - Run harness sprint score only after a non-stale PASS.

## References

- Specify: references/specify.md
- Design: references/design.md
- Tasks: references/tasks.md
- Execute: references/execute.md
- Validate: references/validate.md
- Context loading: references/context-loading.md
- Brownfield mapping: references/brownfield-mapping.md
- State management: references/state-management.md
- Harness artifact mapping: references/harness-artifact-mapping.md
`

const specDrivenSpecifyReference = `# Specify

Specify turns a user prompt into a sprint contract that another agent can
implement and a tester can verify without guessing intent.

## Inputs

- User request
- .harness/spec.md
- .harness/progress.md
- .harness/context/*.md
- Existing code and tests

## Required Output

Write .harness/contracts/sprint-NNN.md with:

- one small goal;
- requirement IDs such as REQ-001;
- concrete deliverables with paths, routes, exported symbols, commands, schema
  names, migrations, or test files;
- acceptance criteria that reference requirement IDs;
- constraints for architecture, security, complexity, visual quality, and
  forbidden shortcuts when relevant;
- verification evidence explaining which Harness sensors, tests, fixtures, or
  inspections will prove each requirement.

## Rules

- Ask a product question only when ambiguity changes the outcome.
- Do not turn a full product request into one sprint.
- Do not lower thresholds or remove risks to make QA easy.
- For deterministic behavior changes, add or update .harness/fixtures/*.json.
- For UI/browser behavior, require Playwright coverage and screenshot review
  when visuals matter.
- When done, run harness contract propose and then approve planner only if the
  contract is complete.
`

const specDrivenDesignReference = `# Design

Design is optional. Use it when the sprint contains decisions that should be
explicit before implementation.

Create .harness/design/sprint-NNN.md when the sprint changes:

- module boundaries;
- database schema or migrations;
- permissions, auth, tenancy, or RLS;
- external integrations;
- caching or async workflows;
- visual systems or reusable UI patterns;
- security-sensitive behavior.

## Template

# Design - Sprint NNN

## Linked Requirements

- REQ-001: ...

## Decision Summary

- ...

## Options Considered

| Option | Pros | Cons | Decision |
|---|---|---|---|
| ... | ... | ... | ... |

## Architecture Notes

- ...

## Risks And Mitigations

- ...

## Verification Plan

- ...

Design is not permission to implement. Implementation still waits for contract
agreement.
`

const specDrivenTasksReference = `# Tasks

Tasks are optional for tiny sprints and required when implementation needs
explicit sequencing.

Create .harness/tasks/sprint-NNN.md when:

- the sprint has more than three non-trivial steps;
- multiple files or modules must change;
- tests and product code should be paired;
- work could be delegated safely;
- the repair loop needs a clear checklist.

## Task Quality Rules

- One task should produce one verifiable outcome.
- Each task maps to one or more requirement IDs.
- Tests belong near the behavior they validate.
- Do not mark tasks parallel-safe if they touch the same files or shared
  fragile state.
- Do not include vague tasks such as "polish" or "fix bugs".

## Template

# Tasks - Sprint NNN

| ID | Requirement | Task | Files | Verification | Parallel |
|---|---|---|---|---|---|
| T-001 | REQ-001 | ... | ... | ... | no |

## Execution Notes

- ...
`

const specDrivenExecuteReference = `# Execute

Execution starts only after harness contract status returns AGREED.

## Rules

- Read the current contract, optional design, optional tasks, and relevant
  context before editing product files.
- Implement only the agreed sprint.
- Prefer one atomic task at a time.
- If a task reveals the contract is wrong, stop and route back to the planner;
  do not silently change scope.
- Keep tests with the behavior they verify.
- Do not introduce new dependencies unless the sprint contract permits them or
  the user approves the stack change.
- Do not accept screenshots or fixtures without explicit user approval.

## After Meaningful Changes

Run: harness sprint qa --format=json

If QA fails, switch to Validate.
`

const specDrivenValidateReference = `# Validate

Validation is Harness-owned and deterministic.

## Required Loop

1. Run harness sprint qa --format=json.
2. Read .harness/reports/latest.json.
3. If Doctor reports safe config drift or says to run doctor --fix, run
   harness doctor --fix autonomously.
4. If verdict is FAIL, run harness sprint repair.
5. Read .harness/repairs/latest.md.
6. Fix the listed findings without weakening the agreed contract.
7. Rerun QA.
8. Repeat until PASS.
9. Run harness sprint score only after PASS.

## Human Approval Boundaries

Ask the user before:

- changing acceptance criteria;
- installing a dependency that changes the app stack;
- accepting visual baselines with harness sprint qa --accept-screenshots;
- accepting behavior fixtures with harness sprint qa --accept-fixtures.

Never claim completion with stale, unagreed, or failing QA.
`

const specDrivenContextLoadingReference = `# Context Loading

Before planning or implementation, load only the context needed for the sprint:

1. .harness/spec.md
2. .harness/progress.md
3. .harness/agent-protocol.md
4. .harness/context/*.md files relevant to the sprint
5. package manifests, framework config, and tests that define the local style
6. existing modules touched by the sprint

Prefer current repository evidence over assumptions. If context files are
missing or stale, update them only when doing so helps future sprints.
`

const specDrivenBrownfieldReference = `# Brownfield Mapping

For an existing project, do not plan from an empty-app assumption.

Use .harness/context/*.md to record stable facts:

- STACK.md: frameworks, package manager, scripts, runtime versions
- ARCHITECTURE.md: modules, boundaries, routing, data flow
- CONVENTIONS.md: naming, UI, folder structure, testing style
- TESTING.md: commands, test frameworks, coverage expectations
- INTEGRATIONS.md: auth, database, storage, APIs, external services
- CONCERNS.md: risks, recurring failures, fragile areas

Keep these files concise. They are memory aids, not documentation dumps.
`

const specDrivenStateReference = `# State Management

Harness has two state layers:

- .harness/progress.md is committed narrative memory.
- .harness/memory.db is local generated runtime memory.

Agents should update progress.md after scoring a meaningful sprint or when a
planning decision changes future work. Do not edit memory.db manually.

Progress entries should include:

- sprint number and goal;
- verdict and score;
- important design decisions;
- remaining work;
- known risks or follow-up sprints.
`

const specDrivenArtifactMappingReference = `# Harness Artifact Mapping

This skill intentionally maps spec-driven concepts into Harness artifacts.

| Concept | Harness Artifact |
|---|---|
| Project mission | .harness/spec.md |
| Project state | .harness/progress.md |
| Brownfield context | .harness/context/*.md |
| Feature spec | .harness/contracts/sprint-NNN.md |
| Design | .harness/design/sprint-NNN.md |
| Tasks | .harness/tasks/sprint-NNN.md |
| Validation report | .harness/reports/latest.json |
| Human-readable evaluation | .harness/evaluations/sprint-NNN.md |
| Repair instructions | .harness/repairs/latest.md |

Do not create .specs/ by default. A future compatibility export can mirror
Harness artifacts into .specs/ if a team explicitly wants that.
`

const contextStackTemplate = `# Stack Context

Record stable stack facts that agents should not rediscover every sprint.

- Runtime:
- Package manager:
- Framework:
- Test runner:
- E2E runner:
- Database/backend:
- Important scripts:
`

const contextArchitectureTemplate = `# Architecture Context

Record module boundaries, routing, data flow, and important architectural
constraints for this project.

- Modules:
- Boundaries:
- Forbidden shortcuts:
- Data flow:
- Security boundaries:
`

const contextConventionsTemplate = `# Conventions Context

Record project conventions agents should follow.

- Naming:
- Folder layout:
- UI/component style:
- Error handling:
- Form patterns:
- Testing patterns:
`

const contextTestingTemplate = `# Testing Context

Record test commands and expectations.

- Lint:
- Typecheck:
- Unit tests:
- Coverage:
- E2E:
- Fixtures:
- Known slow/flaky tests:
`

const contextIntegrationsTemplate = `# Integrations Context

Record external systems and local integration rules.

- Auth:
- Database:
- Storage:
- Payments:
- Email/notifications:
- External APIs:
- Secrets policy:
`

const contextConcernsTemplate = `# Concerns Context

Record recurring risks and fragile areas.

- Known risks:
- Recurring findings:
- Performance concerns:
- Security concerns:
- UX concerns:
- Migration concerns:
`
