package harness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureRoadmapCreatesTemplate(t *testing.T) {
	root := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(wd) }()

	path, err := ensureRoadmap()
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"## Now", "## Next", "## Later", "Project Roadmap"} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("expected template to contain %q, got:\n%s", want, body)
		}
	}
}

func TestAppendRoadmapAddsChecklistEntry(t *testing.T) {
	root := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(wd) }()

	if err := appendRoadmap("auth rotation playbook"); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(siblingSpecsRoot(".harness"), "project", "ROADMAP.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "- [ ] auth rotation playbook") {
		t.Fatalf("expected checklist entry in ROADMAP.md, got:\n%s", body)
	}
}
