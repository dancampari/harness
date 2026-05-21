# Inferential Reviewer (External Adapter)

Harness's deterministic sensors cover lint, tests, coverage, complexity,
architecture, audits, contract structure, and e2e. They do not assess
**meaning**. The inferential-review adapter plugs a user-controlled
LLM-backed reviewer into the same Evaluator loop so Harness can also
report on intent: did the implementation actually deliver what each
REQ-ID promised?

The harness binary never embeds a model. The adapter is a generic
shell-out: it runs a configured external CLI and parses a structured
JSON response. Anything that satisfies the I/O contract below works —
Claude Code with a custom subagent, Codex with a custom agent, a Python
script calling the Anthropic SDK, a bash wrapper around `ollama`, etc.

## Status

Optional and disabled by default. Enabling it adds an extra QA latency
cost (one LLM call per `harness sprint qa`, typically 30s–10min). The
adapter is automatically excluded from `harness sprint qa --fast` and
`harness watch`, so pre-commit and the drift watch loop never pay the
inferential cost.

## Configuration

Add four blocks to `.harness/config.yaml`:

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

The review dimension follows the same active-or-disabled rule as every
other Harness dimension: both `thresholds.review` and `weights.review`
must be greater than zero, otherwise the dimension is silently disabled.

## Invocation Contract

When the dimension is active, the Evaluator invokes
`review.command` as a subprocess with:

- **stdin**: a JSON bundle (see Input Schema below).
- **stdout**: a JSON findings document (see Output Schema below).
- **stderr**: free-form diagnostics for the human; not parsed by
  Harness.
- **cwd**: the repository root, not `.harness/`.
- **timeout**: capped at `review.timeout_seconds` (default 10min).

A non-zero exit code is reported as a sensor error and the dimension
fails for that QA pass.

## Input Schema (Harness → Reviewer)

```json
{
  "schema_version": "1",
  "harness_dir": "/abs/path/to/.harness",
  "repo_root": "/abs/path/to/repo",
  "contract_path": "/abs/path/to/.harness/contracts/sprint-007.md",
  "contract_md": "# Sprint 007 — ...\n\n## Goal\n..."
}
```

The reviewer is responsible for any additional context it needs. The
`harness_dir` lets the reviewer read `progress.md`, `spec.md`,
`reports/latest.json`, and the previous repair brief. The `repo_root`
lets it inspect product files directly.

`contract_path` and `contract_md` are empty when there is no sprint
contract yet (the reviewer can do general project review in that case).

## Output Schema (Reviewer → Harness)

```json
{
  "schema_version": "1",
  "findings": [
    {
      "requirement_id": "REQ-001",
      "severity": "high",
      "rule": "missing-guard",
      "file": "src/auth.ts",
      "line": 12,
      "message": "Admin route lacks the required role check.",
      "suggestion": "Wrap the handler with requireRole(admin) middleware."
    }
  ]
}
```

Field semantics:

- `requirement_id` (optional): when present, Harness prefixes the
  finding message with `[REQ-001]` so the agent can trace back which
  acceptance criterion the issue maps to.
- `severity`: one of `critical`, `high`, `medium`, `low`, `info`.
  Defaults to `medium` if missing or unrecognised.
- `rule` (optional): short stable identifier for fingerprinting. Falls
  back to `external-reviewer` so recurrence still works.
- `file`, `line` (optional): location to surface in the repair brief
  and TUI.
- `message`: human-readable description of what is wrong.
- `suggestion` (optional): becomes the Finding's `Hint` field, which
  the repair brief renders under "Suggested Fixes (LLM-optimized)".

## Scoring

- Zero findings → `RawScore: 100`.
- N findings → `RawScore: max(0, 100 - 10*N)`.

Severity affects sort order in the repair brief but not the score, so
the formula stays transparent. Adjust `thresholds.review` and
`weights.review` to control how heavily the review dimension counts
toward the overall verdict.

## Example Reviewer Script

Minimal Python wrapper around the Anthropic SDK (pseudocode):

```python
#!/usr/bin/env python3
import json, sys
from anthropic import Anthropic

bundle = json.loads(sys.stdin.read())
client = Anthropic()

prompt = f"""You are reviewing a sprint implementation against its
contract. Return only JSON in this shape:
{{ "schema_version": "1", "findings": [...] }}

Contract:
{bundle['contract_md']}

Workspace: {bundle['repo_root']}
"""

reply = client.messages.create(
    model="claude-opus-4-7",
    max_tokens=4096,
    messages=[{"role": "user", "content": prompt}],
)
sys.stdout.write(reply.content[0].text)
```

For Claude Code or Codex, configure a custom agent that emits the same
JSON shape and point `review.command` at the agent invocation.

## Disabling

Set either `thresholds.review` or `weights.review` to `0` (or remove
both fields). The adapter stays registered but Harness stops running
it. The default Harness install ships with these set to zero, so no
LLM call ever happens unless the user opts in.

## Why an External Adapter?

The harness binary is intentionally deterministic and offline. Embedding
an SDK would couple the framework to one provider's auth, pricing, and
release cadence. A shell-out adapter keeps the inferential loop
optional, swappable, and bring-your-own-key.

It also keeps the trust boundary clean: when a reviewer flags an issue,
the deterministic sensors are unaffected, so a hallucinated finding
cannot silently lower the overall verdict beyond its declared weight.
