package adapters

import "github.com/dancampari/harness/internal/sensors"

// BuildRegistry returns a registry populated with every concrete adapter.
// The Evaluator filters this list with .Available(root) before running.
//
// To add a new adapter:
//  1. Create a new file in this package implementing sensors.Sensor.
//  2. Append it here.
//  3. Add its tool name to config.AdaptersConfig defaults.
func BuildRegistry() *sensors.Registry {
	r := sensors.NewRegistry()
	r.Register(ESLint{})
	r.Register(Jest{})
	r.Register(Vitest{})
	r.Register(JestCoverage{})
	r.Register(VitestCoverage{})
	r.Register(NpmAudit{})
	r.Register(JSComplexity{})
	r.Register(JSArchitecture{})
	r.Register(ApprovedFixtures{})
	r.Register(Playwright{})
	r.Register(Ruff{})
	r.Register(Mypy{})
	r.Register(Pytest{})
	r.Register(PytestCoverage{})
	r.Register(PipAudit{})
	r.Register(GoVet{})
	r.Register(GoTest{})
	r.Register(GoTestCoverage{})
	r.Register(Staticcheck{})
	r.Register(Govulncheck{})
	r.Register(CargoClippy{})
	r.Register(CargoTest{})
	r.Register(CargoAudit{})
	r.Register(Semgrep{})
	r.Register(SpecDeviationScanner{}) // TLC implement.md gate
	r.Register(ScopeCreep{})           // TLC implement.md scope guardrail
	r.Register(TDDViolation{})         // TLC implement.md tests-first gate
	r.Register(TestCountTracker{})     // TLC implement.md test-count regression gate
	r.Register(ExternalReviewer{})     // optional; reads its config from .harness/config.yaml at Run time
	return r
}
