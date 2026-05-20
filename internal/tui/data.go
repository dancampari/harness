package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dancampari/harness/internal/agreement"
	"github.com/dancampari/harness/internal/config"
	"github.com/dancampari/harness/internal/detect"
	"github.com/dancampari/harness/internal/evaluator"
	"github.com/dancampari/harness/internal/planner"
)

type DashboardData struct {
	Project        ProjectProfile
	Current        RunRecord
	Runs           []RunRecord
	Events         []ActivityEvent
	Commands       []string
	ReportMarkdown string
	ReportPath     string
	Skills         SkillState
	Doctor         DoctorState
	LastEvent      string
	LastSeen       time.Time
}

type ProjectProfile struct {
	Name           string
	Stack          string
	PackageManager string
	Frameworks     []string
	CodingCLIs     []string
	Agent          string
	Status         string
	Branch         string
	PlanningMode   string
}

type RunRecord struct {
	RunID       string             `json:"runId"`
	Number      int                `json:"-"`
	Feature     string             `json:"feature"`
	Agent       string             `json:"agent"`
	Status      string             `json:"status"`
	Score       int                `json:"score"`
	StartedAt   time.Time          `json:"-"`
	FinishedAt  time.Time          `json:"-"`
	Runtime     string             `json:"runtime"`
	UpdatedAt   time.Time          `json:"-"`
	Branch      string             `json:"branch"`
	ReportPath  string             `json:"reportPath,omitempty"`
	Validations map[string]string  `json:"validations"`
	Quality     []QualityDimension `json:"quality"`
	Findings    int                `json:"findings"`
	Stale       bool               `json:"stale"`
}

type QualityDimension struct {
	Dimension string `json:"dimension"`
	Score     int    `json:"score"`
	Threshold int    `json:"threshold"`
	Status    string `json:"status"`
	Findings  int    `json:"findings"`
	Sensors   string `json:"sensors,omitempty"`
}

type ActivityEvent struct {
	Timestamp time.Time      `json:"-"`
	Type      string         `json:"type"`
	Message   string         `json:"message"`
	Agent     string         `json:"agent"`
	Metadata  map[string]any `json:"metadata"`
}

type SkillState struct {
	Active     []string
	Suggested  []string
	Categories []string
	Adapters   []string
}

type DoctorState struct {
	Stack          string
	PackageManager string
	Scripts        []string
	Validations    []string
	Alerts         []string
	Files          []string
	Risks          []string
}

type runJSON struct {
	RunID       string             `json:"runId"`
	Feature     string             `json:"feature"`
	Agent       string             `json:"agent"`
	Status      string             `json:"status"`
	Score       int                `json:"score"`
	StartedAt   *time.Time         `json:"startedAt"`
	FinishedAt  *time.Time         `json:"finishedAt"`
	Runtime     string             `json:"runtime"`
	UpdatedAt   *time.Time         `json:"updatedAt"`
	Branch      string             `json:"branch"`
	ReportPath  string             `json:"reportPath"`
	Validations map[string]string  `json:"validations"`
	Quality     []QualityDimension `json:"quality"`
	Findings    int                `json:"findings"`
}

func loadDashboardData(harnessDir string) DashboardData {
	projectRoot := projectRootForHarness(harnessDir)
	project := loadProjectProfile(harnessDir, projectRoot)
	runs := loadRunRecords(harnessDir)
	if len(runs) == 0 {
		runs = loadLegacyRuns(harnessDir)
	}
	sort.SliceStable(runs, func(i, j int) bool {
		ti := runs[i].UpdatedAt
		if ti.IsZero() {
			ti = runs[i].StartedAt
		}
		tj := runs[j].UpdatedAt
		if tj.IsZero() {
			tj = runs[j].StartedAt
		}
		if ti.Equal(tj) {
			return runs[i].Number > runs[j].Number
		}
		return ti.After(tj)
	})
	current := loadCurrentRun(harnessDir)
	if current.RunID == "" && len(runs) > 0 {
		current = runs[0]
	}
	if current.Agent == "" {
		current.Agent = project.Agent
	}
	if current.Branch == "" {
		current.Branch = project.Branch
	}
	if current.Status == "" {
		current.Status = "idle"
	}
	project.Status = current.Status
	if current.Agent != "" {
		project.Agent = current.Agent
	}

	reportMarkdown, reportPath := loadReportMarkdown(harnessDir, current)
	events := loadRecentEvents(harnessDir, current, 80)
	if len(events) == 0 {
		events = legacyEventsFromState(harnessDir, current)
	}
	commands := loadRecentCommands(harnessDir, current, 40)
	skills := loadSkillState(harnessDir)
	doctor := loadDoctorState(harnessDir, projectRoot)
	lastSeen := newestArtifactTime(harnessDir)
	lastEvent := newestArtifactLabel(harnessDir)
	if lastSeen.IsZero() {
		lastSeen = time.Now()
	}
	if lastEvent == "" {
		lastEvent = "watching .harness"
	}
	return DashboardData{
		Project:        project,
		Current:        current,
		Runs:           runs,
		Events:         events,
		Commands:       commands,
		ReportMarkdown: reportMarkdown,
		ReportPath:     reportPath,
		Skills:         skills,
		Doctor:         doctor,
		LastEvent:      lastEvent,
		LastSeen:       lastSeen,
	}
}

func loadProjectProfile(harnessDir, projectRoot string) ProjectProfile {
	info := detect.DetectProject(projectRoot)
	profile := ProjectProfile{
		Name:           defaultString(info.Name, projectName(harnessDir)),
		Stack:          defaultString(info.Stack, "unknown"),
		PackageManager: defaultString(info.PackageManager, "-"),
		Frameworks:     info.Frameworks,
		CodingCLIs:     info.CodingCLIs,
		Agent:          firstNonEmpty(info.CodingCLIs, "manual"),
		Status:         "idle",
		Branch:         gitBranch(projectRoot),
	}
	var setup struct {
		Project      string `json:"project"`
		Stack        string `json:"stack"`
		CodingCLI    string `json:"coding_cli"`
		PlanningMode string `json:"planning_mode"`
	}
	if readJSONFile(filepath.Join(harnessDir, "setup.json"), &setup) == nil {
		if setup.Project != "" {
			profile.Name = setup.Project
		}
		if setup.Stack != "" {
			profile.Stack = setup.Stack
		}
		if setup.CodingCLI != "" && setup.CodingCLI != "none" && setup.CodingCLI != "auto" {
			profile.Agent = setup.CodingCLI
		}
		profile.PlanningMode = setup.PlanningMode
	}
	if profile.Branch == "" {
		profile.Branch = "-"
	}
	return profile
}

func loadCurrentRun(harnessDir string) RunRecord {
	return readRunJSON(filepath.Join(harnessDir, "current-run.json"))
}

func loadRunRecords(harnessDir string) []RunRecord {
	dir := filepath.Join(harnessDir, "runs")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var runs []RunRecord
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		run := readRunJSON(filepath.Join(dir, entry.Name(), "run.json"))
		if run.RunID == "" {
			continue
		}
		if run.ReportPath == "" {
			if p := filepath.Join(dir, entry.Name(), "report.md"); fileExists(p) {
				run.ReportPath = p
			}
		}
		runs = append(runs, run)
	}
	return runs
}

func readRunJSON(path string) RunRecord {
	var raw runJSON
	if readJSONFile(path, &raw) != nil {
		return RunRecord{}
	}
	run := RunRecord{
		RunID:       raw.RunID,
		Feature:     raw.Feature,
		Agent:       raw.Agent,
		Status:      raw.Status,
		Score:       raw.Score,
		Runtime:     raw.Runtime,
		Branch:      raw.Branch,
		ReportPath:  raw.ReportPath,
		Validations: raw.Validations,
		Quality:     raw.Quality,
		Findings:    raw.Findings,
	}
	if raw.StartedAt != nil {
		run.StartedAt = *raw.StartedAt
	}
	if raw.FinishedAt != nil {
		run.FinishedAt = *raw.FinishedAt
	}
	if raw.UpdatedAt != nil {
		run.UpdatedAt = *raw.UpdatedAt
	}
	if run.Validations == nil {
		run.Validations = map[string]string{}
	}
	if run.Findings == 0 {
		for _, q := range run.Quality {
			run.Findings += q.Findings
		}
	}
	return run
}

func loadLegacyRuns(harnessDir string) []RunRecord {
	contractsDir := filepath.Join(harnessDir, "contracts")
	entries, err := os.ReadDir(contractsDir)
	if err != nil {
		return nil
	}
	progress := readOptionalFile(filepath.Join(harnessDir, "progress.md"))
	var runs []RunRecord
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "sprint-") || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		var number int
		fmt.Sscanf(entry.Name(), "sprint-%d.md", &number)
		feature := fmt.Sprintf("Sprint %03d", number)
		if contract, err := planner.Parse(filepath.Join(contractsDir, entry.Name())); err == nil && contract.Title != "" {
			feature = contract.Title
		}
		run := RunRecord{
			RunID:   fmt.Sprintf("sprint-%03d", number),
			Number:  number,
			Feature: feature,
			Status:  "pending",
			Score:   0,
			Runtime: "-",
			Branch:  gitBranch(projectRootForHarness(harnessDir)),
			Validations: map[string]string{
				"contract": "draft",
				"build":    "pending",
				"qa":       "pending",
				"report":   "pending",
				"accept":   "pending",
			},
		}
		ag, _ := agreement.NewManager(harnessDir).Status(number)
		if ag.State != "" {
			run.Validations["contract"] = ag.State
		}
		reportPath := filepath.Join(harnessDir, "reports", fmt.Sprintf("sprint-%03d.json", number))
		if report, ok := readEvaluation(reportPath); ok {
			run.Score = report.TotalScore
			run.Status = strings.ToLower(report.Verdict)
			run.Runtime = compactSeconds(report.DurationSeconds)
			run.UpdatedAt = report.Timestamp
			run.ReportPath = filepath.Join(harnessDir, "evaluations", fmt.Sprintf("sprint-%03d.md", number))
			if !fileExists(run.ReportPath) {
				run.ReportPath = reportPath
			}
			run.Quality = qualityFromEvaluation(report)
			run.Findings = countFindings(report)
			run.Validations["build"] = "done"
			run.Validations["qa"] = strings.ToLower(report.Verdict)
			run.Validations["report"] = "done"
			if strings.Contains(progress, fmt.Sprintf("Sprint %03d", number)) {
				run.Validations["accept"] = "done"
			}
			if !ag.ReportIsCurrent(report.Timestamp) {
				run.Stale = true
				run.Status = "blocked"
				run.Validations["build"] = "stale"
				run.Validations["qa"] = "stale"
				run.Validations["report"] = "stale"
			}
		}
		if run.Status == "pending" && normalizeStatus(run.Validations["contract"]) == "agreed" {
			run.Status = "running"
		}
		runs = append(runs, run)
	}
	return runs
}

func readEvaluation(path string) (evaluator.EvaluationResult, bool) {
	var report evaluator.EvaluationResult
	if readJSONFile(path, &report) != nil {
		return report, false
	}
	return report, true
}

func qualityFromEvaluation(report evaluator.EvaluationResult) []QualityDimension {
	names := orderedDimensionNames(report.Dimensions)
	quality := make([]QualityDimension, 0, len(names))
	for _, name := range names {
		d := report.Dimensions[name]
		status := "fail"
		if d.Passed {
			status = "pass"
		}
		quality = append(quality, QualityDimension{
			Dimension: name,
			Score:     d.Score,
			Threshold: d.Threshold,
			Status:    status,
			Findings:  len(d.Findings),
			Sensors:   strings.Join(d.SensorsUsed, ","),
		})
	}
	return quality
}

func orderedDimensionNames(dims map[string]evaluator.DimensionScore) []string {
	preferred := []string{"correctness", "coverage", "complexity", "security", "architecture", "behavior", "contract", "e2e"}
	seen := map[string]bool{}
	var names []string
	for _, name := range preferred {
		if _, ok := dims[name]; ok {
			names = append(names, name)
			seen[name] = true
		}
	}
	otherStart := len(names)
	for name := range dims {
		if !seen[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names[otherStart:])
	return names
}

func loadReportMarkdown(harnessDir string, current RunRecord) (string, string) {
	candidates := []string{
		filepath.Join(harnessDir, "reports", "latest.md"),
		current.ReportPath,
	}
	if current.Number > 0 {
		candidates = append(candidates, filepath.Join(harnessDir, "evaluations", fmt.Sprintf("sprint-%03d.md", current.Number)))
	}
	for _, path := range candidates {
		if path == "" || filepath.Ext(path) != ".md" {
			continue
		}
		if b, err := os.ReadFile(path); err == nil {
			return string(b), path
		}
	}
	if report, ok := readEvaluation(filepath.Join(harnessDir, "reports", "latest.json")); ok {
		return markdownFromEvaluation(report), filepath.Join(harnessDir, "reports", "latest.json")
	}
	return "", ""
}

func markdownFromEvaluation(report evaluator.EvaluationResult) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Verdict: %s\n\n", report.Verdict))
	sb.WriteString(fmt.Sprintf("Sprint: %03d\n\nScore: %d/100\n\n", report.SprintNumber, report.TotalScore))
	sb.WriteString("| Dimension | Score | Threshold | Status | Findings |\n")
	sb.WriteString("|---|---:|---:|---|---:|\n")
	for _, q := range qualityFromEvaluation(report) {
		sb.WriteString(fmt.Sprintf("| %s | %d | %d | %s | %d |\n",
			q.Dimension, q.Score, q.Threshold, q.Status, q.Findings))
	}
	return sb.String()
}

func loadRecentEvents(harnessDir string, current RunRecord, limit int) []ActivityEvent {
	paths := eventPaths(harnessDir, current)
	var events []ActivityEvent
	for _, path := range paths {
		for _, line := range tailLines(path, limit) {
			var raw struct {
				Timestamp *time.Time     `json:"timestamp"`
				Type      string         `json:"type"`
				Message   string         `json:"message"`
				Agent     string         `json:"agent"`
				Metadata  map[string]any `json:"metadata"`
			}
			if json.Unmarshal([]byte(line), &raw) != nil {
				continue
			}
			ev := ActivityEvent{
				Type:     raw.Type,
				Message:  raw.Message,
				Agent:    raw.Agent,
				Metadata: raw.Metadata,
			}
			if raw.Timestamp != nil {
				ev.Timestamp = *raw.Timestamp
			}
			events = append(events, ev)
		}
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].Timestamp.After(events[j].Timestamp) })
	if len(events) > limit {
		events = events[:limit]
	}
	return events
}

func eventPaths(harnessDir string, current RunRecord) []string {
	var paths []string
	if current.RunID != "" {
		paths = append(paths, filepath.Join(harnessDir, "runs", current.RunID, "events.jsonl"))
	}
	paths = append(paths, filepath.Join(harnessDir, "events.jsonl"))
	runsDir := filepath.Join(harnessDir, "runs")
	entries, err := os.ReadDir(runsDir)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				paths = append(paths, filepath.Join(runsDir, entry.Name(), "events.jsonl"))
			}
		}
	}
	return uniqueExisting(paths)
}

func loadRecentCommands(harnessDir string, current RunRecord, limit int) []string {
	var paths []string
	if current.RunID != "" {
		paths = append(paths, filepath.Join(harnessDir, "runs", current.RunID, "commands.log"))
	}
	paths = append(paths, filepath.Join(harnessDir, "commands.log"))
	var commands []string
	for _, path := range uniqueExisting(paths) {
		commands = append(commands, tailLines(path, limit)...)
	}
	if len(commands) > limit {
		commands = commands[len(commands)-limit:]
	}
	return commands
}

func legacyEventsFromState(harnessDir string, current RunRecord) []ActivityEvent {
	now := time.Now()
	var events []ActivityEvent
	if current.RunID != "" {
		events = append(events, ActivityEvent{
			Timestamp: current.UpdatedAt,
			Type:      "qa.report.updated",
			Message:   fmt.Sprintf("Score: %d/100", current.Score),
			Agent:     current.Agent,
		})
		for _, q := range current.Quality {
			eventType := "validation.passed"
			if normalizeStatus(q.Status) != "pass" {
				eventType = "validation.failed"
			}
			events = append(events, ActivityEvent{
				Timestamp: current.UpdatedAt.Add(-time.Duration(len(events)) * time.Second),
				Type:      eventType,
				Message:   q.Dimension,
				Agent:     current.Agent,
			})
			if len(events) >= 8 {
				return events
			}
		}
	}
	progress := readOptionalFile(filepath.Join(harnessDir, "progress.md"))
	lines := cleanLines(strings.Split(progress, "\n"))
	if len(lines) > 0 {
		if len(lines) > 6 {
			lines = lines[len(lines)-6:]
		}
		for i := len(lines) - 1; i >= 0; i-- {
			events = append(events, ActivityEvent{
				Timestamp: now.Add(-time.Duration(len(events)+1) * time.Second),
				Type:      "progress.note",
				Message:   lines[i],
			})
		}
	}
	if len(events) == 0 {
		events = append(events, ActivityEvent{Timestamp: now, Type: "idle", Message: "Waiting for agent activity"})
	}
	return events
}

func loadSkillState(harnessDir string) SkillState {
	var raw SkillState
	if readJSONFile(filepath.Join(harnessDir, "skills.json"), &raw) == nil {
		return raw
	}
	var active []string
	skillsDir := filepath.Join(harnessDir, "skills")
	if entries, err := os.ReadDir(skillsDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				active = append(active, entry.Name())
			}
		}
	}
	cfg, _ := config.Load(filepath.Join(harnessDir, "config.yaml"))
	return SkillState{
		Active:     active,
		Suggested:  suggestedSkills(cfg),
		Categories: []string{"contract", "spec-driven", "validation", "repair"},
		Adapters:   cfg.AllAdapterNames(),
	}
}

func suggestedSkills(cfg config.Config) []string {
	var out []string
	if cfg.Stack == "node" || cfg.Stack == "typescript" {
		out = append(out, "spec-driven", "contract-review", "approved-fixtures")
	}
	if len(out) == 0 {
		out = append(out, "contract-authoring", "contract-review")
	}
	return out
}

func loadDoctorState(harnessDir, projectRoot string) DoctorState {
	info := detect.DetectProject(projectRoot)
	cfg, cfgErr := config.Load(filepath.Join(harnessDir, "config.yaml"))
	doctor := DoctorState{
		Stack:          defaultString(info.Stack, "unknown"),
		PackageManager: defaultString(info.PackageManager, "-"),
		Scripts:        packageScripts(projectRoot),
		Files:          importantFiles(projectRoot, harnessDir),
	}
	if cfgErr != nil {
		doctor.Alerts = append(doctor.Alerts, "Harness config not found. Run: harness init")
		doctor.Risks = append(doctor.Risks, "quality gates cannot be evaluated without .harness/config.yaml")
	} else {
		doctor.Validations = cfg.ActiveDimensions()
		for _, err := range cfg.Validate() {
			doctor.Alerts = append(doctor.Alerts, err)
		}
		if len(cfg.AllAdapterNames()) == 0 {
			doctor.Risks = append(doctor.Risks, "no adapters configured")
		}
	}
	if len(doctor.Scripts) == 0 && (info.Stack == "node" || info.Stack == "typescript") {
		doctor.Alerts = append(doctor.Alerts, "package.json scripts not detected")
	}
	if len(doctor.Validations) == 0 {
		doctor.Validations = []string{"contract"}
	}
	return doctor
}

func packageScripts(projectRoot string) []string {
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if readJSONFile(filepath.Join(projectRoot, "package.json"), &pkg) != nil {
		return nil
	}
	names := make([]string, 0, len(pkg.Scripts))
	for name := range pkg.Scripts {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func importantFiles(projectRoot, harnessDir string) []string {
	checks := []string{
		filepath.Join(harnessDir, "config.yaml"),
		filepath.Join(harnessDir, "spec.md"),
		filepath.Join(harnessDir, "progress.md"),
		filepath.Join(harnessDir, "agent-protocol.md"),
		filepath.Join(projectRoot, "package.json"),
		filepath.Join(projectRoot, "go.mod"),
		filepath.Join(projectRoot, "AGENTS.md"),
		filepath.Join(projectRoot, "CLAUDE.md"),
		filepath.Join(projectRoot, ".cursor", "rules", "harness.mdc"),
	}
	var found []string
	for _, path := range checks {
		if fileExists(path) {
			if rel, err := filepath.Rel(projectRoot, path); err == nil {
				found = append(found, filepath.ToSlash(rel))
			} else {
				found = append(found, filepath.ToSlash(path))
			}
		}
	}
	return found
}

func countFindings(report evaluator.EvaluationResult) int {
	total := 0
	for _, d := range report.Dimensions {
		total += len(d.Findings)
	}
	return total
}

func compactSeconds(seconds float64) string {
	if seconds <= 0 {
		return "-"
	}
	d := time.Duration(seconds * float64(time.Second)).Round(time.Millisecond)
	if d < time.Second {
		return d.String()
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return d.Round(time.Second).String()
}

func projectRootForHarness(root string) string {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "."
	}
	if filepath.Base(abs) == ".harness" {
		return filepath.Dir(abs)
	}
	return abs
}

func projectName(root string) string {
	return filepath.Base(projectRootForHarness(root))
}

func gitBranch(projectRoot string) string {
	headPath := filepath.Join(projectRoot, ".git", "HEAD")
	b, err := os.ReadFile(headPath)
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(b))
	const prefix = "ref: refs/heads/"
	if strings.HasPrefix(line, prefix) {
		return strings.TrimPrefix(line, prefix)
	}
	if len(line) >= 7 {
		return line[:7]
	}
	return line
}

func newestArtifactTime(root string) time.Time {
	artifacts := watchedArtifacts(root)
	var newest time.Time
	for _, artifact := range artifacts {
		if artifact.mod.After(newest) {
			newest = artifact.mod
		}
	}
	return newest
}

func newestArtifactLabel(root string) string {
	artifacts := watchedArtifacts(root)
	var newest watchedArtifact
	for i, artifact := range artifacts {
		if i == 0 || artifact.mod.After(newest.mod) {
			newest = artifact
		}
	}
	return newest.label
}

type watchedArtifact struct {
	path  string
	label string
	mod   time.Time
	size  int64
}

func watchedArtifacts(root string) []watchedArtifact {
	var artifacts []watchedArtifact
	addFile := func(rel, label string) {
		info, err := os.Stat(filepath.Join(root, rel))
		if err != nil || info.IsDir() {
			return
		}
		artifacts = append(artifacts, watchedArtifact{path: filepath.ToSlash(rel), label: label, mod: info.ModTime(), size: info.Size()})
	}
	addDir := func(rel, label string) {
		entries, err := os.ReadDir(filepath.Join(root, rel))
		if err != nil {
			return
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			artifacts = append(artifacts, watchedArtifact{
				path:  filepath.ToSlash(filepath.Join(rel, entry.Name())),
				label: label,
				mod:   info.ModTime(),
				size:  info.Size(),
			})
		}
	}
	addDirRecursive := func(rel, label string) {
		base := filepath.Join(root, rel)
		_ = filepath.WalkDir(base, func(path string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return nil
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				relative = path
			}
			artifacts = append(artifacts, watchedArtifact{
				path:  filepath.ToSlash(relative),
				label: label,
				mod:   info.ModTime(),
				size:  info.Size(),
			})
			return nil
		})
	}
	addFile("current-run.json", "run updated")
	addFile("config.yaml", "config updated")
	addFile("spec.md", "spec updated")
	addFile("progress.md", "progress scored")
	addDir("contracts", "contract updated")
	addDir("evaluations", "evaluation updated")
	addDir("reports", "qa report updated")
	addDir("repairs", "repair brief updated")
	addDirRecursive("runs", "run artifacts updated")
	return artifacts
}

func tailLines(path string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return nil
	}
	const maxRead int64 = 64 * 1024
	start := info.Size() - maxRead
	if start < 0 {
		start = 0
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	buf := make([]byte, info.Size()-start)
	if _, err := f.ReadAt(buf, start); err != nil && len(buf) == 0 {
		return nil
	}
	if start > 0 {
		if idx := bytes.IndexByte(buf, '\n'); idx >= 0 && idx+1 < len(buf) {
			buf = buf[idx+1:]
		}
	}
	raw := strings.Split(strings.TrimRight(string(buf), "\r\n"), "\n")
	var lines []string
	for _, line := range raw {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	return lines
}

func readOptionalFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

func readJSONFile(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})
	return json.Unmarshal(b, v)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func uniqueExisting(paths []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, path := range paths {
		if path == "" || seen[path] || !fileExists(path) {
			continue
		}
		seen[path] = true
		out = append(out, path)
	}
	return out
}

func cleanLines(lines []string) []string {
	var out []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "<!--") {
			out = append(out, line)
		}
	}
	return out
}

func firstNonEmpty(values []string, fallback string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return fallback
}
