// Package config defines the harness configuration schema and defaults.
package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const (
	DimCorrectness  = "correctness"
	DimCoverage     = "coverage"
	DimComplexity   = "complexity"
	DimSecurity     = "security"
	DimArchitecture = "architecture"
	DimContract     = "contract"
	DimE2E          = "e2e"
)

// Config is the root configuration for a project's harness.
// Persisted at .harness/config.yaml. Versioned in git so the team shares
// the same thresholds and adapter selection.
type Config struct {
	Version    string           `yaml:"version"`
	Stack      string           `yaml:"stack"`
	Adapters   AdaptersConfig   `yaml:"adapters"`
	Thresholds ThresholdsConfig `yaml:"thresholds"`
	Weights    DimensionWeights `yaml:"weights"`
	E2E        E2EConfig        `yaml:"e2e"`
	Memory     MemoryConfig     `yaml:"memory"`
}

// AdaptersConfig lists which tool adapters are enabled for this project.
// Each adapter wraps a real tool (eslint, ruff, jest, etc.) behind a
// uniform sensor interface.
type AdaptersConfig struct {
	Lint         []string `yaml:"lint"`
	Test         []string `yaml:"test"`
	Coverage     []string `yaml:"coverage"`
	Security     []string `yaml:"security"`
	Complexity   []string `yaml:"complexity"`
	Architecture []string `yaml:"architecture"`
	E2E          []string `yaml:"e2e"`
}

// ThresholdsConfig defines minimum acceptable values per dimension.
// A run is considered "passing" when every dimension meets its threshold.
type ThresholdsConfig struct {
	Correctness  int `yaml:"correctness"`
	Coverage     int `yaml:"coverage"`
	Complexity   int `yaml:"complexity"`
	Security     int `yaml:"security"`
	Architecture int `yaml:"architecture"`
	Contract     int `yaml:"contract"`
	E2E          int `yaml:"e2e"`
}

// DimensionWeights are used to compute the total score as a weighted average.
// Sum should equal 100 but is normalized at runtime if not.
type DimensionWeights struct {
	Correctness  int `yaml:"correctness"`
	Coverage     int `yaml:"coverage"`
	Complexity   int `yaml:"complexity"`
	Security     int `yaml:"security"`
	Architecture int `yaml:"architecture"`
	Contract     int `yaml:"contract"`
	E2E          int `yaml:"e2e"`
}

// E2EConfig controls end-to-end testing behavior. Killing "Teste Fake"
// from problem 4 requires real browser tests with screenshot evidence.
type E2EConfig struct {
	Required        bool     `yaml:"required"`
	Runner          string   `yaml:"runner"` // playwright | puppeteer
	ScreenshotDir   string   `yaml:"screenshot_dir"`
	BaselineDir     string   `yaml:"baseline_dir"`
	BrowsersAllowed []string `yaml:"browsers"`
}

// MemoryConfig tunes the persistent-memory layer.
type MemoryConfig struct {
	// RetentionDays for raw run records. Fingerprints are kept forever.
	RetentionDays int `yaml:"retention_days"`
	// TrendWindow is how many recent runs to consider when computing trend.
	TrendWindow int `yaml:"trend_window"`
}

// ActiveDimensions returns dimensions whose threshold and weight are enabled.
// A dimension is disabled only when both threshold and weight are zero.
func (c Config) ActiveDimensions() []string {
	order := []string{
		DimCorrectness,
		DimCoverage,
		DimComplexity,
		DimSecurity,
		DimArchitecture,
		DimContract,
		DimE2E,
	}
	var out []string
	for _, dim := range order {
		if c.ThresholdFor(dim) > 0 && c.WeightFor(dim) > 0 {
			out = append(out, dim)
		}
	}
	return out
}

// Validate rejects ambiguous config states that could hide missing controls.
func (c Config) Validate() []string {
	var errs []string
	for _, dim := range []string{
		DimCorrectness,
		DimCoverage,
		DimComplexity,
		DimSecurity,
		DimArchitecture,
		DimContract,
		DimE2E,
	} {
		th := c.ThresholdFor(dim)
		wt := c.WeightFor(dim)
		if th < 0 || th > 100 {
			errs = append(errs, fmt.Sprintf("%s threshold must be between 0 and 100", dim))
		}
		if wt < 0 {
			errs = append(errs, fmt.Sprintf("%s weight must be >= 0", dim))
		}
		if (th == 0) != (wt == 0) {
			errs = append(errs,
				fmt.Sprintf("%s must have both threshold and weight > 0, or both set to 0 to disable", dim))
		}
	}
	return errs
}

func (c Config) ThresholdFor(dim string) int {
	switch dim {
	case DimCorrectness:
		return c.Thresholds.Correctness
	case DimCoverage:
		return c.Thresholds.Coverage
	case DimComplexity:
		return c.Thresholds.Complexity
	case DimSecurity:
		return c.Thresholds.Security
	case DimArchitecture:
		return c.Thresholds.Architecture
	case DimContract:
		return c.Thresholds.Contract
	case DimE2E:
		return c.Thresholds.E2E
	}
	return 0
}

func (c Config) WeightFor(dim string) int {
	switch dim {
	case DimCorrectness:
		return c.Weights.Correctness
	case DimCoverage:
		return c.Weights.Coverage
	case DimComplexity:
		return c.Weights.Complexity
	case DimSecurity:
		return c.Weights.Security
	case DimArchitecture:
		return c.Weights.Architecture
	case DimContract:
		return c.Weights.Contract
	case DimE2E:
		return c.Weights.E2E
	}
	return 0
}

func (c Config) AdapterNamesForDimension(dim string) []string {
	switch dim {
	case DimCorrectness:
		return append(copyStrings(c.Adapters.Lint), c.Adapters.Test...)
	case DimCoverage:
		return copyStrings(c.Adapters.Coverage)
	case DimComplexity:
		return copyStrings(c.Adapters.Complexity)
	case DimSecurity:
		return copyStrings(c.Adapters.Security)
	case DimArchitecture:
		return copyStrings(c.Adapters.Architecture)
	case DimE2E:
		return copyStrings(c.Adapters.E2E)
	}
	return nil
}

func (c Config) AllAdapterNames() []string {
	seen := map[string]bool{}
	var out []string
	for _, dim := range []string{
		DimCorrectness,
		DimCoverage,
		DimComplexity,
		DimSecurity,
		DimArchitecture,
		DimE2E,
	} {
		for _, name := range c.AdapterNamesForDimension(dim) {
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}

func copyStrings(in []string) []string {
	out := make([]string, len(in))
	copy(out, in)
	return out
}

// DefaultFor returns a sensible default config for the detected stack.
// Stack values come from detect.DetectStack().
func DefaultFor(stack string) Config {
	c := Config{
		Version: "2",
		Stack:   stack,
		Thresholds: ThresholdsConfig{
			Contract: 80,
		},
		Weights: DimensionWeights{
			Contract: 100,
		},
		E2E: E2EConfig{
			Required:      true,
			Runner:        "playwright",
			ScreenshotDir: ".harness/screenshots",
			BaselineDir:   ".harness/screenshots/baseline",
		},
		Memory: MemoryConfig{
			RetentionDays: 365,
			TrendWindow:   10,
		},
	}

	switch stack {
	case "node", "typescript":
		c.Thresholds = ThresholdsConfig{
			Correctness:  80,
			Coverage:     70,
			Complexity:   75,
			Security:     85,
			Architecture: 70,
			Contract:     80,
			E2E:          70,
		}
		c.Weights = DimensionWeights{
			Correctness:  20,
			Coverage:     15,
			Complexity:   10,
			Security:     15,
			Architecture: 10,
			Contract:     20,
			E2E:          10,
		}
		c.Adapters = AdaptersConfig{
			Lint:         []string{"eslint"},
			Test:         []string{"jest", "vitest"},
			Coverage:     []string{"jest-coverage", "vitest-coverage"},
			Security:     []string{"npm-audit"},
			Complexity:   []string{"js-complexity"},
			Architecture: []string{"js-architecture"},
			E2E:          []string{"playwright"},
		}
	case "python":
		c.Adapters = AdaptersConfig{
			Lint:     []string{},
			Test:     []string{},
			Coverage: []string{},
			Security: []string{},
			E2E:      []string{},
		}
	case "go":
		c.Adapters = AdaptersConfig{
			Lint:     []string{},
			Test:     []string{},
			Coverage: []string{},
			Security: []string{},
			E2E:      []string{},
		}
	case "rust":
		c.Adapters = AdaptersConfig{
			Lint:     []string{},
			Test:     []string{},
			Coverage: []string{},
			Security: []string{},
		}
	default:
		// Unknown stack — leave adapters empty; user fills in.
		c.Adapters = AdaptersConfig{}
	}
	return c
}

// Load reads config from disk.
func Load(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return Config{}, err
	}
	return c, nil
}

// Save writes config to disk.
func Save(path string, c Config) error {
	b, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
