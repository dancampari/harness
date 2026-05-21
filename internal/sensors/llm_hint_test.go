package sensors

import (
	"strings"
	"testing"
)

func TestLLMHintReturnsEmptyForUnknownRule(t *testing.T) {
	if hint := LLMHint("rule-that-does-not-exist"); hint != "" {
		t.Fatalf("expected empty hint for unknown rule, got %q", hint)
	}
}

func TestLLMHintReturnsRegisteredHints(t *testing.T) {
	for _, rule := range []string{"no-unused-vars", "complexity-too-high", "test-failure", "missing-sensor"} {
		hint := LLMHint(rule)
		if hint == "" {
			t.Fatalf("expected hint for well-known rule %q, got empty", rule)
		}
		if !strings.Contains(hint, "Suggested fix:") {
			t.Fatalf("hint for %q must begin with `Suggested fix:`, got %q", rule, hint)
		}
		if !strings.Contains(hint, "Do NOT") {
			t.Fatalf("hint for %q must include a `Do NOT` antipattern, got %q", rule, hint)
		}
	}
}

func TestEnrichFindingPopulatesHintField(t *testing.T) {
	f := Finding{
		Dimension: DimCorrectness,
		Severity:  SeverityMedium,
		Rule:      "no-unused-vars",
		Message:   "'foo' is defined but never used",
	}
	enriched := EnrichFinding(f)
	if enriched.Hint == "" {
		t.Fatal("expected EnrichFinding to populate Hint for a known rule")
	}
	if enriched.Message != f.Message {
		t.Fatalf("expected upstream Message to be preserved, got %q vs %q", enriched.Message, f.Message)
	}
}

func TestEnrichFindingIsIdempotentWhenHintAlreadySet(t *testing.T) {
	f := Finding{Rule: "no-unused-vars", Hint: "preset"}
	if got := EnrichFinding(f); got.Hint != "preset" {
		t.Fatalf("expected preset Hint to win, got %q", got.Hint)
	}
}

func TestEnrichFindingLeavesUnknownRulesAlone(t *testing.T) {
	f := Finding{Rule: "anything-bespoke"}
	if got := EnrichFinding(f); got.Hint != "" {
		t.Fatalf("expected empty Hint for unknown rule, got %q", got.Hint)
	}
}
