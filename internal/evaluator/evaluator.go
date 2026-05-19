// Package evaluator is the QA brain. It runs deterministic sensors in an
// isolated subprocess and aggregates their findings into strict dimensions.
package evaluator

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dancampari/harness/internal/config"
	"github.com/dancampari/harness/internal/sensors"
)

// Evaluator runs sensors and aggregates results. Stateless: a new instance per
// run keeps concurrency simple and makes subprocess isolation easier to reason
// about.
type Evaluator struct {
	cfg      config.Config
	registry *sensors.Registry
}

// New creates an evaluator with the given config and sensor registry.
func New(cfg config.Config, reg *sensors.Registry) *Evaluator {
	return &Evaluator{cfg: cfg, registry: reg}
}

// DimensionScore aggregates findings for a single quality dimension.
type DimensionScore struct {
	Dimension   sensors.Dimension `json:"dimension"`
	Score       int               `json:"score"`
	Threshold   int               `json:"threshold"`
	Passed      bool              `json:"passed"`
	Findings    []sensors.Finding `json:"findings"`
	SensorsUsed []string          `json:"sensors_used"`
}

// SensorStatus records the execution state of each configured sensor.
type SensorStatus struct {
	Name            string  `json:"name"`
	Dimension       string  `json:"dimension"`
	Registered      bool    `json:"registered"`
	Available       bool    `json:"available"`
	Executed        bool    `json:"executed"`
	Error           string  `json:"error,omitempty"`
	DurationSeconds float64 `json:"duration_seconds,omitempty"`
}

// EvaluationResult is the complete aggregated output of a QA run.
type EvaluationResult struct {
	SchemaVersion   string                    `json:"schema_version"`
	Timestamp       time.Time                 `json:"timestamp"`
	SprintNumber    int                       `json:"sprint_number"`
	TotalScore      int                       `json:"total_score"`
	Verdict         string                    `json:"verdict"` // PASS | FAIL
	Dimensions      map[string]DimensionScore `json:"dimensions"`
	Sensors         []SensorStatus            `json:"sensors"`
	Process         ProcessInfo               `json:"process"`
	ContractCheck   ContractCheckResult       `json:"contract_check"`
	DurationSeconds float64                   `json:"duration_seconds"`
}

type ProcessInfo struct {
	PID                  int  `json:"pid"`
	ParentPID            int  `json:"parent_pid"`
	Isolated             bool `json:"isolated"`
	ContextEnvStripped   bool `json:"context_env_stripped"`
	AcceptingScreenshots bool `json:"accepting_screenshots"`
}

// ContractCheckResult records whether the sprint's contract was satisfied.
// Set by the contract validator, not by sensors.
type ContractCheckResult struct {
	Status              string   `json:"status"` // satisfied | partial | violated | missing
	MissingDeliverables []string `json:"missing_deliverables,omitempty"`
	UnmetCriteria       []string `json:"unmet_criteria,omitempty"`
	Score               int      `json:"score"`
}

// Evaluate runs configured sensors against root and aggregates results.
func (e *Evaluator) Evaluate(ctx context.Context, root string, sprintNum int,
	contractCheck ContractCheckResult) (*EvaluationResult, error) {

	start := time.Now()
	configured := e.configuredSensors(root)
	results := make([]sensors.Result, len(configured.toRun))

	var wg sync.WaitGroup
	for i, s := range configured.toRun {
		wg.Add(1)
		go func(i int, s sensors.Sensor) {
			defer wg.Done()
			results[i] = s.Run(ctx, root)
		}(i, s)
	}
	wg.Wait()

	for _, r := range results {
		if idx, ok := configured.statusIndex[r.SensorName]; ok {
			configured.statuses[idx].Executed = true
			configured.statuses[idx].Error = r.Error
			configured.statuses[idx].DurationSeconds = r.Duration.Seconds()
			if r.ToolMissing {
				configured.statuses[idx].Available = false
			}
		}
	}

	dims := aggregate(results, e.cfg.Thresholds)
	active := e.cfg.ActiveDimensions()
	if isActive(active, config.DimContract) {
		dims[string(sensors.DimContract)] = DimensionScore{
			Dimension:   sensors.DimContract,
			Score:       contractCheck.Score,
			Threshold:   e.cfg.Thresholds.Contract,
			Passed:      contractCheck.Score >= e.cfg.Thresholds.Contract,
			SensorsUsed: []string{"contract-validator"},
		}
	}

	executedByDim := map[string]int{}
	for _, r := range results {
		if !r.ToolMissing {
			executedByDim[string(r.Dimension)]++
		}
	}
	for _, dim := range active {
		if dim == config.DimContract {
			continue
		}
		if executedByDim[dim] > 0 {
			if _, ok := dims[dim]; !ok {
				dims[dim] = emptyExecutedDimensionScore(dim, e.cfg.ThresholdFor(dim))
			}
			continue
		}
		dims[dim] = missingSensorDimensionScore(dim, e.cfg.ThresholdFor(dim), e.cfg.AdapterNamesForDimension(dim))
	}

	total := weightedTotal(dims, e.cfg.Weights)
	verdict := "PASS"
	for _, dim := range active {
		if d, ok := dims[dim]; !ok || !d.Passed {
			verdict = "FAIL"
			break
		}
	}

	return &EvaluationResult{
		SchemaVersion:   "2",
		Timestamp:       time.Now().UTC(),
		SprintNumber:    sprintNum,
		TotalScore:      total,
		Verdict:         verdict,
		Dimensions:      dims,
		Sensors:         configured.statuses,
		Process:         processInfo(),
		ContractCheck:   contractCheck,
		DurationSeconds: time.Since(start).Seconds(),
	}, nil
}

func processInfo() ProcessInfo {
	return ProcessInfo{
		PID:                  os.Getpid(),
		ParentPID:            os.Getppid(),
		Isolated:             os.Getenv("HARNESS_ISOLATED") == "1",
		ContextEnvStripped:   os.Getenv("CLAUDE_SESSION_TOKEN") == "" && os.Getenv("CODEX_SESSION_TOKEN") == "" && os.Getenv("CURSOR_TRACE_ID") == "",
		AcceptingScreenshots: os.Getenv("HARNESS_ACCEPT_SCREENSHOTS") == "1",
	}
}

type configuredSensors struct {
	toRun       []sensors.Sensor
	statuses    []SensorStatus
	statusIndex map[string]int
}

func (e *Evaluator) configuredSensors(root string) configuredSensors {
	out := configuredSensors{statusIndex: map[string]int{}}
	nameToDim := map[string]string{}
	for _, dim := range []string{
		config.DimCorrectness,
		config.DimCoverage,
		config.DimComplexity,
		config.DimSecurity,
		config.DimArchitecture,
		config.DimE2E,
	} {
		for _, name := range e.cfg.AdapterNamesForDimension(dim) {
			if name != "" {
				nameToDim[name] = dim
			}
		}
	}

	for _, name := range e.cfg.AllAdapterNames() {
		st := SensorStatus{Name: name, Dimension: nameToDim[name]}
		if s, ok := e.registry.ByName(name); ok {
			st.Registered = true
			st.Dimension = string(s.Dimension())
			st.Available = s.Available(root)
			if st.Available {
				out.toRun = append(out.toRun, s)
			}
		} else {
			st.Error = "sensor is configured but not registered in this harness binary"
		}
		out.statusIndex[name] = len(out.statuses)
		out.statuses = append(out.statuses, st)
	}
	return out
}

// aggregate groups sensor results by dimension and averages their scores.
func aggregate(results []sensors.Result, thresholds config.ThresholdsConfig) map[string]DimensionScore {
	type bucket struct {
		scores   []int
		findings []sensors.Finding
		sensors  []string
	}
	buckets := map[sensors.Dimension]*bucket{}
	for _, r := range results {
		b, ok := buckets[r.Dimension]
		if !ok {
			b = &bucket{}
			buckets[r.Dimension] = b
		}
		if !r.ToolMissing {
			b.scores = append(b.scores, r.RawScore)
			b.sensors = append(b.sensors, r.SensorName)
		}
		for _, f := range r.Findings {
			target, ok := buckets[f.Dimension]
			if !ok {
				target = &bucket{}
				buckets[f.Dimension] = target
			}
			target.findings = append(target.findings, f)
		}
	}

	out := map[string]DimensionScore{}
	for dim, b := range buckets {
		score := avgOrZero(b.scores)
		th := thresholdOf(thresholds, dim)
		sort.SliceStable(b.findings, func(i, j int) bool {
			return severityRank(b.findings[i].Severity) > severityRank(b.findings[j].Severity)
		})
		out[string(dim)] = DimensionScore{
			Dimension:   dim,
			Score:       score,
			Threshold:   th,
			Passed:      score >= th,
			Findings:    b.findings,
			SensorsUsed: b.sensors,
		}
	}
	return out
}

func thresholdOf(thresholds config.ThresholdsConfig, d sensors.Dimension) int {
	switch d {
	case sensors.DimCorrectness:
		return thresholds.Correctness
	case sensors.DimCoverage:
		return thresholds.Coverage
	case sensors.DimComplexity:
		return thresholds.Complexity
	case sensors.DimSecurity:
		return thresholds.Security
	case sensors.DimArchitecture:
		return thresholds.Architecture
	case sensors.DimContract:
		return thresholds.Contract
	case sensors.DimE2E:
		return thresholds.E2E
	}
	return 70
}

func missingSensorDimensionScore(dim string, threshold int, expected []string) DimensionScore {
	msg := "no configured sensor executed for active dimension"
	if len(expected) > 0 {
		msg = fmt.Sprintf("no configured sensor executed for active dimension; expected one of: %s",
			strings.Join(expected, ", "))
	}
	f := sensors.Finding{
		Dimension: sensors.Dimension(dim),
		Severity:  sensors.SeverityCritical,
		Rule:      "missing-sensor",
		Message:   msg,
	}
	f.Fingerprint = sensors.Fingerprint(f.Dimension, "", f.Rule, f.Message)
	return DimensionScore{
		Dimension: sensors.Dimension(dim),
		Score:     0,
		Threshold: threshold,
		Passed:    false,
		Findings:  []sensors.Finding{f},
	}
}

func emptyExecutedDimensionScore(dim string, threshold int) DimensionScore {
	return DimensionScore{
		Dimension: sensors.Dimension(dim),
		Score:     0,
		Threshold: threshold,
		Passed:    0 >= threshold,
	}
}

func avgOrZero(xs []int) int {
	if len(xs) == 0 {
		return 0
	}
	sum := 0
	for _, x := range xs {
		sum += x
	}
	return sum / len(xs)
}

func severityRank(s sensors.Severity) int {
	switch s {
	case sensors.SeverityCritical:
		return 4
	case sensors.SeverityHigh:
		return 3
	case sensors.SeverityMedium:
		return 2
	case sensors.SeverityLow:
		return 1
	}
	return 0
}

func weightedTotal(dims map[string]DimensionScore, w config.DimensionWeights) int {
	weights := map[string]int{
		config.DimCorrectness:  w.Correctness,
		config.DimCoverage:     w.Coverage,
		config.DimComplexity:   w.Complexity,
		config.DimSecurity:     w.Security,
		config.DimArchitecture: w.Architecture,
		config.DimContract:     w.Contract,
		config.DimE2E:          w.E2E,
	}
	total := 0
	totalWeight := 0
	for name, d := range dims {
		wt := weights[name]
		total += d.Score * wt
		totalWeight += wt
	}
	if totalWeight == 0 {
		return 0
	}
	return total / totalWeight
}

func isActive(active []string, dim string) bool {
	for _, d := range active {
		if d == dim {
			return true
		}
	}
	return false
}
