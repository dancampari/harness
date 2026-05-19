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

func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
