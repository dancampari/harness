package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestApprovedFixturesPassesApprovedOutput(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "invoice.json", map[string]any{
		"name":    "invoice summary",
		"command": os.Args[0],
		"args":    []string{"-test.run=TestApprovedFixtureHelperProcess", "--", "approved"},
		"env":     map[string]string{"HARNESS_FIXTURE_HELPER": "1"},
		"expect": map[string]any{
			"exit_code": 0,
			"stdout":    "approved\n",
			"stderr":    "",
		},
	})

	res := ApprovedFixtures{}.Run(context.Background(), root)
	if res.RawScore != 100 || len(res.Findings) != 0 {
		t.Fatalf("expected passing fixture, got score=%d findings=%+v", res.RawScore, res.Findings)
	}
}

func TestApprovedFixturesFindsRegression(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "invoice.json", map[string]any{
		"name":    "invoice summary",
		"command": os.Args[0],
		"args":    []string{"-test.run=TestApprovedFixtureHelperProcess", "--", "changed"},
		"env":     map[string]string{"HARNESS_FIXTURE_HELPER": "1"},
		"expect": map[string]any{
			"exit_code": 0,
			"stdout":    "approved\n",
		},
	})

	res := ApprovedFixtures{}.Run(context.Background(), root)
	if res.RawScore >= 100 || len(res.Findings) == 0 {
		t.Fatalf("expected fixture regression, got score=%d findings=%+v", res.RawScore, res.Findings)
	}
	if res.Findings[0].Rule != "fixture-regression" {
		t.Fatalf("expected fixture-regression, got %+v", res.Findings[0])
	}
}

func TestApprovedFixturesAcceptsCurrentOutput(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HARNESS_ACCEPT_FIXTURES", "1")
	path := writeFixture(t, root, "invoice.json", map[string]any{
		"name":    "invoice summary",
		"command": os.Args[0],
		"args":    []string{"-test.run=TestApprovedFixtureHelperProcess", "--", "accepted"},
		"env":     map[string]string{"HARNESS_FIXTURE_HELPER": "1"},
	})

	res := ApprovedFixtures{}.Run(context.Background(), root)
	if len(res.Findings) != 0 {
		t.Fatalf("expected accept to write without findings, got %+v", res.Findings)
	}
	var fixture approvedFixture
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Expect.Stdout == nil || *fixture.Expect.Stdout != "accepted\n" {
		t.Fatalf("expected accepted stdout to be written, got %+v", fixture.Expect.Stdout)
	}
}

func TestApprovedFixtureHelperProcess(t *testing.T) {
	if os.Getenv("HARNESS_FIXTURE_HELPER") != "1" {
		return
	}
	if len(os.Args) == 0 {
		os.Exit(2)
	}
	value := os.Args[len(os.Args)-1]
	fmt.Println(value)
	os.Exit(0)
}

func writeFixture(t *testing.T, root, name string, payload map[string]any) string {
	t.Helper()
	dir := filepath.Join(root, ".harness", "fixtures")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	b = append(b, '\n')
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
