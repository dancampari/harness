package adapters

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSpecDeviationScannerFlagsOrphanMarker(t *testing.T) {
	root := t.TempDir()
	body := []byte(`package main

func handle() {
	// SPEC_DEVIATION switching providers because timeouts increased
	_ = 1
	// the reason for this change is documented in the PR description
}
`)
	if err := os.WriteFile(filepath.Join(root, "main.go"), body, 0o644); err != nil {
		t.Fatal(err)
	}

	res := SpecDeviationScanner{}.Run(context.Background(), root)
	if len(res.Findings) != 1 {
		t.Fatalf("expected 1 finding for orphan marker, got %d: %#v", len(res.Findings), res.Findings)
	}
	if res.Findings[0].Rule != "spec-deviation-without-reason" {
		t.Fatalf("unexpected rule: %s", res.Findings[0].Rule)
	}
	if res.Findings[0].Line != 4 {
		t.Fatalf("expected line 4, got %d", res.Findings[0].Line)
	}
	if res.RawScore != 0 {
		t.Fatalf("expected hard-fail RawScore 0, got %d", res.RawScore)
	}
}

func TestSpecDeviationScannerAcceptsReasonAnnotation(t *testing.T) {
	root := t.TempDir()
	body := []byte(`package main

func handle() {
	// SPEC_DEVIATION switching providers because timeouts increased
	// Reason: provider downtime exceeded contract SLA; safe fallback
	_ = 1
}
`)
	if err := os.WriteFile(filepath.Join(root, "main.go"), body, 0o644); err != nil {
		t.Fatal(err)
	}

	res := SpecDeviationScanner{}.Run(context.Background(), root)
	if len(res.Findings) != 0 {
		t.Fatalf("expected no findings, got: %#v", res.Findings)
	}
	if res.RawScore != 100 {
		t.Fatalf("expected RawScore 100, got %d", res.RawScore)
	}
}

func TestSpecDeviationScannerSkipsBuildDirs(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"node_modules", "dist", ".harness"} {
		full := filepath.Join(root, dir)
		if err := os.MkdirAll(full, 0o755); err != nil {
			t.Fatal(err)
		}
		body := "// SPEC_DEVIATION orphan inside skipped dir\n"
		if err := os.WriteFile(filepath.Join(full, "x.ts"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	res := SpecDeviationScanner{}.Run(context.Background(), root)
	for _, f := range res.Findings {
		if strings.Contains(f.File, "node_modules") || strings.Contains(f.File, "dist") || strings.Contains(f.File, ".harness") {
			t.Fatalf("scanner walked a skipped dir: %#v", f)
		}
	}
}
