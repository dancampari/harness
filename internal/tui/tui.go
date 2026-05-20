// Package tui renders the live "Autonomous Development Pipeline" view.
//
// The screen has four regions:
//  1. Sprints table: fixed pipeline columns with active/pending/done states
//  2. Verdict panel: latest QA report opened automatically after QA finishes
//  3. Activity log: latest QA summary, findings, or project progress lines
//  4. Status bar: current state, sprint count, average score, watch status
package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dancampari/harness/internal/agreement"
	"github.com/dancampari/harness/internal/evaluator"
	"github.com/dancampari/harness/internal/planner"
	"golang.org/x/term"
)

const (
	animationInterval = 150 * time.Millisecond
	refreshInterval   = 750 * time.Millisecond

	colorPanel   = lipgloss.Color("236")
	colorBorder  = lipgloss.Color("66")
	colorAccent  = lipgloss.Color("81")
	colorText    = lipgloss.Color("252")
	colorMuted   = lipgloss.Color("244")
	colorGood    = lipgloss.Color("114")
	colorWarn    = lipgloss.Color("215")
	colorBad     = lipgloss.Color("203")
	colorHeader  = lipgloss.Color("230")
	colorSurface = lipgloss.Color("238")
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorHeader).
			Background(colorSurface).
			Padding(0, 2)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			PaddingLeft(1)

	panelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Background(colorPanel).
			Padding(0, 1).
			MarginBottom(1)

	panelTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorHeader)

	headerCellStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorMuted)

	mutedStyle = lipgloss.NewStyle().Foreground(colorMuted)
	goodStyle  = lipgloss.NewStyle().Foreground(colorGood).Bold(true)
	warnStyle  = lipgloss.NewStyle().Foreground(colorWarn).Bold(true)
	badStyle   = lipgloss.NewStyle().Foreground(colorBad).Bold(true)

	statusBarStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Foreground(colorMuted).
			Background(colorPanel).
			Padding(0, 1)
)

// Run launches the TUI. If resume is true, it loads existing state.
func Run(harnessDir string, resume bool, version string) error {
	m := newModel(harnessDir, resume, version)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

type model struct {
	root         string
	project      string
	version      string
	sprints      []sprintRow
	activity     []string
	latestReport *evaluator.EvaluationResult
	startTime    time.Time
	lastSeen     time.Time
	lastEvent    string
	lastRefresh  time.Time
	signature    string
	frame        int
	width        int
	height       int
	commandMode  bool
	commandInput string
	commandBusy  bool
	commandRun   string
	commandLog   []string
	activityTop  int
	sprintBottom int
}

type sprintRow struct {
	Number   int
	Goal     string
	Contract string
	Build    string
	QA       string
	Score    string
	Time     string
	Findings int
	Scored   bool
	Stale    bool
}

type tickMsg time.Time
type commandDoneMsg struct {
	input  string
	output string
	err    string
}

func newModel(root string, resume bool, version string) *model {
	m := &model{
		root:      root,
		project:   projectName(root),
		version:   displayVersion(version),
		startTime: time.Now(),
		lastSeen:  time.Now(),
		lastEvent: "watching .harness",
		width:     96,
	}
	if resume {
		// State lives on disk in .harness/, so resume just refreshes.
	}
	m.refresh()
	return m
}

func (m *model) Init() tea.Cmd {
	return tick()
}

func tick() tea.Cmd {
	return tea.Tick(animationInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clampScrolls()
		return m, tea.ClearScreen
	case tea.KeyMsg:
		if m.commandMode {
			return m.updateCommandInput(msg)
		}
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "r":
			m.refresh()
		case ":", "c":
			m.commandMode = true
			m.commandInput = ""
		case "up", "k":
			m.scrollActivity(-1)
		case "down", "j":
			m.scrollActivity(1)
		case "pgup":
			m.scrollSprints(1)
		case "pgdown":
			m.scrollSprints(-1)
		case "home":
			m.activityTop = 0
			m.sprintBottom = maxInt(0, len(m.sprints)-1)
		case "end":
			m.activityTop = 0
			m.sprintBottom = 0
		}
	case tickMsg:
		m.frame++
		resized := m.syncTerminalSize()
		now := time.Time(msg)
		if m.lastRefresh.IsZero() || now.Sub(m.lastRefresh) >= refreshInterval {
			m.refresh()
		}
		if resized {
			return m, tea.Batch(tea.ClearScreen, tick())
		}
		return m, tick()
	case commandDoneMsg:
		m.commandBusy = false
		if msg.err != "" {
			m.addCommandLog(fmt.Sprintf("command failed: %s", msg.input))
			for _, line := range firstNonEmptyLines(msg.output+"\n"+msg.err, 2) {
				m.addCommandLog(line)
			}
		} else {
			m.addCommandLog(fmt.Sprintf("command done: %s", msg.input))
			for _, line := range firstNonEmptyLines(msg.output, 2) {
				m.addCommandLog(line)
			}
		}
		m.refresh()
	}
	return m, nil
}

func (m *model) updateCommandInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.commandMode = false
		m.commandInput = ""
		return m, nil
	case "enter":
		input := strings.TrimSpace(m.commandInput)
		m.commandMode = false
		m.commandInput = ""
		return m.executeCommand(input)
	case "backspace", "ctrl+h":
		if len(m.commandInput) > 0 {
			runes := []rune(m.commandInput)
			m.commandInput = string(runes[:len(runes)-1])
		}
		return m, nil
	}
	if len(msg.Runes) > 0 {
		m.commandInput += string(msg.Runes)
	}
	return m, nil
}

func (m *model) executeCommand(input string) (tea.Model, tea.Cmd) {
	if input == "" {
		return m, nil
	}
	switch strings.ToLower(input) {
	case "q", "quit", "exit":
		return m, tea.Quit
	case "r", "refresh":
		m.refresh()
		return m, nil
	}
	if m.commandBusy {
		m.addCommandLog("command ignored: another command is still running")
		return m, nil
	}
	m.commandBusy = true
	m.commandRun = input
	m.addCommandLog("command running: " + input)
	return m, runCommand(m.root, input)
}

func (m *model) refresh() {
	m.syncTerminalSize()
	m.refreshWatchState()
	m.latestReport = m.loadLatestReport()
	m.sprints = m.loadSprints()
	m.activity = m.loadActivity()
	m.clampScrolls()
	m.lastRefresh = time.Now()
}

func (m *model) syncTerminalSize() bool {
	width, height, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width <= 0 || height <= 0 {
		return false
	}
	if width == m.width && height == m.height {
		return false
	}
	m.width, m.height = width, height
	m.clampScrolls()
	return true
}

func (m *model) clampScrolls() {
	if m.activityTop < 0 {
		m.activityTop = 0
	}
	maxActivity := maxInt(0, len(m.combinedActivity())-1)
	if m.activityTop > maxActivity {
		m.activityTop = maxActivity
	}
	if m.sprintBottom < 0 {
		m.sprintBottom = 0
	}
	maxSprint := maxInt(0, len(m.sprints)-1)
	if m.sprintBottom > maxSprint {
		m.sprintBottom = maxSprint
	}
}

func (m *model) scrollActivity(delta int) {
	limit := m.activityVisibleLimit()
	maxTop := maxInt(0, len(m.combinedActivity())-limit)
	m.activityTop += delta
	if m.activityTop < 0 {
		m.activityTop = 0
	}
	if m.activityTop > maxTop {
		m.activityTop = maxTop
	}
}

func (m *model) scrollSprints(delta int) {
	limit := m.sprintVisibleLimit()
	maxBottom := maxInt(0, len(m.sprints)-limit)
	m.sprintBottom += delta
	if m.sprintBottom < 0 {
		m.sprintBottom = 0
	}
	if m.sprintBottom > maxBottom {
		m.sprintBottom = maxBottom
	}
}

func (m *model) refreshWatchState() {
	signature, event := harnessSignature(m.root)
	if m.signature == "" {
		m.signature = signature
		if event != "" {
			m.lastEvent = event
		}
		return
	}
	if signature != m.signature {
		m.signature = signature
		m.lastSeen = time.Now()
		if event != "" {
			m.lastEvent = event
		}
	}
}

func (m *model) loadSprints() []sprintRow {
	dir := filepath.Join(m.root, "contracts")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	progress := readOptionalFile(filepath.Join(m.root, "progress.md"))
	var rows []sprintRow
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "sprint-") || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		var n int
		fmt.Sscanf(e.Name(), "sprint-%d.md", &n)
		row := sprintRow{
			Number:   n,
			Contract: "draft",
			Build:    "pending",
			QA:       "pending",
			Score:    "-",
			Time:     "-",
		}
		var ag agreement.Status
		if c, err := planner.Parse(filepath.Join(dir, e.Name())); err == nil {
			row.Goal = c.Title
			if st, err := agreement.NewManager(m.root).Status(n); err == nil {
				ag = st
				row.Contract = strings.ToUpper(st.State)
			}
		}

		hasLegacyResult := false
		hasCurrentReport := false
		ev := filepath.Join(m.root, "evaluations", fmt.Sprintf("sprint-%03d.md", n))
		if b, err := os.ReadFile(ev); err == nil {
			hasLegacyResult = true
			row.Build = "DONE"
			s := string(b)
			switch {
			case strings.Contains(s, "Verdict: PASS"):
				row.QA = "PASS"
			case strings.Contains(s, "Verdict: FAIL"):
				row.QA = "FAIL"
			}
		}

		rp := filepath.Join(m.root, "reports", fmt.Sprintf("sprint-%03d.json", n))
		if b, err := os.ReadFile(rp); err == nil {
			var er evaluator.EvaluationResult
			if json.Unmarshal(b, &er) == nil {
				hasLegacyResult = true
				row.Score = fmt.Sprintf("%d", er.TotalScore)
				row.QA = er.Verdict
				row.Time = compactSeconds(er.DurationSeconds)
				row.Findings = countFindings(er)
				if ag.ReportIsCurrent(er.Timestamp) {
					hasCurrentReport = true
				} else {
					row.Stale = true
				}
			}
		}
		row.Scored = strings.Contains(progress, fmt.Sprintf("Sprint %03d", n))
		if qaDone(row) && !hasCurrentReport {
			row.Stale = true
		}
		if !contractDone(row) && (hasLegacyResult || row.Scored) {
			row.Stale = true
		}
		if row.Stale {
			row.Build = "STALE"
			row.QA = "STALE"
			row.Score = "-"
			row.Time = "-"
			row.Findings = 0
			row.Scored = false
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Number < rows[j].Number })
	return rows
}

func (m *model) loadActivity() []string {
	if lines := m.activityFromLatestReport(); len(lines) > 0 {
		return lines
	}
	b, err := os.ReadFile(filepath.Join(m.root, "progress.md"))
	if err != nil {
		return []string{"Waiting for agent activity..."}
	}
	lines := cleanLines(strings.Split(string(b), "\n"))
	if len(lines) == 0 {
		return []string{"Waiting for agent activity..."}
	}
	if len(lines) > 7 {
		lines = lines[len(lines)-7:]
	}
	return lines
}

func (m *model) loadLatestReport() *evaluator.EvaluationResult {
	b, err := os.ReadFile(filepath.Join(m.root, "reports", "latest.json"))
	if err != nil {
		return nil
	}
	var er evaluator.EvaluationResult
	if json.Unmarshal(b, &er) != nil {
		return nil
	}
	return &er
}

func (m *model) reportAgreementState(sprintNumber int, reportTime time.Time) (string, bool) {
	st, err := agreement.NewManager(m.root).Status(sprintNumber)
	if err != nil {
		return "UNKNOWN", false
	}
	state := strings.ToUpper(st.State)
	if st.ReportIsCurrent(reportTime) {
		return state, true
	}
	if state == "AGREED" {
		return "STALE", false
	}
	return state, false
}

func (m *model) activityFromLatestReport() []string {
	if m.latestReport == nil {
		return nil
	}
	er := *m.latestReport
	if state, current := m.reportAgreementState(er.SprintNumber, er.Timestamp); !current {
		return []string{
			fmt.Sprintf("Contract gate pending for sprint %03d: %s", er.SprintNumber, state),
			fmt.Sprintf("Previous QA %s score %d/100 is stale and ignored", er.Verdict, er.TotalScore),
			"Next: contract propose, planner approve, tester approve, then rerun QA",
		}
	}
	lines := []string{
		fmt.Sprintf("QA %s  sprint %03d  score %d/100  runtime %s",
			er.Verdict, er.SprintNumber, er.TotalScore, compactSeconds(er.DurationSeconds)),
	}

	dimNames := make([]string, 0, len(er.Dimensions))
	for name := range er.Dimensions {
		dimNames = append(dimNames, name)
	}
	sort.Strings(dimNames)
	for _, name := range dimNames {
		d := er.Dimensions[name]
		state := "pass"
		if !d.Passed {
			state = "fail"
		}
		lines = append(lines, fmt.Sprintf("%s %d/%d %s  sensors: %s",
			name, d.Score, d.Threshold, state, strings.Join(d.SensorsUsed, ",")))
		if len(lines) >= 6 {
			break
		}
	}

	for _, name := range dimNames {
		for _, f := range er.Dimensions[name].Findings {
			file := f.File
			if file == "" {
				file = "-"
			}
			lines = append(lines, fmt.Sprintf("%s %s %s:%d  %s",
				f.Severity, f.Rule, file, f.Line, truncate(f.Message, 58)))
			if len(lines) >= 9 {
				return lines
			}
		}
	}
	return lines
}

func (m *model) View() string {
	width := m.contentWidth()
	header := m.renderHeader(width)
	sprints := panelStyle.Width(width).Render(m.renderSprints(width - 4))
	verdict := ""
	if m.latestReport != nil && m.heightAllowsVerdict() {
		verdict = panelStyle.Width(width).Render(m.renderVerdict(width - 4))
	}
	activity := ""
	if m.heightAllowsActivity() {
		activity = panelStyle.Width(width).Render(m.renderActivity(width - 4))
	}
	status := statusBarStyle.Width(width).Render(m.renderStatus())
	var view string
	if verdict != "" {
		if activity != "" {
			view = lipgloss.JoinVertical(lipgloss.Left, header, sprints, verdict, activity, status)
		} else {
			view = lipgloss.JoinVertical(lipgloss.Left, header, sprints, verdict, status)
		}
		return m.fitToScreen(view)
	}
	if activity != "" {
		view = lipgloss.JoinVertical(lipgloss.Left, header, sprints, activity, status)
	} else {
		view = lipgloss.JoinVertical(lipgloss.Left, header, sprints, status)
	}
	return m.fitToScreen(view)
}

func (m *model) heightAllowsVerdict() bool {
	return m.height <= 0 || m.height >= 18
}

func (m *model) heightAllowsActivity() bool {
	return m.height <= 0 || m.height >= 12
}

func (m *model) fitToScreen(view string) string {
	if m.width <= 0 || m.height <= 0 {
		return view
	}
	width := maxInt(1, m.width)
	height := maxInt(1, m.height)
	lines := strings.Split(view, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for i, line := range lines {
		visible := lipgloss.Width(line)
		if visible < width {
			lines[i] = line + strings.Repeat(" ", width-visible)
		}
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	return strings.Join(lines, "\n")
}

func (m *model) renderHeader(width int) string {
	left := titleStyle.Render("harness")
	right := subtitleStyle.Render("Autonomous Development Pipeline   " + m.version)
	line := lipgloss.JoinHorizontal(lipgloss.Center, left, right)
	return lipgloss.NewStyle().
		Width(width).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccent).
		Render(line)
}

func (m *model) renderSprints(width int) string {
	var sb strings.Builder
	title := "Sprints"
	limit := m.sprintVisibleLimit()
	total := len(m.sprints)
	start, end := sprintRange(total, limit, m.sprintBottom)
	if total > limit {
		title = fmt.Sprintf("Sprints %d-%d/%d", start+1, end, total)
	}
	sb.WriteString(panelTitleStyle.Render(title))
	sb.WriteString("\n")
	sb.WriteString(renderSprintHeader(width))
	sb.WriteString("\n")
	if len(m.sprints) == 0 {
		sb.WriteString(mutedStyle.Render("No sprints yet. Run: harness sprint new \"first goal\""))
		return sb.String()
	}
	for _, r := range m.sprints[start:end] {
		sb.WriteString(renderSprintRow(r, width, m.frame))
		sb.WriteString("\n")
	}
	if total > limit {
		sb.WriteString(mutedStyle.Render("PgUp/PgDn to scroll sprints"))
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

func (m *model) sprintVisibleLimit() int {
	if m.height <= 0 {
		return 8
	}
	return maxInt(2, minInt(8, (m.height-14)/2))
}

func sprintRange(total, limit, bottomOffset int) (int, int) {
	if total <= 0 {
		return 0, 0
	}
	limit = maxInt(1, minInt(limit, total))
	maxBottom := maxInt(0, total-limit)
	if bottomOffset > maxBottom {
		bottomOffset = maxBottom
	}
	if bottomOffset < 0 {
		bottomOffset = 0
	}
	end := total - bottomOffset
	start := maxInt(0, end-limit)
	return start, end
}

func renderSprintHeader(width int) string {
	return headerCellStyle.Render(sprintColumns(width, "#", "Goal", "Contract", "Build", "QA", "Score", "Time", "Find"))
}

func renderSprintRow(r sprintRow, width, frame int) string {
	line := sprintColumns(width,
		fmt.Sprintf("%03d", r.Number),
		defaultString(r.Goal, "-"),
		contractCell(r, frame),
		buildCell(r, frame),
		qaCell(r, frame),
		scoreCell(r, frame),
		r.Time,
		fmt.Sprintf("%d", r.Findings),
	)
	if r.Stale {
		return warnStyle.Render(line)
	}
	switch strings.ToUpper(r.QA) {
	case "PASS":
		if r.Scored {
			return goodStyle.Render(line)
		}
		return warnStyle.Render(line)
	case "FAIL":
		return badStyle.Render(line)
	default:
		if isSprintActive(r) {
			return warnStyle.Render(line)
		}
		return mutedStyle.Render(line)
	}
}

func sprintColumns(width int, number, goal, contract, build, qa, score, elapsed, findings string) string {
	goalWidth := sprintGoalWidth(width)
	columns := []struct {
		value string
		width int
	}{
		{number, 4},
		{goal, goalWidth},
		{contract, 11},
		{build, 9},
		{qa, 9},
		{score, 7},
		{elapsed, 8},
		{findings, 8},
	}
	var sb strings.Builder
	for i, col := range columns {
		if i > 0 {
			sb.WriteString("  ")
		}
		sb.WriteString(padCell(col.value, col.width))
	}
	return truncate(sb.String(), width)
}

func sprintGoalWidth(width int) int {
	goal := width - 63
	if goal < 10 {
		return 10
	}
	if goal > 36 {
		return 36
	}
	return goal
}

func contractCell(r sprintRow, frame int) string {
	if contractDone(r) {
		return "✓ AGREED"
	}
	switch strings.ToUpper(r.Contract) {
	case "DRAFT":
		return spinner(frame) + " DRAFT"
	case "PROPOSED":
		return spinner(frame) + " REVIEW"
	case "CHANGED":
		return "× CHANGED"
	case "REJECTED":
		return "× REJECT"
	case "MISSING":
		return "• missing"
	}
	return "• pending"
}

func buildCell(r sprintRow, frame int) string {
	if strings.EqualFold(r.Build, "STALE") {
		return "× STALE"
	}
	if buildDone(r) {
		return "✓ DONE"
	}
	if contractDone(r) {
		return spinner(frame) + " BUILD"
	}
	return "• pending"
}

func qaCell(r sprintRow, frame int) string {
	switch strings.ToUpper(r.QA) {
	case "PASS":
		return "✓ PASS"
	case "FAIL":
		return "× FAIL"
	case "STALE":
		return "× STALE"
	}
	if buildDone(r) {
		return spinner(frame) + " QA"
	}
	return "• pending"
}

func scoreCell(r sprintRow, frame int) string {
	if r.Stale {
		return "stale"
	}
	if r.Scored {
		if strings.EqualFold(r.QA, "FAIL") {
			return "× " + defaultString(r.Score, "-")
		}
		return "✓ " + defaultString(r.Score, "-")
	}
	if r.Score != "" && r.Score != "-" && qaDone(r) {
		if strings.EqualFold(r.QA, "FAIL") {
			return "× " + r.Score
		}
		return "• " + r.Score
	}
	if qaDone(r) {
		return "ready"
	}
	return "• -"
}

func spinner(frame int) string {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	return frames[frame%len(frames)]
}

func contractDone(r sprintRow) bool {
	return strings.EqualFold(r.Contract, "AGREED")
}

func buildDone(r sprintRow) bool {
	return strings.EqualFold(r.Build, "DONE")
}

func qaDone(r sprintRow) bool {
	return strings.EqualFold(r.QA, "PASS") || strings.EqualFold(r.QA, "FAIL")
}

func isSprintActive(r sprintRow) bool {
	return !contractDone(r) || !buildDone(r) || !qaDone(r) || !r.Scored
}

func padCell(value string, width int) string {
	value = truncate(value, width)
	if len([]rune(value)) >= width {
		return value
	}
	return value + strings.Repeat(" ", width-len([]rune(value)))
}

func (m *model) renderVerdict(width int) string {
	er := m.latestReport
	if er == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(panelTitleStyle.Render("Verdict"))
	sb.WriteString("\n")
	if state, current := m.reportAgreementState(er.SprintNumber, er.Timestamp); !current {
		summary := fmt.Sprintf("sprint %03d  BLOCKED  contract %s", er.SprintNumber, state)
		sb.WriteString(warnStyle.Render(truncate(summary, width-2)))
		sb.WriteString("\n")
		sb.WriteString(mutedStyle.Render(truncate(
			fmt.Sprintf("previous QA %s %d/100 is stale; rerun QA after planner/tester agreement",
				er.Verdict, er.TotalScore), width-2)))
		return sb.String()
	}
	summary := fmt.Sprintf("sprint %03d  %s  score %d/100  runtime %s",
		er.SprintNumber, er.Verdict, er.TotalScore, compactSeconds(er.DurationSeconds))
	if er.Verdict == "PASS" {
		sb.WriteString(goodStyle.Render(truncate(summary, width-2)))
	} else {
		sb.WriteString(badStyle.Render(truncate(summary, width-2)))
	}
	sb.WriteString("\n")
	sb.WriteString(headerCellStyle.Render(verdictColumns(width, "Dimension", "Score", "Threshold", "Status", "Find", "Sensors")))
	sb.WriteString("\n")

	names := visibleVerdictDimensions(er.Dimensions, 6)
	limit := minInt(6, len(names))
	for _, name := range names[:limit] {
		d := er.Dimensions[name]
		status := "✓ pass"
		lineStyle := goodStyle
		if !d.Passed {
			status = "× fail"
			lineStyle = badStyle
		}
		sensors := strings.Join(d.SensorsUsed, ",")
		if sensors == "" {
			sensors = "-"
		}
		sb.WriteString(lineStyle.Render(verdictColumns(width,
			name,
			fmt.Sprintf("%d", d.Score),
			fmt.Sprintf("%d", d.Threshold),
			status,
			fmt.Sprintf("%d", len(d.Findings)),
			sensors,
		)))
		sb.WriteString("\n")
	}
	if len(names) > limit {
		sb.WriteString(mutedStyle.Render(fmt.Sprintf("... %d more dimensions", len(names)-limit)))
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

func verdictColumns(width int, dimension, score, threshold, status, findings, sensors string) string {
	sensorWidth := maxInt(10, width-58)
	columns := []struct {
		value string
		width int
	}{
		{dimension, 15},
		{score, 7},
		{threshold, 10},
		{status, 9},
		{findings, 5},
		{sensors, sensorWidth},
	}
	var sb strings.Builder
	for i, col := range columns {
		if i > 0 {
			sb.WriteString("  ")
		}
		sb.WriteString(padCell(col.value, col.width))
	}
	return truncate(sb.String(), width)
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
	preferredCount := len(names)
	for name := range dims {
		if !seen[name] {
			names = append(names, name)
		}
	}
	sort.Strings(names[preferredCount:])
	return names
}

func visibleVerdictDimensions(dims map[string]evaluator.DimensionScore, limit int) []string {
	names := orderedDimensionNames(dims)
	if len(names) <= limit {
		return names
	}
	selected := append([]string{}, names[:limit]...)
	inSelected := map[string]bool{}
	for _, name := range selected {
		inSelected[name] = true
	}
	for _, name := range names {
		d := dims[name]
		if d.Passed || inSelected[name] {
			continue
		}
		replace := lastPassedDimensionIndex(selected, dims)
		if replace < 0 {
			continue
		}
		delete(inSelected, selected[replace])
		selected[replace] = name
		inSelected[name] = true
	}
	return selected
}

func lastPassedDimensionIndex(names []string, dims map[string]evaluator.DimensionScore) int {
	for i := len(names) - 1; i >= 0; i-- {
		if dims[names[i]].Passed {
			return i
		}
	}
	return -1
}

func (m *model) renderActivity(width int) string {
	var sb strings.Builder
	limit := m.activityVisibleLimit()
	lines := m.combinedActivity()
	total := len(lines)
	start := minInt(m.activityTop, maxInt(0, total-limit))
	end := minInt(total, start+limit)
	title := "Activity"
	if total > limit {
		title = fmt.Sprintf("Activity %d-%d/%d", start+1, end, total)
	}
	sb.WriteString(panelTitleStyle.Render(title))
	sb.WriteString("\n")
	sb.WriteString(renderActivityLine(truncate(fmt.Sprintf("watching .harness  last event: %s  updated %s",
		m.lastEvent, compactDuration(time.Since(m.lastSeen))), width-2)))
	sb.WriteString("\n")
	for _, line := range lines[start:end] {
		rendered := renderActivityLine(truncate(line, width-2))
		sb.WriteString(rendered)
		sb.WriteString("\n")
	}
	if total > limit {
		sb.WriteString(mutedStyle.Render("Up/Down to scroll activity"))
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

func (m *model) activityVisibleLimit() int {
	limit := 8
	if m.height > 0 {
		limit = maxInt(3, minInt(12, m.height-17))
	}
	return limit
}

func (m *model) combinedActivity() []string {
	lines := append([]string{}, m.commandLog...)
	if m.commandBusy {
		lines = append(lines, "command running: "+m.commandRun)
	}
	lines = append(lines, m.activity...)
	if len(lines) == 0 {
		return []string{"Waiting for agent activity..."}
	}
	return lines
}

func renderActivityLine(line string) string {
	low := strings.ToLower(line)
	switch {
	case strings.Contains(low, " fail") || strings.Contains(low, "fail ") ||
		strings.Contains(low, "critical") || strings.Contains(low, " high "):
		return badStyle.Render(line)
	case strings.Contains(low, "miss") || strings.Contains(low, "missing") ||
		strings.Contains(low, "stale") || strings.Contains(low, "blocked") ||
		strings.Contains(low, "draft") || strings.Contains(low, "pending"):
		return warnStyle.Render(line)
	case strings.Contains(low, "pass") || strings.Contains(low, "satisfied"):
		return goodStyle.Render(line)
	default:
		return mutedStyle.Render(line)
	}
}

func (m *model) renderStatus() string {
	state := "idle"
	if latest := latestSprint(m.sprints); latest != nil && latest.Stale {
		state = "blocked"
	}
	if latest := latestSprint(m.sprints); latest != nil && latest.QA == "FAIL" {
		state = "attention"
	}
	if latest := latestSprint(m.sprints); latest != nil && latest.QA == "PASS" && !latest.Stale {
		state = "ready"
	}
	active := 0
	if latest := latestSprint(m.sprints); latest != nil {
		active = latest.Number
	}
	line := fmt.Sprintf("%s   project %s   sprint %d/%d   avg score %s   watch %s: %s   elapsed %s",
		state,
		m.project,
		active,
		maxInt(active, len(m.sprints)),
		averageScore(m.sprints),
		compactDuration(time.Since(m.lastSeen)),
		m.lastEvent,
		time.Since(m.startTime).Round(time.Second),
	)
	if m.commandMode {
		return line + "\n" + "> " + m.commandInput
	}
	if m.commandBusy {
		return line + "\n" + "running: " + m.commandRun
	}
	return line + "\n" + "[: command] qa | repair | accept | score | status | doctor | !shell    [r refresh | q quit | arrows scroll]"
}

func runCommand(root, input string) tea.Cmd {
	return func() tea.Msg {
		output, err := runCommandOutput(root, input)
		done := commandDoneMsg{input: input, output: output}
		if err != nil {
			done.err = err.Error()
		}
		return done
	}
}

func runCommandOutput(root, input string) (string, error) {
	projectRoot := projectRootForHarness(root)
	if strings.HasPrefix(input, "!") {
		cmd := shellCommand(strings.TrimSpace(strings.TrimPrefix(input, "!")))
		cmd.Dir = projectRoot
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	args, err := harnessCommandArgs(input)
	if err != nil {
		return "", err
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	cmd := exec.Command(exe, args...)
	cmd.Dir = projectRoot
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func harnessCommandArgs(input string) ([]string, error) {
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return nil, fmt.Errorf("empty command")
	}
	if fields[0] == "harness" {
		return fields[1:], nil
	}
	if fields[0] == "sprint" || fields[0] == "contract" || fields[0] == "doctor" || fields[0] == "skills" {
		return fields, nil
	}
	switch strings.ToLower(fields[0]) {
	case "qa":
		return append([]string{"sprint", "qa"}, fields[1:]...), nil
	case "accept", "accept-screenshots":
		return append([]string{"sprint", "qa", "--accept-screenshots"}, fields[1:]...), nil
	case "score":
		return append([]string{"sprint", "score"}, fields[1:]...), nil
	case "repair":
		return append([]string{"sprint", "repair"}, fields[1:]...), nil
	case "status":
		return append([]string{"sprint", "status"}, fields[1:]...), nil
	case "doctor":
		return []string{"doctor"}, nil
	case "propose":
		return []string{"contract", "propose"}, nil
	case "approve":
		if len(fields) < 2 {
			return nil, fmt.Errorf("usage: approve planner|tester")
		}
		return []string{"contract", "approve", "--role", fields[1]}, nil
	case "reject":
		if len(fields) < 3 {
			return nil, fmt.Errorf("usage: reject planner|tester <reason>")
		}
		return []string{"contract", "reject", "--role", fields[1], "--reason", strings.Join(fields[2:], " ")}, nil
	case "new":
		goal := strings.TrimSpace(strings.TrimPrefix(input, fields[0]))
		if goal == "" {
			return nil, fmt.Errorf("usage: new <goal>")
		}
		return []string{"sprint", "new", goal}, nil
	default:
		return nil, fmt.Errorf("unknown command %q; use qa, accept, score, status, doctor, propose, approve, reject, new, or !shell", fields[0])
	}
}

func shellCommand(input string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.Command("powershell", "-NoProfile", "-Command", input)
	}
	return exec.Command("sh", "-lc", input)
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

func (m *model) addCommandLog(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	m.commandLog = append(m.commandLog, line)
	if len(m.commandLog) > 5 {
		m.commandLog = m.commandLog[len(m.commandLog)-5:]
	}
}

func firstNonEmptyLines(output string, limit int) []string {
	var lines []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lines = append(lines, line)
		if len(lines) >= limit {
			return lines
		}
	}
	return lines
}

type watchedArtifact struct {
	path  string
	label string
	mod   time.Time
	size  int64
}

func harnessSignature(root string) (string, string) {
	artifacts := watchedArtifacts(root)
	if len(artifacts) == 0 {
		return "empty", "watching .harness"
	}
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].path < artifacts[j].path })
	var newest watchedArtifact
	var sb strings.Builder
	for i, a := range artifacts {
		if i == 0 || a.mod.After(newest.mod) {
			newest = a
		}
		sb.WriteString(a.path)
		sb.WriteByte('|')
		sb.WriteString(fmt.Sprintf("%d|%d\n", a.mod.UnixNano(), a.size))
	}
	return sb.String(), newest.label
}

func watchedArtifacts(root string) []watchedArtifact {
	var artifacts []watchedArtifact
	addFile := func(rel, label string) {
		info, err := os.Stat(filepath.Join(root, rel))
		if err != nil || info.IsDir() {
			return
		}
		artifacts = append(artifacts, watchedArtifact{
			path:  filepath.ToSlash(rel),
			label: label,
			mod:   info.ModTime(),
			size:  info.Size(),
		})
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

	addFile("config.yaml", "config updated")
	addFile("spec.md", "spec updated")
	addFile("progress.md", "progress scored")
	addDir("contracts", "contract updated")
	addDir("evaluations", "evaluation updated")
	addDir("reports", "qa report updated")
	addDir("repairs", "repair brief updated")
	return artifacts
}

func (m *model) contentWidth() int {
	if m.width <= 0 {
		return 96
	}
	return maxInt(72, minInt(m.width-2, 118))
}

func projectName(root string) string {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "project"
	}
	if filepath.Base(abs) == ".harness" {
		return filepath.Base(filepath.Dir(abs))
	}
	return filepath.Base(abs)
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func displayVersion(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return "dev"
	}
	if version == "dev" || strings.HasPrefix(version, "v") {
		return version
	}
	return "v" + version
}

func readOptionalFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

func stageText(value string) string {
	switch strings.ToUpper(value) {
	case "AGREED":
		return "AGREED"
	case "DONE":
		return "DONE"
	case "PASS":
		return "PASS"
	case "FAIL":
		return "FAIL"
	case "STALE":
		return "stale"
	case "DRAFT":
		return "draft"
	default:
		return value
	}
}

func statusText(value string) string {
	switch strings.ToUpper(value) {
	case "AGREED":
		return "✓ AGREED"
	case "DONE":
		return "✓ DONE"
	case "PASS":
		return "✓ PASS"
	case "FAIL":
		return "× FAIL"
	case "STALE":
		return "× STALE"
	case "DRAFT":
		return "• draft"
	default:
		return value
	}
}

func latestSprint(rows []sprintRow) *sprintRow {
	if len(rows) == 0 {
		return nil
	}
	return &rows[len(rows)-1]
}

func averageScore(rows []sprintRow) string {
	total, count := 0, 0
	for _, r := range rows {
		var n int
		if _, err := fmt.Sscanf(r.Score, "%d", &n); err == nil {
			total += n
			count++
		}
	}
	if count == 0 {
		return "-"
	}
	return fmt.Sprintf("%d", total/count)
}

func countFindings(er evaluator.EvaluationResult) int {
	total := 0
	for _, d := range er.Dimensions {
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

func compactDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	d = d.Round(time.Second)
	if d < time.Second {
		return "just now"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	}
	return d.String() + " ago"
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

func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
