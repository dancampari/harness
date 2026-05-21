package evaluator

import (
	"context"
	"testing"

	"github.com/dancampari/harness/internal/config"
	"github.com/dancampari/harness/internal/sensors"
)

func TestEvaluateFastSkipsSlowDimensions(t *testing.T) {
	cfg := config.Config{
		Adapters: config.AdaptersConfig{
			Lint:     []string{"eslint"},
			Test:     []string{"jest"},
			Coverage: []string{"jest-coverage"},
		},
		Thresholds: config.ThresholdsConfig{
			Correctness: 80,
			Coverage:    70,
			Contract:    80,
		},
		Weights: config.DimensionWeights{
			Correctness: 50,
			Coverage:    25,
			Contract:    25,
		},
	}
	registry := sensors.NewRegistry()
	registry.Register(fakeSensor{name: "eslint", dimension: sensors.DimCorrectness, available: true, score: 90})
	registry.Register(fakeSensor{name: "jest", dimension: sensors.DimCorrectness, available: true, score: 90})
	registry.Register(fakeSensor{name: "jest-coverage", dimension: sensors.DimCoverage, available: true, score: 90})

	ev := New(cfg, registry)
	check := ContractCheckResult{Status: "satisfied", Score: 100}

	result, err := ev.EvaluateWith(context.Background(), t.TempDir(), 1, check, Options{Fast: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Verdict != "PASS" {
		t.Fatalf("expected PASS in fast mode, got %s", result.Verdict)
	}
	covDim, ok := result.Dimensions["coverage"]
	if !ok {
		t.Fatal("expected coverage dimension to be reported as skipped")
	}
	if !covDim.Skipped {
		t.Fatalf("expected coverage to be Skipped in fast mode, got %+v", covDim)
	}
	if !covDim.Passed {
		t.Fatal("skipped dimensions must not block the verdict")
	}
	corrDim, ok := result.Dimensions["correctness"]
	if !ok {
		t.Fatal("expected correctness dimension to be present")
	}
	if corrDim.Skipped {
		t.Fatal("correctness has a fast sensor (eslint) so it must run, not be skipped")
	}
}

func TestEvaluateFastSkipsDimensionWithoutFastSensor(t *testing.T) {
	cfg := config.Config{
		Adapters: config.AdaptersConfig{
			Security: []string{"npm-audit"}, // slow only
		},
		Thresholds: config.ThresholdsConfig{
			Security: 85,
			Contract: 80,
		},
		Weights: config.DimensionWeights{
			Security: 50,
			Contract: 50,
		},
	}
	registry := sensors.NewRegistry()
	registry.Register(fakeSensor{name: "npm-audit", dimension: sensors.DimSecurity, available: true, score: 100})

	ev := New(cfg, registry)
	check := ContractCheckResult{Status: "satisfied", Score: 100}

	result, err := ev.EvaluateWith(context.Background(), t.TempDir(), 1, check, Options{Fast: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Verdict != "PASS" {
		t.Fatalf("expected PASS in fast mode (security skipped), got %s", result.Verdict)
	}
	secDim := result.Dimensions["security"]
	if !secDim.Skipped {
		t.Fatalf("expected security to be skipped in fast mode, got %+v", secDim)
	}
}

func TestEvaluateFastWithAuditsRunsAuditSensors(t *testing.T) {
	cfg := config.Config{
		Adapters: config.AdaptersConfig{
			Lint:     []string{"eslint"},
			Security: []string{"npm-audit"},
		},
		Thresholds: config.ThresholdsConfig{
			Correctness: 80,
			Security:    85,
			Contract:    80,
		},
		Weights: config.DimensionWeights{
			Correctness: 40,
			Security:    30,
			Contract:    30,
		},
	}
	registry := sensors.NewRegistry()
	registry.Register(fakeSensor{name: "eslint", dimension: sensors.DimCorrectness, available: true, score: 100})
	registry.Register(fakeSensor{name: "npm-audit", dimension: sensors.DimSecurity, available: true, score: 100})

	ev := New(cfg, registry)
	check := ContractCheckResult{Status: "satisfied", Score: 100}

	result, err := ev.EvaluateWith(context.Background(), t.TempDir(), 1, check, Options{Fast: true, IncludeAudits: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Verdict != "PASS" {
		t.Fatalf("expected PASS with audits enabled, got %s", result.Verdict)
	}
	secDim := result.Dimensions["security"]
	if secDim.Skipped {
		t.Fatal("security must run when IncludeAudits=true; it is the centerpiece of the watch loop")
	}
	if secDim.Score != 100 {
		t.Fatalf("expected security score 100, got %d", secDim.Score)
	}
}

func TestEvaluateFastSkipsReviewDimension(t *testing.T) {
	cfg := config.Config{
		Adapters: config.AdaptersConfig{
			Lint:   []string{"eslint"},
			Review: []string{"external-reviewer"},
		},
		Thresholds: config.ThresholdsConfig{
			Correctness: 80,
			Review:      70,
			Contract:    80,
		},
		Weights: config.DimensionWeights{
			Correctness: 40,
			Review:      20,
			Contract:    40,
		},
	}
	registry := sensors.NewRegistry()
	registry.Register(fakeSensor{name: "eslint", dimension: sensors.DimCorrectness, available: true, score: 100})
	registry.Register(fakeSensor{name: "external-reviewer", dimension: sensors.DimReview, available: true, score: 100})

	ev := New(cfg, registry)
	check := ContractCheckResult{Status: "satisfied", Score: 100}

	result, err := ev.EvaluateWith(context.Background(), t.TempDir(), 1, check, Options{Fast: true})
	if err != nil {
		t.Fatal(err)
	}
	reviewDim, ok := result.Dimensions["review"]
	if !ok {
		t.Fatal("expected review dimension to appear (even if skipped) when active")
	}
	if !reviewDim.Skipped {
		t.Fatalf("expected review dim to be Skipped in fast mode (LLM-backed), got %+v", reviewDim)
	}
	if result.Verdict != "PASS" {
		t.Fatalf("expected PASS when review is skipped, got %s", result.Verdict)
	}
}

func TestEvaluateSkipContractRemovesContractDimension(t *testing.T) {
	cfg := config.Config{
		Adapters: config.AdaptersConfig{
			Lint: []string{"eslint"},
		},
		Thresholds: config.ThresholdsConfig{
			Correctness: 80,
			Contract:    80,
		},
		Weights: config.DimensionWeights{
			Correctness: 100,
			Contract:    50, // still active in config, but watch skips it
		},
	}
	registry := sensors.NewRegistry()
	registry.Register(fakeSensor{name: "eslint", dimension: sensors.DimCorrectness, available: true, score: 100})

	ev := New(cfg, registry)
	check := ContractCheckResult{Status: "skipped"}

	result, err := ev.EvaluateWith(context.Background(), t.TempDir(), 0, check, Options{Fast: true, SkipContract: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := result.Dimensions["contract"]; ok {
		t.Fatalf("expected contract dimension to be absent when SkipContract=true, got %+v", result.Dimensions["contract"])
	}
}

func TestEvaluateFastWeightedTotalIgnoresSkippedDimensions(t *testing.T) {
	cfg := config.Config{
		Adapters: config.AdaptersConfig{
			Lint:     []string{"eslint"},
			Coverage: []string{"jest-coverage"},
		},
		Thresholds: config.ThresholdsConfig{
			Correctness: 80,
			Coverage:    70,
			Contract:    80,
		},
		Weights: config.DimensionWeights{
			Correctness: 50,
			Coverage:    25,
			Contract:    25,
		},
	}
	registry := sensors.NewRegistry()
	registry.Register(fakeSensor{name: "eslint", dimension: sensors.DimCorrectness, available: true, score: 100})
	registry.Register(fakeSensor{name: "jest-coverage", dimension: sensors.DimCoverage, available: true, score: 0})

	ev := New(cfg, registry)
	check := ContractCheckResult{Status: "satisfied", Score: 100}

	result, err := ev.EvaluateWith(context.Background(), t.TempDir(), 1, check, Options{Fast: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalScore == 0 {
		t.Fatal("expected total score to be derived from non-skipped dimensions only")
	}
	if result.TotalScore < 100 {
		t.Fatalf("eslint = 100 + contract = 100 (coverage skipped) should average to 100, got %d", result.TotalScore)
	}
}
