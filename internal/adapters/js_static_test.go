package adapters

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dancampari/harness/internal/sensors"
)

func TestJSComplexityFindsComplexFunction(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"), `{"name":"x"}`)
	writeFile(t, filepath.Join(root, "src", "bad.js"), `
export function bad(x) {
  if (x.a) {}
  if (x.b) {}
  if (x.c) {}
  if (x.d) {}
  if (x.e) {}
  if (x.f) {}
  if (x.g) {}
  if (x.h) {}
  if (x.i) {}
  if (x.j) {}
}
`)

	res := JSComplexity{}.Run(context.Background(), root)
	if res.RawScore >= 100 {
		t.Fatalf("expected complexity deduction, got %+v", res)
	}
	if len(res.Findings) == 0 {
		t.Fatal("expected complexity finding")
	}
}

func TestJSArchitectureFindsForbiddenImportAndCycle(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"), `{"name":"x"}`)
	writeFile(t, filepath.Join(root, ".harness", "contracts", "sprint-001.md"), "- forbidden_imports: `src/domain/* -> src/ui/*`\n")
	writeFile(t, filepath.Join(root, "src", "domain", "a.ts"), `import "../ui/b"; import "./c";`)
	writeFile(t, filepath.Join(root, "src", "domain", "c.ts"), `import "./a";`)
	writeFile(t, filepath.Join(root, "src", "ui", "b.ts"), `export const b = 1;`)

	res := JSArchitecture{}.Run(context.Background(), root)
	if res.RawScore >= 100 {
		t.Fatalf("expected architecture deduction, got %+v", res)
	}
	if !hasFindingRule(res.Findings, "forbidden-import") {
		t.Fatalf("expected forbidden import finding, got %+v", res.Findings)
	}
	if !hasFindingRule(res.Findings, "import-cycle") {
		t.Fatalf("expected import cycle finding, got %+v", res.Findings)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func hasFindingRule(findings []sensors.Finding, rule string) bool {
	for _, finding := range findings {
		if finding.Rule == rule {
			return true
		}
	}
	return false
}
