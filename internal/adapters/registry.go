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
	r.Register(Playwright{})
	// Future: Ruff{}, Mypy{}, Pytest{}, GoVet{}, Staticcheck{}, Clippy{},
	// Semgrep{}, NpmAudit{}, PipAudit{}, etc.
	return r
}
