package evaluator

import (
	"context"
	"testing"
	"time"

	"github.com/dancampari/harness/internal/config"
	"github.com/dancampari/harness/internal/sensors"
)

type fakeSensor struct {
	name      string
	dimension sensors.Dimension
	available bool
	score     int
	findings  []sensors.Finding
}

func (f fakeSensor) Name() string                 { return f.name }
func (f fakeSensor) Dimension() sensors.Dimension { return f.dimension }
func (f fakeSensor) Available(root string) bool   { return f.available }
func (f fakeSensor) Run(ctx context.Context, root string) sensors.Result {
	return sensors.Result{
		SensorName: f.name,
		Dimension:  f.dimension,
		RawScore:   f.score,
		Findings:   f.findings,
		Duration:   time.Millisecond,
	}
}

func TestEvaluateFailsActiveDimensionWithoutExecutedSensor(t *testing.T) {
	cfg := config.DefaultFor("unknown")
	cfg.Thresholds.Correctness = 80
	cfg.Weights.Correctness = 50
	cfg.Thresholds.Contract = 80
	cfg.Weights.Contract = 50
	cfg.Adapters.Lint = []string{"eslint"}

	reg := sensors.NewRegistry()
	reg.Register(fakeSensor{name: "eslint", dimension: sensors.DimCorrectness, available: false, score: 100})

	ev := New(cfg, reg)
	result, err := ev.Evaluate(context.Background(), t.TempDir(), 1, ContractCheckResult{
		Status: "satisfied",
		Score:  100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Verdict != "FAIL" {
		t.Fatalf("expected FAIL, got %s", result.Verdict)
	}
	correctness := result.Dimensions[config.DimCorrectness]
	if correctness.Score != 0 || correctness.Passed {
		t.Fatalf("expected missing sensor to score 0 and fail, got %+v", correctness)
	}
	if len(correctness.Findings) != 1 || correctness.Findings[0].Rule != "missing-sensor" {
		t.Fatalf("expected missing-sensor finding, got %+v", correctness.Findings)
	}
}

func TestEvaluatePassesOnlyWhenAllActiveDimensionsPass(t *testing.T) {
	cfg := config.DefaultFor("unknown")
	cfg.Thresholds.Correctness = 80
	cfg.Weights.Correctness = 50
	cfg.Thresholds.Contract = 80
	cfg.Weights.Contract = 50
	cfg.Adapters.Lint = []string{"eslint"}

	reg := sensors.NewRegistry()
	reg.Register(fakeSensor{name: "eslint", dimension: sensors.DimCorrectness, available: true, score: 100})

	ev := New(cfg, reg)
	result, err := ev.Evaluate(context.Background(), t.TempDir(), 1, ContractCheckResult{
		Status: "satisfied",
		Score:  100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Verdict != "PASS" {
		t.Fatalf("expected PASS, got %s", result.Verdict)
	}
	if result.TotalScore != 100 {
		t.Fatalf("expected total score 100, got %d", result.TotalScore)
	}
	if len(result.Sensors) != 1 || !result.Sensors[0].Executed {
		t.Fatalf("expected executed sensor status, got %+v", result.Sensors)
	}
}

func TestEvaluateHardFailsBlockingContractFinding(t *testing.T) {
	cfg := config.DefaultFor("unknown")
	reg := sensors.NewRegistry()
	reg.Register(fakeSensor{
		name:      "spec-deviation-scanner",
		dimension: sensors.DimContract,
		available: true,
		score:     100,
		findings: []sensors.Finding{{
			Dimension: sensors.DimContract,
			Severity:  sensors.SeverityMedium,
			Rule:      "spec-deviation-without-reason",
			Message:   "orphan SPEC_DEVIATION marker",
		}},
	})

	ev := New(cfg, reg)
	result, err := ev.Evaluate(context.Background(), t.TempDir(), 1, ContractCheckResult{
		Status: "satisfied",
		Score:  100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Verdict != "FAIL" {
		t.Fatalf("expected FAIL for blocking contract finding, got %s", result.Verdict)
	}
	contract := result.Dimensions[config.DimContract]
	if contract.Score != 0 || contract.Passed {
		t.Fatalf("expected contract hard-fail score 0, got %+v", contract)
	}
}
