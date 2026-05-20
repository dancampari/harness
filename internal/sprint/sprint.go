// Package sprint orchestrates the Contract → Build → QA → Score lifecycle.
//
// Each sprint advances through four states, recorded in memory.db and
// surfaced via the TUI:
//
//	Contract:  user/CLI writes contracts/sprint-NNN.md
//	Build:     CLI implements the feature (harness has no role here)
//	QA:        Evaluator subprocess runs sensors, emits evaluations/sprint-NNN.md
//	Score:     Manager consolidates, writes report, appends to progress.md
//
// The Manager is the orchestrator. Sensors live in /internal/adapters,
// the evaluation logic in /internal/evaluator, and persistent memory
// in /internal/memory.
package sprint

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/dancampari/harness/internal/adapters"
	"github.com/dancampari/harness/internal/config"
	"github.com/dancampari/harness/internal/evaluator"
	"github.com/dancampari/harness/internal/memory"
	"github.com/dancampari/harness/internal/planner"
)

// Manager coordinates the sprint lifecycle for one project.
type Manager struct {
	root string // .harness directory
	cfg  config.Config
	mem  *memory.DB
}

// NewManager loads config and opens memory.
func NewManager(harnessDir string) (*Manager, error) {
	cfg, err := config.Load(filepath.Join(harnessDir, "config.yaml"))
	if err != nil {
		return nil, fmt.Errorf("load config: %w (did you run 'harness init'?)", err)
	}
	if errs := cfg.Validate(); len(errs) > 0 {
		return nil, fmt.Errorf("invalid config: %s", strings.Join(errs, "; "))
	}
	db, err := memory.Open(filepath.Join(harnessDir, "memory.db"))
	if err != nil {
		return nil, err
	}
	return &Manager{root: harnessDir, cfg: cfg, mem: db}, nil
}

// NewContract creates the next sprint contract from a goal.
// Returns the file path and the assigned sprint number.
func (m *Manager) NewContract(goal string) (string, int, error) {
	next, err := m.nextSprintNumber()
	if err != nil {
		return "", 0, err
	}
	path := filepath.Join(m.root, "contracts", fmt.Sprintf("sprint-%03d.md", next))
	if err := os.WriteFile(path, []byte(planner.Template(next, goal)), 0o644); err != nil {
		return "", 0, err
	}
	_ = m.mem.UpsertContract(next, goal, "draft", path)
	return path, next, nil
}

// Status reports the current state of the most recent sprint.
type Status struct {
	Number   int
	Goal     string
	Contract string // draft | agreed | violated | missing
	Build    string // pending | done
	QA       string // pending | pass | fail
	Score    string // - | NN/100
}

// Status returns the current sprint status.
func (m *Manager) Status() (Status, error) {
	n, err := m.currentSprintNumber()
	if err != nil {
		return Status{}, err
	}
	st := Status{Number: n, Contract: "missing", Build: "pending", QA: "pending", Score: "-"}
	cpath := filepath.Join(m.root, "contracts", fmt.Sprintf("sprint-%03d.md", n))
	if c, err := planner.Parse(cpath); err == nil {
		st.Goal = c.Title
		if errs := c.Validate(); len(errs) == 0 {
			st.Contract = "agreed"
		} else {
			st.Contract = "draft"
		}
	}
	epath := filepath.Join(m.root, "evaluations", fmt.Sprintf("sprint-%03d.md", n))
	if b, err := os.ReadFile(epath); err == nil {
		st.Build = "done"
		if strings.Contains(string(b), "Verdict: PASS") {
			st.QA = "pass"
		} else if strings.Contains(string(b), "Verdict: FAIL") {
			st.QA = "fail"
		}
		if m := regexp.MustCompile(`Aggregate Score:\s*(\d+)`).FindStringSubmatch(string(b)); m != nil {
			st.Score = m[1] + "/100"
		}
	}
	return st, nil
}

// RunQA is the parent-side entry point. It spawns the evaluator as an
// ISOLATED SUBPROCESS to honor problem 5 of the video: the process that
// wrote the code must not be the one that judges it.
//
// The subprocess runs `harness sprint qa --internal`. It:
//   - inherits no in-memory state from the parent or from the Builder
//   - runs with a deliberately filtered environment (no env vars that
//     could carry contextual state from the coding CLI)
//   - reads only files from disk (contract, code, config)
//   - emits the EvaluationResult as JSON on stdout
//   - dies when done
//
// The parent collects the JSON and returns a QAResult. Even if running
// in the same shell session, OS process boundaries enforce the
// separation. There is no way for the subprocess to be influenced by
// the parent's prior state.
func (m *Manager) RunQA(acceptScreenshots bool) (*QAResult, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("locate harness binary: %w", err)
	}
	repoRoot, err := filepath.Abs(filepath.Dir(m.root))
	if err != nil {
		return nil, fmt.Errorf("resolve repo root: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 16*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, exe, "sprint", "qa", "--internal")
	cmd.Stdin = nil        // sever any inherited stdin (no terminal access)
	cmd.Stderr = os.Stderr // forward subprocess diagnostics
	cmd.Dir = repoRoot     // explicit working dir, not inherited
	cmd.Env = isolatedEvaluatorEnv(repoRoot, acceptScreenshots)

	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("evaluator subprocess failed (exit %d)",
				exitErr.ExitCode())
		}
		return nil, fmt.Errorf("evaluator subprocess: %w", err)
	}

	var result evaluator.EvaluationResult
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("parse subprocess output: %w", err)
	}
	return &QAResult{
		result:         &result,
		sprintNumber:   result.SprintNumber,
		evaluationPath: filepath.Join(m.root, "evaluations", fmt.Sprintf("sprint-%03d.md", result.SprintNumber)),
		reportPath:     filepath.Join(m.root, "reports", fmt.Sprintf("sprint-%03d.json", result.SprintNumber)),
	}, nil
}

// RunQAInternal is the child-side worker. Called only when this process
// is acting as the isolated evaluator subprocess spawned by RunQA.
//
// Contract with the parent:
//   - stdin is closed (we cannot prompt the user)
//   - stdout is reserved for the EvaluationResult JSON — nothing else
//     may be written there, ever (no log lines, no progress prints)
//   - stderr is for human-facing diagnostics from sensors
//   - on success, exit 0 regardless of the verdict (PASS or FAIL is data,
//     not a process-level failure)
//   - on failure of the harness itself, exit non-zero
//
// We DO write the evaluation markdown and JSON report to disk here
// (under .harness/evaluations/ and .harness/reports/) so those
// artifacts exist even if the parent crashes between subprocess exit
// and result rendering.
func (m *Manager) RunQAInternal(stdout io.Writer, acceptScreenshots bool) error {
	if acceptScreenshots {
		_ = os.Setenv("HARNESS_ACCEPT_SCREENSHOTS", "1")
	}
	n, err := m.currentSprintNumber()
	if err != nil {
		return err
	}
	cpath := filepath.Join(m.root, "contracts", fmt.Sprintf("sprint-%03d.md", n))
	contract, err := planner.Parse(cpath)
	if err != nil {
		return fmt.Errorf("parse contract: %w", err)
	}
	if errs := contract.Validate(); len(errs) > 0 {
		return fmt.Errorf("contract not valid: %s", strings.Join(errs, "; "))
	}

	repoRoot, err := filepath.Abs(filepath.Dir(m.root))
	if err != nil {
		return fmt.Errorf("resolve repo root: %w", err)
	}

	check := contract.CheckAgainstDiff(repoRoot)
	ev := evaluator.New(m.cfg, adapters.BuildRegistry())
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	result, err := ev.Evaluate(ctx, repoRoot, n, check)
	if err != nil {
		return err
	}

	if err := m.writeEvaluationMarkdown(n, contract, result); err != nil {
		return err
	}
	if err := m.writeJSONReport(n, result); err != nil {
		return err
	}

	// Emit ONLY the result as JSON on stdout. The parent depends on
	// this output being parseable; no other bytes may be written.
	return json.NewEncoder(stdout).Encode(result)
}

// isolatedEvaluatorEnv builds the environment for the subprocess. We
// deliberately strip everything that could carry context from the
// coding CLI (the Builder). We pass through only what underlying tools
// (npx, eslint, jest, playwright, etc.) actually need to function.
//
// The allowlist approach is critical: a denylist would forever risk
// missing some new env var that leaks state. Allowlist means new vars
// stay out by default.
func isolatedEvaluatorEnv(repoRoot string, acceptScreenshots bool) []string {
	allowed := map[string]bool{
		"PATH":         true, // find binaries
		"HOME":         true, // many tools require this
		"USER":         true,
		"USERPROFILE":  true, // Windows equivalent of HOME
		"LOGNAME":      true,
		"LANG":         true, // locale affects tool output
		"LC_ALL":       true,
		"LC_CTYPE":     true,
		"TMPDIR":       true, // temp files for test runners
		"TEMP":         true,
		"TMP":          true,
		"SHELL":        true,
		"ComSpec":      true, // Windows process spawning
		"COMSPEC":      true,
		"SystemRoot":   true,
		"WINDIR":       true,
		"PATHEXT":      true,
		"APPDATA":      true, // npm and Playwright caches on Windows
		"LOCALAPPDATA": true,

		// Tool-specific paths (read-only resolution, no state).
		"NODE_PATH":                true,
		"NPM_CONFIG_CACHE":         true,
		"npm_config_cache":         true,
		"YARN_CACHE_FOLDER":        true,
		"PNPM_HOME":                true,
		"PLAYWRIGHT_BROWSERS_PATH": true,
		"PIP_CACHE_DIR":            true,
		"CARGO_HOME":               true,
		"RUSTUP_HOME":              true,
		"GOCACHE":                  true,
		"GOPATH":                   true,
		"GOROOT":                   true,
		"GOMODCACHE":               true,
	}

	out := []string{}
	for _, kv := range os.Environ() {
		eq := strings.IndexByte(kv, '=')
		if eq < 0 {
			continue
		}
		k := kv[:eq]
		if allowed[k] {
			out = append(out, kv)
		}
	}

	// Marker so the subprocess can self-identify in error reports and
	// so adapters can tighten behavior (e.g. force CI=1 for playwright).
	out = append(out, "PWD="+repoRoot)
	out = append(out, "HARNESS_ISOLATED=1")
	out = append(out, "CI=1")
	if acceptScreenshots {
		out = append(out, "HARNESS_ACCEPT_SCREENSHOTS=1")
	}
	return out
}

// Consolidate finalizes the sprint: writes the score, appends to
// progress.md, records the run in memory.db. Called after RunQA.
func (m *Manager) Consolidate() (*ConsolidatedReport, error) {
	n, err := m.currentSprintNumber()
	if err != nil {
		return nil, err
	}
	reportPath := filepath.Join(m.root, "reports", fmt.Sprintf("sprint-%03d.json", n))
	b, err := os.ReadFile(reportPath)
	if err != nil {
		return nil, fmt.Errorf("read report: %w", err)
	}
	var ev evaluator.EvaluationResult
	if err := json.Unmarshal(b, &ev); err != nil {
		return nil, err
	}

	// Record in memory.db.
	runID, err := m.mem.InsertRun(memory.Run{
		Timestamp:    ev.Timestamp,
		SprintNumber: n,
		Trigger:      "manual",
		Verdict:      ev.Verdict,
		ScoreTotal:   ev.TotalScore,
		Scores:       extractDimScores(ev.Dimensions),
	})
	if err != nil {
		return nil, err
	}
	for _, d := range ev.Dimensions {
		for _, f := range d.Findings {
			_ = m.mem.InsertFinding(memory.Finding{
				ID:          fmt.Sprintf("run%d-%s", runID, f.Fingerprint),
				RunID:       runID,
				Dimension:   string(f.Dimension),
				Severity:    string(f.Severity),
				File:        f.File,
				Line:        f.Line,
				Rule:        f.Rule,
				Message:     f.Message,
				Fingerprint: f.Fingerprint,
			})
		}
	}

	// Append to progress.md (the narrative brain).
	if err := m.appendProgress(n, &ev); err != nil {
		return nil, err
	}

	return &ConsolidatedReport{
		SprintNumber: n,
		Score:        evScore{Total: ev.TotalScore},
		Verdict:      ev.Verdict,
		Path:         reportPath,
		EvaluationPath: filepath.Join(m.root, "evaluations",
			fmt.Sprintf("sprint-%03d.md", n)),
	}, nil
}

// List returns all completed sprints with their scores.
func (m *Manager) List() ([]SprintListItem, error) {
	dir := filepath.Join(m.root, "reports")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []SprintListItem
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "sprint-") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var ev evaluator.EvaluationResult
		if err := json.Unmarshal(b, &ev); err != nil {
			continue
		}
		goal := ""
		cpath := filepath.Join(m.root, "contracts",
			strings.Replace(e.Name(), ".json", ".md", 1))
		if c, err := planner.Parse(cpath); err == nil {
			goal = c.Title
		}
		out = append(out, SprintListItem{
			Number:  ev.SprintNumber,
			Goal:    goal,
			Score:   ev.TotalScore,
			Verdict: ev.Verdict,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Number < out[j].Number })
	return out, nil
}

// nextSprintNumber returns the integer for the next sprint to create.
func (m *Manager) nextSprintNumber() (int, error) {
	n, err := m.currentSprintNumber()
	if err != nil {
		return 0, err
	}
	return n + 1, nil
}

// currentSprintNumber returns the highest existing sprint number, or 0.
func (m *Manager) currentSprintNumber() (int, error) {
	dir := filepath.Join(m.root, "contracts")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	re := regexp.MustCompile(`sprint-(\d+)\.md`)
	max := 0
	for _, e := range entries {
		m := re.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		var n int
		fmt.Sscanf(m[1], "%d", &n)
		if n > max {
			max = n
		}
	}
	return max, nil
}

func (m *Manager) writeEvaluationMarkdown(n int, c *planner.Contract,
	r *evaluator.EvaluationResult) error {
	path := filepath.Join(m.root, "evaluations", fmt.Sprintf("sprint-%03d.md", n))
	var sb strings.Builder
	fmt.Fprintf(&sb, "# Sprint %03d — Evaluation", n)
	if c != nil && c.Title != "" {
		fmt.Fprintf(&sb, " · %s", c.Title)
	}
	fmt.Fprintf(&sb, "\n\n")
	fmt.Fprintf(&sb, "## Verdict: %s\n\n", r.Verdict)
	fmt.Fprintf(&sb, "## Scores\n")
	fmt.Fprintf(&sb, "| Dimension     | Score | Threshold | Passed | Findings |\n")
	fmt.Fprintf(&sb, "|---------------|-------|-----------|--------|----------|\n")
	dimOrder := []string{"correctness", "coverage", "complexity", "security",
		"architecture", "contract", "e2e"}
	for _, name := range dimOrder {
		d, ok := r.Dimensions[name]
		if !ok {
			continue
		}
		passMark := "❌"
		if d.Passed {
			passMark = "✅"
		}
		fmt.Fprintf(&sb, "| %-13s | %3d   | %3d       | %s     | %d        |\n",
			name, d.Score, d.Threshold, passMark, len(d.Findings))
	}
	fmt.Fprintf(&sb, "\n## Aggregate Score: %d/100\n\n", r.TotalScore)
	if len(r.Sensors) > 0 {
		fmt.Fprintf(&sb, "## Sensors\n")
		fmt.Fprintf(&sb, "| Sensor | Dimension | Registered | Available | Executed | Error |\n")
		fmt.Fprintf(&sb, "|--------|-----------|------------|-----------|----------|-------|\n")
		for _, s := range r.Sensors {
			fmt.Fprintf(&sb, "| %s | %s | %t | %t | %t | %s |\n",
				s.Name, s.Dimension, s.Registered, s.Available, s.Executed, s.Error)
		}
		fmt.Fprintln(&sb)
	}
	if r.ContractCheck.Status != "satisfied" {
		fmt.Fprintf(&sb, "## Contract Status: %s\n", r.ContractCheck.Status)
		for _, d := range r.ContractCheck.MissingDeliverables {
			fmt.Fprintf(&sb, "- missing: %s\n", d)
		}
		fmt.Fprintln(&sb)
	}
	for _, name := range dimOrder {
		d, ok := r.Dimensions[name]
		if !ok || len(d.Findings) == 0 {
			continue
		}
		fmt.Fprintf(&sb, "## Findings — %s\n\n", name)
		for _, f := range d.Findings {
			fmt.Fprintf(&sb, "- [%s] %s:%d  %s  %s\n",
				f.Severity, f.File, f.Line, f.Rule, f.Message)
		}
		fmt.Fprintln(&sb)
	}
	return os.WriteFile(path, []byte(sb.String()), 0o644)
}

func (m *Manager) writeJSONReport(n int, r *evaluator.EvaluationResult) error {
	dir := filepath.Join(m.root, "reports")
	_ = os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, fmt.Sprintf("sprint-%03d.json", n))
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return err
	}
	// Update latest.json symlink (or plain copy on systems without symlinks).
	latest := filepath.Join(dir, "latest.json")
	_ = os.Remove(latest)
	if err := os.Symlink(filepath.Base(path), latest); err != nil {
		_ = os.WriteFile(latest, b, 0o644)
	}
	return nil
}

func (m *Manager) appendProgress(n int, r *evaluator.EvaluationResult) error {
	path := filepath.Join(m.root, "progress.md")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	date := time.Now().Format("2006-01-02")
	verdictMark := "✅"
	if r.Verdict != "PASS" {
		verdictMark = "❌"
	}
	fmt.Fprintf(f, "\n### Sprint %03d (%s) %s\n", n, date, verdictMark)
	fmt.Fprintf(f, "- Verdict: %s\n", r.Verdict)
	fmt.Fprintf(f, "- Score: %d/100\n", r.TotalScore)
	failedDims := []string{}
	for name, d := range r.Dimensions {
		if !d.Passed {
			failedDims = append(failedDims, fmt.Sprintf("%s (%d<%d)", name, d.Score, d.Threshold))
		}
	}
	if len(failedDims) > 0 {
		sort.Strings(failedDims)
		fmt.Fprintf(f, "- Failed dimensions: %s\n", strings.Join(failedDims, ", "))
	}
	return nil
}

func extractDimScores(dims map[string]evaluator.DimensionScore) map[string]int {
	out := map[string]int{}
	for k, v := range dims {
		out[k] = v.Score
	}
	return out
}

// QAResult is the in-process result of a QA run. Provides format adapters
// for the CLI to print it either as TTY or JSON.
type QAResult struct {
	result         *evaluator.EvaluationResult
	sprintNumber   int
	evaluationPath string
	reportPath     string
}

// ConsolidatedReport is what `harness sprint score` returns.
type ConsolidatedReport struct {
	SprintNumber   int
	Score          evScore
	Verdict        string
	Path           string
	EvaluationPath string
}

type evScore struct{ Total int }

// SprintListItem is one row in `harness sprint list`.
type SprintListItem struct {
	Number  int
	Goal    string
	Score   int
	Verdict string
}
