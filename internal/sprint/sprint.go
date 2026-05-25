// Package sprint orchestrates the Contract → Build → QA → Score lifecycle.
//
// Each sprint advances through four states, recorded in memory.db and
// surfaced via the TUI:
//
//	Contract:  user/CLI writes .specs/features/sprint-NNN/spec.md
//	Build:     CLI implements the feature (harness has no role here)
//	QA:        Evaluator subprocess runs sensors, emits evaluations/sprint-NNN.md
//	Score:     Manager consolidates, writes report, appends to .specs/project/STATE.md
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
	"github.com/dancampari/harness/internal/agreement"
	"github.com/dancampari/harness/internal/config"
	"github.com/dancampari/harness/internal/evaluator"
	"github.com/dancampari/harness/internal/memory"
	"github.com/dancampari/harness/internal/planner"
	"github.com/dancampari/harness/internal/progress"
	"github.com/dancampari/harness/internal/sensors"
	"github.com/dancampari/harness/internal/traceability"
	"github.com/dancampari/harness/internal/workspace"
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
	if err := db.Migrate(); err != nil {
		return nil, fmt.Errorf("migrate memory: %w", err)
	}
	return &Manager{root: harnessDir, cfg: cfg, mem: db}, nil
}

// Close releases the underlying memory database. Production callers can
// usually rely on process exit, but tests and long-running services should
// call Close so file handles do not leak.
func (m *Manager) Close() error {
	if m == nil || m.mem == nil {
		return nil
	}
	return m.mem.Close()
}

// NewContract creates the next sprint contract from a goal.
// Returns the file path and the assigned sprint number.
//
// New contracts are written under .specs/features/sprint-NNN/spec.md
// (TLC's canonical location). Legacy projects continue to read existing
// contracts from .harness/contracts/ via agreement.Manager's dual-layout
// resolver until `harness upgrade` migrates them.
func (m *Manager) NewContract(goal string) (string, int, error) {
	next, err := m.nextSprintNumber()
	if err != nil {
		return "", 0, err
	}
	path := agreement.NewManager(m.root).CanonicalContractPath(next)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", 0, err
	}
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
	var ag agreement.Status
	agMgr := agreement.NewManager(m.root)
	cpath := agMgr.ContractPath(n)
	if c, err := planner.Parse(cpath); err == nil {
		st.Goal = c.Title
		if agStatus, err := agMgr.Status(n); err == nil {
			ag = agStatus
			st.Contract = ag.State
		}
	}
	reportPath := filepath.Join(m.root, "reports", fmt.Sprintf("sprint-%03d.json", n))
	if b, err := os.ReadFile(reportPath); err == nil {
		var ev evaluator.EvaluationResult
		if json.Unmarshal(b, &ev) == nil {
			st.Build = "done"
			st.QA = strings.ToLower(ev.Verdict)
			st.Score = fmt.Sprintf("%d/100", ev.TotalScore)
			if !ag.ReportIsCurrent(ev.Timestamp) {
				st.Build = "stale"
				st.QA = "stale"
				st.Score = "-"
			}
			return st, nil
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
		if !strings.EqualFold(st.Contract, "agreed") {
			st.Build = "stale"
			st.QA = "stale"
			st.Score = "-"
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
func (m *Manager) RunQA(acceptScreenshots, acceptFixtures bool) (*QAResult, error) {
	return m.RunQAWith(QAOptions{
		AcceptScreenshots: acceptScreenshots,
		AcceptFixtures:    acceptFixtures,
	})
}

// QAOptions controls one isolated QA pass. Fast skips slow sensors so the
// pre-commit hook can complete in seconds; full runs still use the zero
// value to keep the existing contract.
type QAOptions struct {
	AcceptScreenshots bool
	AcceptFixtures    bool
	Fast              bool
}

// RunQAWith spawns the evaluator subprocess with explicit options. The
// fast flag short-circuits dimensions whose configured sensors are not
// in sensors.IsFast, marking them Skipped instead of running them.
func (m *Manager) RunQAWith(opts QAOptions) (*QAResult, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("locate harness binary: %w", err)
	}
	repoRoot, err := filepath.Abs(filepath.Dir(m.root))
	if err != nil {
		return nil, fmt.Errorf("resolve repo root: %w", err)
	}

	timeout := 16 * time.Minute
	if opts.Fast {
		timeout = 2 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	args := []string{"sprint", "qa", "--internal"}
	if opts.Fast {
		args = append(args, "--fast")
	}
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Stdin = nil        // sever any inherited stdin (no terminal access)
	cmd.Stderr = os.Stderr // forward subprocess diagnostics
	cmd.Dir = repoRoot     // explicit working dir, not inherited
	cmd.Env = isolatedEvaluatorEnv(repoRoot, opts.AcceptScreenshots, opts.AcceptFixtures)

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
func (m *Manager) RunQAInternal(stdout io.Writer, acceptScreenshots, acceptFixtures bool) error {
	return m.RunQAInternalWith(stdout, QAOptions{
		AcceptScreenshots: acceptScreenshots,
		AcceptFixtures:    acceptFixtures,
	})
}

// RunQAInternalWith is the child-side worker for the parent's RunQAWith.
// Fast runs filter the configured sensors to fast-only and write
// reports/evaluations exactly like a full run does, except dimensions
// without a fast sensor are marked Skipped.
func (m *Manager) RunQAInternalWith(stdout io.Writer, opts QAOptions) error {
	if opts.AcceptScreenshots {
		_ = os.Setenv("HARNESS_ACCEPT_SCREENSHOTS", "1")
	}
	if opts.AcceptFixtures {
		_ = os.Setenv("HARNESS_ACCEPT_FIXTURES", "1")
	}
	n, err := m.currentSprintNumber()
	if err != nil {
		return err
	}
	cpath := agreement.NewManager(m.root).ContractPath(n)
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

	// Publish a live progress snapshot so the TUI (and any agent
	// watching the run) sees the contract phase and the per-sensor
	// checklist in real time. NewWriter flushes the contract phase
	// immediately; the evaluator advances it through sensors and done.
	prog := progress.NewWriter(filepath.Join(m.root, "run-progress.json"), n)

	check := contract.CheckAgainstDiff(repoRoot)
	ev := evaluator.New(m.cfg, adapters.BuildRegistry())
	runTimeout := 15 * time.Minute
	if opts.Fast {
		runTimeout = 90 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()
	result, err := ev.EvaluateWith(ctx, repoRoot, n, check, evaluator.Options{
		Fast:     opts.Fast,
		Progress: prog,
	})
	if err != nil {
		return err
	}

	// Fast runs are informational shift-left checks. They must not
	// overwrite the full QA report artifacts that score consolidation
	// relies on, so emit the JSON to stdout and skip writing reports,
	// evaluations, and repair briefs to disk.
	if !opts.Fast {
		if err := m.writeEvaluationMarkdown(n, contract, result); err != nil {
			return err
		}
		if err := m.writeJSONReport(n, result); err != nil {
			return err
		}
		if result.Verdict == "PASS" {
			_ = m.clearRepairBrief(n)
		} else if _, err := m.writeRepairBriefFromResult(n, result); err != nil {
			return err
		}
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
func isolatedEvaluatorEnv(repoRoot string, acceptScreenshots, acceptFixtures bool) []string {
	allowed := map[string]bool{
		"PATH":         true, // find binaries
		"Path":         true, // Windows preserves this casing in many shells
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
	pathKey := "PATH"
	pathValue := ""
	for _, kv := range os.Environ() {
		eq := strings.IndexByte(kv, '=')
		if eq < 0 {
			continue
		}
		k := kv[:eq]
		v := kv[eq+1:]
		if strings.EqualFold(k, "PATH") {
			pathKey = k
			pathValue = v
			continue
		}
		if allowed[k] {
			out = append(out, kv)
		}
	}

	// Marker so the subprocess can self-identify in error reports and
	// so adapters can tighten behavior (e.g. force CI=1 for playwright).
	out = append(out, pathKey+"="+isolatedEvaluatorPath(repoRoot, pathValue))
	out = append(out, "PWD="+repoRoot)
	out = append(out, "HARNESS_ISOLATED=1")
	out = append(out, "CI=1")
	if acceptScreenshots {
		out = append(out, "HARNESS_ACCEPT_SCREENSHOTS=1")
	}
	if acceptFixtures {
		out = append(out, "HARNESS_ACCEPT_FIXTURES=1")
	}
	return out
}

func isolatedEvaluatorPath(repoRoot, current string) string {
	entries := []string{}
	seen := map[string]bool{}
	add := func(entry string) {
		entry = strings.TrimSpace(strings.Trim(entry, `"`))
		if entry == "" || entry == "." {
			return
		}
		if !filepath.IsAbs(entry) {
			entry = filepath.Join(repoRoot, entry)
			if _, err := os.Stat(entry); err != nil {
				return
			}
		}
		entry = filepath.Clean(entry)
		key := strings.ToLower(entry)
		if seen[key] {
			return
		}
		seen[key] = true
		entries = append(entries, entry)
	}

	add(filepath.Join(repoRoot, "node_modules", ".bin"))
	for _, entry := range filepath.SplitList(current) {
		add(entry)
	}
	return strings.Join(entries, string(os.PathListSeparator))
}

// Consolidate finalizes the sprint: writes the score, appends to
// STATE.md, records the run in memory.db. Called after RunQA.
func (m *Manager) Consolidate(allowFail bool) (*ConsolidatedReport, error) {
	n, err := m.currentSprintNumber()
	if err != nil {
		return nil, err
	}
	ag, err := agreement.NewManager(m.root).Status(n)
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
	if !ag.ReportIsCurrent(ev.Timestamp) {
		return nil, fmt.Errorf("contract agreement required before scoring: sprint %03d report is stale or unagreed; run contract propose/approve, then rerun harness sprint qa",
			n)
	}
	if ev.Process.WorkspaceSHA != "" {
		repoRoot, repoErr := filepath.Abs(filepath.Dir(m.root))
		if repoErr == nil {
			currentSHA, hashErr := workspace.Hash(repoRoot)
			if hashErr == nil && currentSHA != ev.Process.WorkspaceSHA {
				return nil, fmt.Errorf("workspace changed after QA for sprint %03d: rerun harness sprint qa before consolidating; report SHA=%s current SHA=%s",
					n, shortSHA(ev.Process.WorkspaceSHA), shortSHA(currentSHA))
			}
		}
	}
	if ev.Verdict != "PASS" && !allowFail {
		if _, err := m.writeRepairBriefFromResult(n, &ev); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("sprint %03d cannot be scored because QA verdict is %s; read .harness/repairs/latest.md, fix findings, rerun harness sprint qa, and score only after PASS",
			n, ev.Verdict)
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

	// Append to STATE.md (the narrative brain).
	if err := m.appendProgress(n, &ev); err != nil {
		return nil, err
	}

	// Mark every requirement Verified in the traceability ledger so
	// the live TUI and trend tooling see closure. Best-effort: a
	// missing ledger or a transient I/O error must not block scoring.
	if ev.Verdict == "PASS" {
		agreement.NewManager(m.root).AdvanceTraceabilityTo(n, traceability.StatusVerified)
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

// WriteRepairBrief creates an actionable repair brief from the latest current
// QA report. Agents read this after FAIL and iterate until QA returns PASS.
func (m *Manager) WriteRepairBrief() (*RepairBrief, error) {
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
	if ev.Verdict == "PASS" {
		_ = m.clearRepairBrief(n)
		return &RepairBrief{
			SprintNumber: n,
			Verdict:      ev.Verdict,
			TotalScore:   ev.TotalScore,
		}, nil
	}
	return m.writeRepairBriefFromResult(n, &ev)
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
		if c, err := planner.Parse(agreement.NewManager(m.root).ContractPath(ev.SprintNumber)); err == nil {
			goal = c.Title
		}
		out = append(out, SprintListItem{
			Number:  ev.SprintNumber,
			Goal:    goal,
			Score:   ev.TotalScore,
			Verdict: ev.Verdict,
		})
		if ag, err := agreement.NewManager(m.root).Status(ev.SprintNumber); err == nil && !ag.ReportIsCurrent(ev.Timestamp) {
			out[len(out)-1].Score = 0
			out[len(out)-1].Verdict = "STALE"
		}
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
// Delegates to agreement.Manager so the same dual-layout (.specs/ canonical
// + .harness/contracts legacy) scan is shared.
func (m *Manager) currentSprintNumber() (int, error) {
	return agreement.NewManager(m.root).CurrentSprintNumber()
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
		"architecture", "behavior", "contract", "e2e"}
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
	path := projectStatePath(m.root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
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

func projectStatePath(harnessDir string) string {
	clean := filepath.Clean(harnessDir)
	parent := filepath.Dir(clean)
	if parent == "" || parent == "." {
		return filepath.Join(".specs", "project", "STATE.md")
	}
	return filepath.Join(parent, ".specs", "project", "STATE.md")
}

func (m *Manager) writeRepairBriefFromResult(n int, r *evaluator.EvaluationResult) (*RepairBrief, error) {
	dir := filepath.Join(m.root, "repairs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, fmt.Sprintf("sprint-%03d.md", n))
	latest := filepath.Join(dir, "latest.md")
	content := m.renderRepairBrief(n, r)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(latest, []byte(content), 0o644); err != nil {
		return nil, err
	}
	return &RepairBrief{
		SprintNumber: n,
		Verdict:      r.Verdict,
		TotalScore:   r.TotalScore,
		Path:         path,
		LatestPath:   latest,
	}, nil
}

func (m *Manager) clearRepairBrief(n int) error {
	dir := filepath.Join(m.root, "repairs")
	_ = os.Remove(filepath.Join(dir, "latest.md"))
	_ = os.Remove(filepath.Join(dir, fmt.Sprintf("sprint-%03d.md", n)))
	return nil
}

func (m *Manager) renderRepairBrief(n int, r *evaluator.EvaluationResult) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "# Sprint %03d Repair Brief\n\n", n)
	fmt.Fprintf(&sb, "## Status\n")
	fmt.Fprintf(&sb, "- Verdict: %s\n", r.Verdict)
	fmt.Fprintf(&sb, "- Score: %d/100\n", r.TotalScore)
	fmt.Fprintf(&sb, "- Report: `.harness/reports/sprint-%03d.json`\n", n)
	fmt.Fprintf(&sb, "- Evaluation: `.harness/evaluations/sprint-%03d.md`\n\n", n)

	if r.Verdict == "PASS" {
		fmt.Fprintf(&sb, "No repair required. Run `harness sprint score` to consolidate the sprint.\n")
		return sb.String()
	}

	fmt.Fprintf(&sb, "## Required Agent Loop\n")
	fmt.Fprintf(&sb, "1. Fix every actionable finding below without changing the agreed contract.\n")
	fmt.Fprintf(&sb, "2. Run `harness sprint qa --format=json` after the fix.\n")
	fmt.Fprintf(&sb, "3. If QA still returns FAIL, reread `.harness/repairs/latest.md` and repeat.\n")
	fmt.Fprintf(&sb, "4. Run `harness sprint score` only after QA returns PASS.\n")
	fmt.Fprintf(&sb, "5. Do not declare the sprint complete while this file contains unresolved findings.\n\n")

	flat := flattenedFindings(r)
	if len(flat) == 0 {
		fmt.Fprintf(&sb, "## Findings\n")
		fmt.Fprintf(&sb, "- QA failed without structured findings. Inspect `.harness/reports/sprint-%03d.json`, fix the failing sensor output, and rerun QA.\n\n", n)
		return sb.String()
	}

	fmt.Fprintf(&sb, "## Findings\n")
	fmt.Fprintf(&sb, "| Severity | Dimension | Rule | Location | Action |\n")
	fmt.Fprintf(&sb, "|---|---|---|---|---|\n")
	hintsSeen := map[string]string{}
	for _, f := range flat {
		fmt.Fprintf(&sb, "| %s | %s | %s | %s | %s |\n",
			escapeTable(f.severity),
			escapeTable(f.dimension),
			escapeTable(f.rule),
			escapeTable(f.location),
			escapeTable(repairAction(f)),
		)
		if f.hint != "" {
			hintsSeen[f.rule] = f.hint
		}
	}
	fmt.Fprintln(&sb)

	// Agent-readable hint catalog. We dedupe by rule so a sprint with
	// 50 lint findings does not repeat the same Suggested fix 50 times.
	if len(hintsSeen) > 0 {
		fmt.Fprintf(&sb, "## Suggested Fixes (LLM-optimized)\n\n")
		ruleOrder := make([]string, 0, len(hintsSeen))
		for rule := range hintsSeen {
			ruleOrder = append(ruleOrder, rule)
		}
		sort.Strings(ruleOrder)
		for _, rule := range ruleOrder {
			fmt.Fprintf(&sb, "- **%s** — %s\n", rule, hintsSeen[rule])
		}
		fmt.Fprintln(&sb)
	}

	if humanApprovalRequired(flat) {
		fmt.Fprintf(&sb, "## User Approval Required\n")
		if visualApprovalRequired(flat) {
			fmt.Fprintf(&sb, "- A screenshot baseline is missing or visual output changed. The agent must not accept it silently.\n")
			fmt.Fprintf(&sb, "- Ask the user to review `.harness/screenshots/current/` and, for regressions, `.harness/screenshots/diff/`.\n")
			fmt.Fprintf(&sb, "- Only after explicit approval, run `harness sprint qa --accept-screenshots`.\n")
		}
		if fixtureApprovalRequired(flat) {
			fmt.Fprintf(&sb, "- An approved behavior fixture is missing or changed. The agent must not accept it silently.\n")
			fmt.Fprintf(&sb, "- Ask the user to review `.harness/fixtures/` and the behavior change.\n")
			fmt.Fprintf(&sb, "- Only after explicit approval, run `harness sprint qa --accept-fixtures`.\n")
		}
		fmt.Fprintln(&sb)
	}

	fmt.Fprintf(&sb, "## Agent Notes\n")
	fmt.Fprintf(&sb, "- Fix product code, tests, config, or contract deliverables only when they are the real cause.\n")
	fmt.Fprintf(&sb, "- Do not lower thresholds or remove acceptance criteria to make QA pass.\n")
	fmt.Fprintf(&sb, "- If a required tool is missing, run `harness doctor` and install only the project dependency that matches the stack.\n")
	return sb.String()
}

type repairFinding struct {
	severity  string
	dimension string
	rule      string
	location  string
	message   string
	hint      string
}

func flattenedFindings(r *evaluator.EvaluationResult) []repairFinding {
	var out []repairFinding
	if r.ContractCheck.Status != "" && r.ContractCheck.Status != "satisfied" {
		for _, item := range r.ContractCheck.MissingDeliverables {
			out = append(out, repairFinding{
				severity:  "high",
				dimension: "contract",
				rule:      "missing-deliverable",
				location:  "-",
				message:   item,
				hint:      sensors.LLMHint("missing-deliverable"),
			})
		}
		for _, item := range r.ContractCheck.UnmetCriteria {
			out = append(out, repairFinding{
				severity:  "high",
				dimension: "contract",
				rule:      "unmet-criterion",
				location:  "-",
				message:   item,
				hint:      sensors.LLMHint("unmet-criterion"),
			})
		}
	}
	for dimName, d := range r.Dimensions {
		for _, f := range d.Findings {
			location := f.File
			if location == "" {
				location = "-"
			}
			if f.Line > 0 {
				location = fmt.Sprintf("%s:%d", location, f.Line)
			}
			out = append(out, repairFinding{
				severity:  string(f.Severity),
				dimension: dimName,
				rule:      f.Rule,
				location:  filepath.ToSlash(location),
				message:   f.Message,
				hint:      f.Hint,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if repairSevRank(out[i].severity) == repairSevRank(out[j].severity) {
			if out[i].dimension == out[j].dimension {
				return out[i].rule < out[j].rule
			}
			return out[i].dimension < out[j].dimension
		}
		return repairSevRank(out[i].severity) > repairSevRank(out[j].severity)
	})
	return out
}

func repairAction(f repairFinding) string {
	switch f.rule {
	case "screenshot-baseline-missing":
		return fmt.Sprintf("Review %s with the user. If correct, rerun QA with --accept-screenshots.", f.location)
	case "visual-regression":
		return fmt.Sprintf("Inspect %s and .harness/screenshots/diff; fix UI regression or ask user to approve the new baseline.", f.location)
	case "fixture-baseline-missing":
		return fmt.Sprintf("Review %s with the user. If correct, rerun QA with --accept-fixtures.", f.location)
	case "fixture-regression":
		return "Fix the behavior regression or ask the user to approve the changed fixture with --accept-fixtures."
	case "no-approved-fixtures":
		return "Add approved JSON fixtures under .harness/fixtures or disable behavior by setting threshold and weight to 0."
	case "missing-sensor":
		return "Run harness doctor, install/configure the missing sensor, then rerun QA."
	case "missing-deliverable", "unmet-criterion":
		return f.message
	case "no-e2e-tests":
		return "Add real Playwright coverage for the primary flow or disable e2e by setting threshold and weight to 0."
	case "e2e-failure":
		return "Fix the browser flow or selector failure reported by Playwright."
	case "test-failure":
		return "Fix the failing test or the implementation under test."
	case "coverage-below-threshold":
		return "Add meaningful tests for uncovered behavior."
	default:
		if f.message != "" {
			return f.message
		}
		return "Fix this finding and rerun harness sprint qa."
	}
}

func humanApprovalRequired(findings []repairFinding) bool {
	return visualApprovalRequired(findings) || fixtureApprovalRequired(findings)
}

func visualApprovalRequired(findings []repairFinding) bool {
	for _, f := range findings {
		if f.rule == "screenshot-baseline-missing" || f.rule == "visual-regression" {
			return true
		}
	}
	return false
}

func fixtureApprovalRequired(findings []repairFinding) bool {
	for _, f := range findings {
		if f.rule == "fixture-baseline-missing" || f.rule == "fixture-regression" {
			return true
		}
	}
	return false
}

func escapeTable(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	return value
}

func repairSevRank(severity string) int {
	switch severity {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func shortSHA(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
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

// RepairBrief points agents at the current failure loop instructions.
type RepairBrief struct {
	SprintNumber int
	Verdict      string
	TotalScore   int
	Path         string
	LatestPath   string
}

// SprintListItem is one row in `harness sprint list`.
type SprintListItem struct {
	Number  int
	Goal    string
	Score   int
	Verdict string
}
