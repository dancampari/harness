# Multi-Agent Protocol

> Canonical reference for the harness's sub-agent delegation contract.
> Generated AGENTS.md / CLAUDE.md files embed a compact version of this;
> this document is the full specification.

The harness assumes the coding CLI (Codex, Claude Code, Cursor) is the
orchestrator. It plans the work, holds the spec/contract context, calls
the harness CLI, and decides when to spawn a sub-agent. The harness
itself never spawns agents — it observes the events the agent emits and
gates state transitions.

The delegation matrix and the knowledge-verification chain below are
copied verbatim into every generated AGENTS.md / CLAUDE.md so the agent
sees them at session start without depending on this doc.

## Delegation matrix

From TLC `SKILL.md` (lines 122–131):

| Activity                                   | Delegate? | Why                                                                  |
|--------------------------------------------|-----------|----------------------------------------------------------------------|
| Research / brownfield mapping              | Yes       | Output is large; only the summary matters to the main context.        |
| Implementing one task from `tasks.md`      | Yes       | Edits + test output consume context; only the result matters.         |
| Parallel `[P]` tasks                       | Yes (one per task) | The only way to actually run tasks in parallel.             |
| Sequential tasks (no `[P]`)                | Yes       | Keeps implementation artifacts out of the main context.               |
| Planning, task creation, validation        | No        | Requires the full accumulated context to stay coherent.               |
| Quick mode (`harness quick "<one-line>"`)  | No        | Too small to justify the sub-agent overhead.                          |

### Sub-agent personas the harness ships

When the project runs `harness setup --planning spec-driven`, the
harness installs five sub-agent definitions for the coding CLI:

| Persona                  | Codex name                     | Claude name                  | Maps to TLC row                  |
|--------------------------|--------------------------------|------------------------------|----------------------------------|
| Researcher               | `harness_researcher`           | `harness-researcher`         | Research / brownfield mapping    |
| Spec planner             | `harness_spec_planner`         | `harness-spec-planner`       | Specify / Design / Tasks         |
| Contract author          | `harness_contract_author`      | `harness-contract-author`    | Contract creation / repair       |
| Contract reviewer        | `harness_contract_reviewer`    | `harness-contract-reviewer`  | Tester approval / rejection      |
| Task worker              | `harness_task_worker`          | `harness-task-worker`        | Implement one task               |

The orchestrator (the coding CLI itself) stays in the **main context**
for planning, validation reports, and quick mode — the matrix's "No"
rows. Sub-agents are short-lived: one task, return, free their context.

### Context the orchestrator hands a sub-agent

Sub-agents MUST receive only:

- the specific task definition from `tasks.md` (What / Where / Depends
  on / Reuses / Done when / Tests / Gate)
- `.specs/codebase/CONVENTIONS.md` and any coding-principles document
- `.specs/codebase/TESTING.md` if it exists (test patterns and gate
  check commands)
- the spec/design sections the task references

Sub-agents MUST NOT receive: other tasks' definitions, accumulated chat
history, `.specs/project/STATE.md` (unless the task explicitly
references a decision/blocker), or validation reports from other tasks.

### What sub-agents return

Each sub-agent reports back with:

- **Status**: Complete | Blocked | Partial
- **Files changed**: list of relative paths
- **Gate check result**: PASS / FAIL plus test counts
- **SPEC_DEVIATION markers**: any new markers introduced
- **Issues encountered**: short bullet list

The orchestrator uses that to update `tasks.md`, the requirement
traceability state, and decide the next step.

## Events that record delegation

The agent emits these event types into `.harness/events.jsonl` so the
live TUI and trend tooling can render the multi-agent flow:

| Event                          | Phase    | Emitted when                                              |
|--------------------------------|----------|-----------------------------------------------------------|
| `agent.delegate.research`      | Contract | Orchestrator spawns a researcher sub-agent                |
| `agent.delegate.implement`     | Build    | Orchestrator spawns an implementation sub-agent           |
| `agent.delegate.parallel`      | Build    | Orchestrator spawns a parallel `[P]` sub-agent batch      |
| `agent.subagent.done`          | Build    | A sub-agent returned (carry Status + files-changed count) |
| `verification.codebase`        | Contract | Knowledge chain step 1 consulted                          |
| `verification.docs`            | Contract | Knowledge chain step 2 consulted                          |
| `verification.context7`        | Contract | Knowledge chain step 3 consulted                          |
| `verification.web`             | Contract | Knowledge chain step 4 consulted                          |
| `verification.uncertain`       | Contract | Chain exhausted without a confident answer                |

Events are advisory — the harness never blocks on their absence. They
exist so the human reviewer can see what was actually checked.

## Knowledge Verification Chain

When the orchestrator (or a sub-agent) needs information not already in
context, follow this order. Stop as soon as a step yields a confident
answer; emit the matching event before moving to the next step.

1. **Codebase** — read the source files first. Emit
   `verification.codebase`.
2. **Project docs** — read `.specs/codebase/*.md` and
   `.specs/project/STATE.md`. Emit `verification.docs`.
3. **Context7 MCP** — query for upstream library / API behaviour. Emit
   `verification.context7`.
4. **Web** — search only as a last resort, prefer first-party sources.
   Emit `verification.web`.
5. **Uncertain** — if the chain ends without confidence, emit
   `verification.uncertain` and ask the user. Do NOT invent.

Inventing facts is a contract violation. The chain exists so the agent
demonstrates *what it checked* before answering.

## Failure handling

When a sub-agent reports **Blocked** or **Partial**, the orchestrator
must:

1. Record a `state record blocker` entry describing what is missing.
2. Decide between (a) re-spawning the sub-agent with extra context, (b)
   splitting the task further in `tasks.md`, or (c) escalating to the
   user.

Never re-spawn a sub-agent with the same context expecting a different
result. If the first attempt was Blocked, the orchestrator owes the
sub-agent more context or a smaller scope.
