package adapters

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/dancampari/harness/internal/planner"
	"github.com/dancampari/harness/internal/sensors"
)

// ScopeCreep enforces TLC implement.md's "touch only the files listed in
// the task's Where: line" rule. It reads the active sprint's tasks.md
// (canonical .specs/features/<slug>/tasks.md or legacy
// .harness/tasks/sprint-NNN.md), builds the union of every `Where:` path
// declared across tasks, then asks git which files have changed since
// HEAD. Any modified file outside the union is reported as scope-creep.
//
// The sensor degrades gracefully:
//   - no git → skip (RawScore 100, no findings)
//   - no tasks.md → skip (the granularity gate already covers that case)
//   - all changed files are listed → RawScore 100, no findings
type ScopeCreep struct{}

func (ScopeCreep) Name() string                 { return "scope-creep" }
func (ScopeCreep) Dimension() sensors.Dimension { return sensors.DimContract }

// Available reports true whenever git is on PATH; the run path further
// short-circuits if no tasks.md exists. Keeping Available() loose avoids
// hiding the sensor from doctor/status when only the tasks plan is
// missing.
func (ScopeCreep) Available(root string) bool {
	_, err := exec.LookPath("git")
	return err == nil
}

func (s ScopeCreep) Run(ctx context.Context, root string) sensors.Result {
	start := time.Now()
	res := sensors.Result{
		SensorName: s.Name(),
		Dimension:  s.Dimension(),
		RawScore:   100,
	}

	tasksPath := findLatestTasksPath(root)
	if tasksPath == "" {
		res.Duration = time.Since(start)
		return res
	}

	plan, err := planner.ParseTasks(tasksPath)
	if err != nil || plan == nil || len(plan.Tasks) == 0 {
		res.Duration = time.Since(start)
		return res
	}

	allowed := unionWhere(plan)
	if len(allowed) == 0 {
		res.Duration = time.Since(start)
		return res
	}

	changed, ok := changedFilesViaGit(ctx, root)
	if !ok {
		// git failed or returned nothing useful; do not penalise.
		res.Duration = time.Since(start)
		return res
	}

	outsiders := filesOutsideAllowed(changed, allowed)
	for _, f := range outsiders {
		res.Findings = append(res.Findings, finding(
			sensors.DimContract,
			sensors.SeverityHigh,
			f,
			0,
			"scope-creep",
			"file modified outside the union of `Where:` paths declared in "+filepath.ToSlash(filepath.Base(tasksPath))+"; either add it to a task's Where list or revert the change (TLC implement.md scope guardrail)",
		))
	}
	if len(res.Findings) > 0 {
		// Any off-contract product edit invalidates the implementation
		// scope. Keep this blocking; otherwise sensor averaging can hide
		// exactly the class of drift TLC is meant to prevent.
		res.RawScore = 0
	}
	res.Duration = time.Since(start)
	return res
}

// findLatestTasksPath returns the highest-numbered tasks.md from the
// canonical .specs/features/<slug>/tasks.md tree, falling back to
// .harness/tasks/sprint-NNN.md. Returns empty when none exist.
func findLatestTasksPath(root string) string {
	if p := latestTasksFromSpecs(root); p != "" {
		return p
	}
	return latestTasksFromHarness(root)
}

var (
	specsSprintDirRe  = regexp.MustCompile(`^sprint-(\d+)$`)
	harnessTaskFileRe = regexp.MustCompile(`^sprint-(\d+)\.md$`)
)

func latestTasksFromSpecs(root string) string {
	dir := filepath.Join(root, ".specs", "features")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	max := 0
	var pick string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		m := specsSprintDirRe.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		var n int
		for _, r := range m[1] {
			n = n*10 + int(r-'0')
		}
		candidate := filepath.Join(dir, e.Name(), "tasks.md")
		if _, err := os.Stat(candidate); err != nil {
			continue
		}
		if n > max {
			max = n
			pick = candidate
		}
	}
	return pick
}

func latestTasksFromHarness(root string) string {
	dir := filepath.Join(root, ".harness", "tasks")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	max := 0
	var pick string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := harnessTaskFileRe.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		var n int
		for _, r := range m[1] {
			n = n*10 + int(r-'0')
		}
		if n > max {
			max = n
			pick = filepath.Join(dir, e.Name())
		}
	}
	return pick
}

func unionWhere(plan *planner.TaskPlan) []string {
	seen := map[string]struct{}{}
	for _, t := range plan.Tasks {
		for _, w := range t.Where {
			seen[filepath.ToSlash(strings.TrimSpace(w))] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// changedFilesViaGit returns the relative paths of files modified since
// HEAD according to git. The second return value is false when git is
// unavailable, the repo lacks a HEAD commit, or the command otherwise
// fails — in any of those cases the sensor must NOT report findings.
func changedFilesViaGit(ctx context.Context, root string) ([]string, bool) {
	cmd := exec.CommandContext(ctx, "git", "diff", "--name-only", "HEAD")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, false
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		files = append(files, filepath.ToSlash(line))
	}
	return files, true
}

// filesOutsideAllowed returns the subset of changed files whose path is
// neither listed verbatim in allowed nor matches an allowed prefix.
// Prefix matching catches cases where a task says "src/auth/" and the
// agent edits "src/auth/user.ts".
func filesOutsideAllowed(changed, allowed []string) []string {
	var out []string
	for _, file := range changed {
		if isAllowed(file, allowed) {
			continue
		}
		// Always allow the workspace's own task plan to evolve without
		// triggering self-referential findings.
		if strings.Contains(file, ".specs/features/") || strings.HasPrefix(file, ".harness/") {
			continue
		}
		out = append(out, file)
	}
	return out
}

func isAllowed(file string, allowed []string) bool {
	for _, a := range allowed {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		if a == file {
			return true
		}
		if strings.HasSuffix(a, "/") && strings.HasPrefix(file, a) {
			return true
		}
		if strings.HasSuffix(a, "*") && strings.HasPrefix(file, strings.TrimSuffix(a, "*")) {
			return true
		}
	}
	return false
}
