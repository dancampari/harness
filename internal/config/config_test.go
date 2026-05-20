package config

import "testing"

func TestValidateRejectsHalfEnabledDimension(t *testing.T) {
	cfg := DefaultFor("unknown")
	cfg.Thresholds.Coverage = 70
	cfg.Weights.Coverage = 0

	errs := cfg.Validate()
	if len(errs) == 0 {
		t.Fatal("expected validation error for half-enabled coverage dimension")
	}
}

func TestActiveDimensionsRequireThresholdAndWeight(t *testing.T) {
	cfg := DefaultFor("unknown")
	cfg.Thresholds.Correctness = 80
	cfg.Weights.Correctness = 20
	cfg.Thresholds.Coverage = 70
	cfg.Weights.Coverage = 0

	active := cfg.ActiveDimensions()
	if !contains(active, DimCorrectness) {
		t.Fatalf("expected correctness to be active, got %v", active)
	}
	if contains(active, DimCoverage) {
		t.Fatalf("expected coverage to be inactive until weight is set, got %v", active)
	}
}

func TestBroaderStackDefaultsConfigureRealAdapters(t *testing.T) {
	cases := map[string][]string{
		"python": {"ruff", "mypy", "pytest", "pytest-cov", "pip-audit"},
		"go":     {"go-vet", "staticcheck", "go-test", "go-test-coverage", "govulncheck"},
		"rust":   {"clippy", "cargo-test", "cargo-audit"},
	}
	for stack, adapters := range cases {
		cfg := DefaultFor(stack)
		if cfg.Stack != stack {
			t.Fatalf("%s: expected stack to be preserved, got %q", stack, cfg.Stack)
		}
		if len(cfg.Validate()) > 0 {
			t.Fatalf("%s: default config should validate, got %v", stack, cfg.Validate())
		}
		if !contains(cfg.ActiveDimensions(), DimCorrectness) ||
			!contains(cfg.ActiveDimensions(), DimSecurity) ||
			!contains(cfg.ActiveDimensions(), DimContract) {
			t.Fatalf("%s: expected correctness, security and contract dimensions, got %v", stack, cfg.ActiveDimensions())
		}
		for _, adapter := range adapters {
			if !contains(cfg.AllAdapterNames(), adapter) {
				t.Fatalf("%s: expected adapter %q in defaults, got %v", stack, adapter, cfg.AllAdapterNames())
			}
		}
	}
}

func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
