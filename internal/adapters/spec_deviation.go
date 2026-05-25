package adapters

import (
	"bufio"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/dancampari/harness/internal/sensors"
)

// SpecDeviationScanner walks the project tree looking for the literal
// token "SPEC_DEVIATION" left behind by agents who consciously broke
// from the contract. TLC's implement.md mandates that any deviation be
// followed (within five lines) by a "Reason:" annotation explaining
// why; orphan markers without a Reason are reported as findings so the
// human reviewer sees the unjustified divergences.
//
// The sensor never reads or interprets the marker's surrounding code.
// It just enforces the *presence* of a reason — which is exactly the
// deterministic gate TLC describes.
type SpecDeviationScanner struct{}

func (SpecDeviationScanner) Name() string                 { return "spec-deviation-scanner" }
func (SpecDeviationScanner) Dimension() sensors.Dimension { return sensors.DimContract }

// Available is always true: the scanner is a pure file walker with no
// external tool dependency.
func (SpecDeviationScanner) Available(root string) bool { return true }

var (
	specDeviationRe = regexp.MustCompile(`SPEC_DEVIATION\b`)
	reasonRe        = regexp.MustCompile(`(?i)\bReason\s*:`)
	// reasonLookahead is how many lines after the marker we accept a
	// `Reason:` annotation on. TLC's implement.md suggests "near the
	// marker"; five lines covers the typical 1-2 comment lines plus a
	// blank or guard line between them.
	reasonLookahead = 5
)

func (s SpecDeviationScanner) Run(ctx context.Context, root string) sensors.Result {
	start := time.Now()
	res := sensors.Result{
		SensorName: s.Name(),
		Dimension:  s.Dimension(),
		RawScore:   100,
	}
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() {
			if shouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !shouldScanFile(d.Name()) {
			return nil
		}
		orphans, scanErr := scanForOrphanDeviations(path, reasonLookahead)
		if scanErr != nil {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		for _, lineNo := range orphans {
			res.Findings = append(res.Findings, finding(
				sensors.DimContract,
				sensors.SeverityMedium,
				filepath.ToSlash(rel),
				lineNo,
				"spec-deviation-without-reason",
				"SPEC_DEVIATION marker is not followed by a `Reason:` annotation within "+itoa(reasonLookahead)+" lines (TLC implement.md requires every deviation to declare its reason)",
			))
		}
		return nil
	})
	if len(res.Findings) > 0 {
		// SPEC_DEVIATION without an adjacent Reason is an explicit break
		// from the agreed spec without accountability. Treat it as a hard
		// contract failure instead of letting averages dilute it.
		res.RawScore = 0
	}
	res.Duration = time.Since(start)
	return res
}

// scanForOrphanDeviations returns the 1-indexed line numbers of every
// SPEC_DEVIATION marker that lacks a `Reason:` annotation within
// `lookahead` lines after it.
func scanForOrphanDeviations(path string, lookahead int) ([]int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var lines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	var orphans []int
	for i, line := range lines {
		if !specDeviationRe.MatchString(line) {
			continue
		}
		hasReason := false
		end := i + lookahead
		if end >= len(lines) {
			end = len(lines) - 1
		}
		for j := i; j <= end; j++ {
			if reasonRe.MatchString(lines[j]) {
				hasReason = true
				break
			}
		}
		if !hasReason {
			orphans = append(orphans, i+1)
		}
	}
	return orphans, nil
}

// shouldScanFile keeps the walker fast by limiting the scan to text
// files where a SPEC_DEVIATION marker would plausibly live.
func shouldScanFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".go", ".ts", ".tsx", ".js", ".jsx", ".py", ".rs", ".java",
		".kt", ".rb", ".cs", ".cpp", ".c", ".h", ".hpp", ".swift",
		".md":
		return true
	}
	return false
}

func shouldSkipDir(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", ".harness", ".specs",
		"dist", "build", "out", "target", ".next", ".cache":
		return true
	}
	return false
}

// itoa avoids pulling strconv just for two int formats in a sensor hot
// loop. Mirrors the helper in internal/planning/policy.go.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if negative {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}
