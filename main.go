// Package main is the entry point for the harness CLI.
// Harness Engineering agent — stack-agnostic, deterministic, offline.
//
// Philosophy:
//   - Agent = Model + Harness
//   - Three isolated processes: Planner (manual) → Builder (CLI) → Evaluator (subprocess)
//   - Memory is first-class: progress.md (narrative) + memory.db (index)
//   - Reports only — never blocks. The CLI decides.
package main

import (
	"fmt"
	"os"

	"github.com/dancampari/harness/cmd/harness"
)

// version is set at build time via -ldflags.
var version = "dev"

func main() {
	if err := harness.Execute(version); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
