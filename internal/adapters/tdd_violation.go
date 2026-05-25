package adapters

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/dancampari/harness/internal/sensors"
)

// TDDViolation enforces TLC implement.md's "Tests First (RED → GREEN)"
// rule across git history. For every commit in the active sprint
// window (defaulting to the last 20) the sensor inspects the file list:
// if the commit touched at least one implementation file in a
// recognised language but no test file in the same language, a
// `tdd-violation` finding is emitted under the contract dimension.
//
// The sensor stays conservative:
//   - it never blocks commits that contain only doc / config / asset
//     changes,
//   - it allows a commit to ship multiple impl files alongside ONE
//     test file (TLC's Granularity check governs the file count, not
//     this gate),
//   - it skips merge commits (parents > 1) since merge diffs are
//     mechanical rather than authored.
//
// When git is unavailable the sensor degrades to a no-op so harness
// keeps working in repos without VCS.
type TDDViolation struct{}

func (TDDViolation) Name() string                 { return "tdd-violation" }
func (TDDViolation) Dimension() sensors.Dimension { return sensors.DimContract }

// Available reports whether the project is in a git repo and the git
// binary is on PATH. We don't gate on the existence of tasks.md — TDD
// is a code-level rule, not a planning gate.
func (TDDViolation) Available(root string) bool {
	if _, err := exec.LookPath("git"); err != nil {
		return false
	}
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "true"
}

// scanWindow defaults the commit window size; can be overridden by an
// env var in tests if we ever need to.
const scanWindow = 20

func (t TDDViolation) Run(ctx context.Context, root string) sensors.Result {
	start := time.Now()
	res := sensors.Result{
		SensorName: t.Name(),
		Dimension:  t.Dimension(),
		RawScore:   100,
	}

	commits, err := recentCommitHashes(ctx, root, scanWindow)
	if err != nil || len(commits) == 0 {
		res.Duration = time.Since(start)
		return res
	}
	for _, sha := range commits {
		if isMergeCommit(ctx, root, sha) {
			continue
		}
		files, err := commitFiles(ctx, root, sha)
		if err != nil {
			continue
		}
		impl, test := classifyChanges(files)
		if len(impl) == 0 {
			continue
		}
		if len(test) == 0 {
			res.Findings = append(res.Findings, finding(
				sensors.DimContract,
				sensors.SeverityMedium,
				shortFilesRef(impl),
				0,
				"tdd-violation",
				"commit "+sha[:7]+" modified implementation files without any test file in the same commit (TLC implement.md requires tests-first RED → GREEN)",
			))
		}
	}
	if len(res.Findings) > 0 {
		// A commit that changes implementation without tests violates the
		// agreed execution method, so it must block the contract gate.
		res.RawScore = 0
	}
	res.Duration = time.Since(start)
	return res
}

// recentCommitHashes returns the most recent up-to-N commit hashes in
// reverse-chronological order. Returns nil when git fails or the repo
// has no commits yet (fresh init).
func recentCommitHashes(ctx context.Context, root string, n int) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "log", "-n", itoa(n), "--pretty=format:%H")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var hashes []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			hashes = append(hashes, line)
		}
	}
	return hashes, nil
}

func isMergeCommit(ctx context.Context, root, sha string) bool {
	cmd := exec.CommandContext(ctx, "git", "rev-list", "--parents", "-n", "1", sha)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	parts := strings.Fields(strings.TrimSpace(string(out)))
	// rev-list emits "<sha> <parent1> <parent2?>"; merges have 2+ parents.
	return len(parts) > 2
}

func commitFiles(ctx context.Context, root, sha string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "show", "--no-color", "--pretty=format:", "--name-only", sha)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		files = append(files, filepath.ToSlash(line))
	}
	return files, nil
}

// classifyChanges splits a commit's file list into implementation
// files and test files based on path heuristics. The rule of thumb is
// deliberately permissive: anything matching the conventional test
// patterns (test/, tests/, __tests__/, *_test.*, *.test.*,
// *.spec.*) counts as a test file. Implementation files are
// non-test source files in a language we recognise.
func classifyChanges(files []string) (impl, test []string) {
	for _, f := range files {
		if !isSourceFile(f) {
			continue
		}
		if isTestFile(f) {
			test = append(test, f)
		} else {
			impl = append(impl, f)
		}
	}
	return impl, test
}

var sourceExtensions = map[string]bool{
	".go":    true,
	".ts":    true,
	".tsx":   true,
	".js":    true,
	".jsx":   true,
	".py":    true,
	".rs":    true,
	".java":  true,
	".kt":    true,
	".swift": true,
	".rb":    true,
	".cs":    true,
}

func isSourceFile(path string) bool {
	return sourceExtensions[strings.ToLower(filepath.Ext(path))]
}

func isTestFile(path string) bool {
	lower := strings.ToLower(path)
	name := filepath.Base(lower)
	if strings.Contains(lower, "/test/") || strings.Contains(lower, "/tests/") ||
		strings.Contains(lower, "/__tests__/") || strings.Contains(lower, "/spec/") {
		return true
	}
	if strings.HasSuffix(name, "_test.go") {
		return true
	}
	// pytest convention: filenames starting with "test_" are tests.
	if strings.HasPrefix(name, "test_") {
		return true
	}
	for _, marker := range []string{".test.", ".spec.", "_test.", "_spec."} {
		if strings.Contains(name, marker) {
			return true
		}
	}
	return false
}

// shortFilesRef joins up to three implementation paths so the finding
// surfaces context without flooding the report.
func shortFilesRef(files []string) string {
	if len(files) == 0 {
		return ""
	}
	if len(files) > 3 {
		return strings.Join(files[:3], ", ") + " (+ more)"
	}
	return strings.Join(files, ", ")
}
