package adapters

import (
	"strings"
	"testing"

	"github.com/dancampari/harness/internal/sensors"
)

func TestParseMypyOutput(t *testing.T) {
	findings := parseMypyOutput("pkg/app.py:12: error: Incompatible return value type [return-value]\n")
	if len(findings) != 1 {
		t.Fatalf("expected one mypy finding, got %#v", findings)
	}
	if findings[0].File != "pkg/app.py" || findings[0].Line != 12 || findings[0].Rule != "return-value" {
		t.Fatalf("unexpected finding: %#v", findings[0])
	}
}

func TestParsePytestSummary(t *testing.T) {
	passed, failed := parsePytestSummary("====== 2 failed, 5 passed, 1 skipped in 0.12s ======", 1)
	if passed != 5 || failed != 2 {
		t.Fatalf("expected 5 passed and 2 failed, got passed=%d failed=%d", passed, failed)
	}
}

func TestParseGoCoverageTotal(t *testing.T) {
	score := parseGoCoverageTotal("github.com/acme/pkg/a.go:10: A 100.0%\ntotal: (statements) 84.6%\n")
	if score != 84 {
		t.Fatalf("expected 84, got %d", score)
	}
}

func TestParseCargoMessages(t *testing.T) {
	output := `{"reason":"compiler-message","message":{"message":"needless return","code":{"code":"clippy::needless_return"},"level":"warning","spans":[{"file_name":"src/lib.rs","line_start":7,"is_primary":true}]}}`
	findings := parseCargoMessages(output)
	if len(findings) != 1 {
		t.Fatalf("expected one cargo finding, got %#v", findings)
	}
	if findings[0].File != "src/lib.rs" || findings[0].Rule != "clippy::needless_return" {
		t.Fatalf("unexpected finding: %#v", findings[0])
	}
}

func TestRegistryIncludesBroaderStackSensors(t *testing.T) {
	reg := BuildRegistry()
	for _, name := range []string{
		"ruff", "mypy", "pytest", "pytest-cov", "pip-audit",
		"go-vet", "staticcheck", "go-test", "go-test-coverage", "govulncheck",
		"clippy", "cargo-test", "cargo-audit", "semgrep",
	} {
		if _, ok := reg.ByName(name); !ok {
			t.Fatalf("expected registry to include %s", name)
		}
	}
}

func TestParseTextToolFindings(t *testing.T) {
	findings := parseTextToolFindings(sensors.DimCorrectness, "tool", "src/main.go:20: something broke\n# package\n", sensors.SeverityHigh)
	if len(findings) != 1 || !strings.Contains(findings[0].Message, "something broke") {
		t.Fatalf("unexpected findings: %#v", findings)
	}
}
