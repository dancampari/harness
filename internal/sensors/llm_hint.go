package sensors

// LLMHint catalog. Each entry is an agent-readable Suggested fix paired
// with one or more Do NOT antipatterns the LLM tends to reach for. This
// is what Martin Fowler calls "positive prompt injection": custom
// linter messages optimized for LLM consumption so the agent does not
// reflexively pick the laziest fix that silences the rule without
// addressing the underlying issue.
//
// Hints are advisory. Sensors keep their upstream message intact; the
// Hint is appended via the separate Finding.Hint field so TTY output
// stays compact for humans and JSON consumers (agents) get the full
// guidance.
//
// Adding a hint:
//
//	llmHints["my-rule"] = "Suggested fix: <do X>. Do NOT: <common bad fix>."
//
// Conventions:
//   - Lead with "Suggested fix:".
//   - Follow with one or more "Do NOT:" lines for the antipatterns that
//     would be wrong despite making the warning go away.
//   - Keep total length under ~280 chars so it fits in repair briefs
//     without dominating the table.
var llmHints = map[string]string{
	// Generic correctness
	"no-unused-vars": "Suggested fix: delete the declaration if it is unreachable, or reference it in an expression. Do NOT prefix with `_` unless the variable is required by a callback signature.",
	"no-explicit-any": "Suggested fix: replace `any` with a concrete type, an interface, or `unknown` plus a type guard. Do NOT cast to `any` to silence the rule.",
	"no-console": "Suggested fix: replace `console.log` with the project's structured logger. Do NOT delete the log if it carries operational signal; route it through the logger instead.",
	"prefer-const": "Suggested fix: change `let` to `const`. Do NOT reintroduce mutability elsewhere to keep the original `let`.",
	"no-undef": "Suggested fix: import the symbol or declare it. Do NOT add `// eslint-disable-line no-undef` to bypass it.",
	"no-await-in-loop": "Suggested fix: parallelise with Promise.all when iterations are independent. Do NOT add the disable comment if the loop genuinely should be sequential — keep it but document why.",
	"no-floating-promises": "Suggested fix: await the call or wire it through a deliberate `.catch(handler)`. Do NOT void the promise to silence the rule.",

	// Structural / complexity
	"complexity-too-high": "Suggested fix: extract sub-functions for nested logic, or replace nested branches with table-driven dispatch. Do NOT raise the threshold; complexity is a maintenance signal.",
	"function-too-long":   "Suggested fix: split the function along its existing scopes (auth, validation, persistence, return). Do NOT inline helpers just to reduce the line count.",
	"deep-nesting":        "Suggested fix: invert guard clauses (early return) to flatten the structure. Do NOT replace nested ifs with chained ternaries.",
	"circular-import":     "Suggested fix: break the cycle by extracting a shared interface or moving the depended-on symbol to a leaf module. Do NOT use dynamic imports to mask the cycle.",
	"forbidden-import":    "Suggested fix: respect the boundary; if cross-module access is genuinely needed, route through the public interface of the target module. Do NOT delete the architecture rule to make this import legal.",

	// Coverage / behavior
	"test-failure":              "Suggested fix: fix the failing test or the implementation under test. Do NOT delete the test or comment its assertion out.",
	"coverage-below-threshold":  "Suggested fix: add tests that exercise the uncovered branches reported by the coverage tool. Do NOT lower the coverage threshold.",
	"fixture-baseline-missing":  "Suggested fix: review the new fixture output with the user, then run `harness sprint qa --accept-fixtures` after approval. Do NOT accept silently.",
	"fixture-regression":        "Suggested fix: fix the regression in product code so the approved fixture passes again. Do NOT accept the new output without explicit user approval.",

	// E2E
	"e2e-failure":                "Suggested fix: fix the failing flow or selector reported by Playwright. Do NOT mark the spec as `.skip()` without recording the reason in the contract and getting tester approval.",
	"no-e2e-tests":               "Suggested fix: add Playwright coverage for the primary user flow described in the contract. Do NOT lower the E2E threshold to make QA pass without coverage.",
	"screenshot-baseline-missing": "Suggested fix: ask the user to review `.harness/screenshots/current/`. Only after approval, rerun with `harness sprint qa --accept-screenshots`. Do NOT accept blindly.",
	"visual-regression":          "Suggested fix: inspect `.harness/screenshots/diff/` and fix the UI change, or ask the user to approve the new baseline. Do NOT update the baseline silently.",

	// Contract / harness internals
	"missing-deliverable":  "Suggested fix: create the declared file at the contract path. Do NOT remove the deliverable line from the contract; if the file is no longer required, propose a contract revision and route through tester approval.",
	"unmet-criterion":      "Suggested fix: implement the behavior described, then point the Evidence cell at the test/e2e/fixture that proves it. Do NOT remove the criterion to clear the verdict.",
	"missing-sensor":       "Suggested fix: install the missing tool and configure it (run `harness doctor`). Do NOT lower the threshold or remove the sensor from `.harness/config.yaml` to make the dimension pass.",

	// Security
	"npm-audit":  "Suggested fix: upgrade the vulnerable dependency to a patched version, or remove it if unused. Do NOT pin the resolution to a vulnerable range to keep CI green.",
	"pip-audit":  "Suggested fix: upgrade the vulnerable dependency to a patched version. Do NOT add the CVE to an ignore list without recording the risk decision in `.harness/context/CONCERNS.md`.",
	"vulnerable-dependency": "Suggested fix: review the CVE, then upgrade or replace the dependency. Do NOT silence the audit without an explicit, time-bound exception recorded in the project's risk log.",
}

// LLMHint returns the agent-readable hint registered for a sensor
// rule, or empty when none exists. The empty case is normal: only
// well-known rules carry positive prompt injection.
func LLMHint(rule string) string {
	return llmHints[rule]
}

// EnrichFinding fills f.Hint from the LLMHint catalog when the finding's
// rule has a registered hint and Hint is not already set. Returns a
// shallow copy so callers can use the result without worrying about
// aliasing the input.
func EnrichFinding(f Finding) Finding {
	if f.Hint != "" {
		return f
	}
	if hint := LLMHint(f.Rule); hint != "" {
		f.Hint = hint
	}
	return f
}
