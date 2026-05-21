// Package sensors defines the contract that every tool adapter implements.
//
// A sensor is the harness's eye into one specific quality dimension.
// It does NOT execute LLMs. It runs deterministic tools (lint, type check,
// tests, coverage, AST analysis) and reports structured findings.
//
// Per problem 5 of the video ("Tudo no Mesmo Processo"), sensors run in
// isolated subprocesses spawned by the Evaluator — they never share
// context with the Builder that produced the code.
package sensors

import (
	"context"
	"time"
)

// Severity ranks finding importance. Used to filter/sort and to compute
// per-dimension scores.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

// Dimension identifies which quality dimension a finding belongs to.
// Matches config.ThresholdsConfig field names.
type Dimension string

const (
	DimCorrectness  Dimension = "correctness"
	DimCoverage     Dimension = "coverage"
	DimComplexity   Dimension = "complexity"
	DimSecurity     Dimension = "security"
	DimArchitecture Dimension = "architecture"
	DimBehavior     Dimension = "behavior"
	DimContract     Dimension = "contract"
	DimE2E          Dimension = "e2e"
	// DimReview is fed by the optional external inferential reviewer.
	// Disabled by default. Sensors in this dimension shell out to an
	// LLM-backed CLI; Harness never embeds a model.
	DimReview Dimension = "review"
)

// Finding is the raw output of a sensor. The Evaluator aggregates findings
// across all sensors to compute dimension scores.
type Finding struct {
	Dimension Dimension `json:"dimension"`
	Severity  Severity  `json:"severity"`
	File      string    `json:"file,omitempty"`
	Line      int       `json:"line,omitempty"`
	Rule      string    `json:"rule"`
	Message   string    `json:"message"`
	// Hint is an optional "positive prompt injection" addendum: a short,
	// agent-readable Suggested fix / Do NOT pair for the rule. It is
	// surfaced in JSON reports and the repair brief but intentionally
	// omitted from the compact TTY output so humans see only the raw
	// linter message. See internal/sensors.LLMHint for the catalog.
	Hint string `json:"hint,omitempty"`
	// Fingerprint is a stable hash that identifies the SAME logical issue
	// across runs. The harness uses it to detect recurring problems
	// (the AI Slop accumulation from problem 6 of the video).
	Fingerprint string `json:"fingerprint"`
}

// Result is what a sensor returns after one execution.
type Result struct {
	SensorName string        `json:"sensor"`
	Dimension  Dimension     `json:"dimension"`
	Duration   time.Duration `json:"duration_ms"`
	// RawScore is 0-100, computed by the sensor based on its own measure
	// (passing tests, coverage %, complexity buckets, etc.). The Evaluator
	// may combine multiple sensor scores into a single dimension score.
	RawScore int       `json:"raw_score"`
	Findings []Finding `json:"findings"`
	// ToolMissing is true when the underlying tool isn't installed.
	// In that case RawScore is 0 and the sensor is essentially skipped.
	ToolMissing bool   `json:"tool_missing,omitempty"`
	Error       string `json:"error,omitempty"`
}

// Sensor is the interface every adapter implements. The Evaluator queries
// Available() to skip sensors whose tools aren't present, then calls Run().
type Sensor interface {
	// Name returns a stable identifier (e.g. "eslint", "pytest", "playwright").
	Name() string
	// Dimension returns which quality dimension this sensor contributes to.
	Dimension() Dimension
	// Available checks whether the underlying tool is installed and the
	// project is configured to use it. Should be fast and side-effect-free.
	Available(root string) bool
	// Run executes the sensor against the project at root. Implementations
	// MUST respect ctx cancellation (long-running test suites need a kill
	// switch to keep the harness responsive).
	Run(ctx context.Context, root string) Result
}

// Registry holds all known sensors. Adapters register themselves via init().
type Registry struct {
	sensors []Sensor
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// Register adds a sensor to the registry.
func (r *Registry) Register(s Sensor) {
	r.sensors = append(r.sensors, s)
}

// All returns every registered sensor.
func (r *Registry) All() []Sensor {
	out := make([]Sensor, len(r.sensors))
	copy(out, r.sensors)
	return out
}

// ByName returns a sensor with the given stable name.
func (r *Registry) ByName(name string) (Sensor, bool) {
	for _, s := range r.sensors {
		if s.Name() == name {
			return s, true
		}
	}
	return nil, false
}

// Available filters the registry to sensors whose tools are present.
func (r *Registry) Available(root string) []Sensor {
	var out []Sensor
	for _, s := range r.sensors {
		if s.Available(root) {
			out = append(out, s)
		}
	}
	return out
}

// fastSensorNames lists adapters that complete in seconds and never
// require network or browser. These are the sensors the pre-commit
// shift-left hook can safely run on every git commit. Tests, coverage,
// dependency audit, browser e2e, and external CLI fixtures are all
// excluded so commits stay quick.
//
// Adding a new fast sensor: extend this map. Slow sensors do not need
// to declare anything; the default is slow.
var fastSensorNames = map[string]bool{
	"eslint":          true,
	"ruff":            true,
	"mypy":            true,
	"go-vet":          true,
	"staticcheck":     true,
	"clippy":          true,
	"js-architecture": true,
	"js-complexity":   true,
}

// IsFast reports whether the sensor with the given name is safe to run
// inside the fast-feedback pre-commit loop.
func IsFast(name string) bool {
	return fastSensorNames[name]
}

// auditSensorNames lists dependency-vulnerability adapters. These do not
// belong in the pre-commit loop (they require network and can be slow)
// but they are the centerpiece of `harness watch`, which monitors drift
// between sprints. Audit findings often change without any code change
// (a new CVE published upstream invalidates a dependency that was clean
// last week), so they need their own cadence outside the sprint cycle.
var auditSensorNames = map[string]bool{
	"npm-audit":   true,
	"pip-audit":   true,
	"cargo-audit": true,
	"govulncheck": true,
}

// IsAudit reports whether the sensor with the given name is a
// dependency-vulnerability audit. Used by `harness watch` to include
// audits in the periodic drift check even though they are not fast
// enough for pre-commit.
func IsAudit(name string) bool {
	return auditSensorNames[name]
}
