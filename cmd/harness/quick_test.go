package harness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNextQuickTaskNumberIncrements(t *testing.T) {
	root := t.TempDir()
	specsRoot := filepath.Join(root, ".specs")
	quickDir := filepath.Join(specsRoot, "quick")
	for _, name := range []string{"001-existing", "003-skipped", "002-another", "notes.md"} {
		if err := os.MkdirAll(filepath.Join(quickDir, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	got, err := nextQuickTaskNumber(specsRoot)
	if err != nil {
		t.Fatal(err)
	}
	if got != 4 {
		t.Fatalf("expected next quick task to be 4, got %d", got)
	}
}

func TestSlugifyCanonicalisesInput(t *testing.T) {
	cases := map[string]string{
		"fix navbar overflow on mobile":         "fix-navbar-overflow-on-mobile",
		"   leading and trailing whitespace   ": "leading-and-trailing-whitespace",
		"@@@ symbols ::: only ###":              "symbols-only",
		"":                                      "task",
		strings.Repeat("a", 80):                 strings.Repeat("a", 48),
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Fatalf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}
