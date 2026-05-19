// Package tui renders the live "Autonomous Development Pipeline" view.
//
// The screen has three regions:
//  1. Sprints table: Goal / Contract / Build / QA / Score / Time / Findings
//  2. Activity log: latest QA summary, findings, or project progress lines
//  3. Status bar: current state, sprint count, average score, elapsed time
package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dancampari/harness/internal/evaluator"
	"github.com/dancampari/harness/internal/planner"
)

const (
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
func Run(harnessDir string, resume bool) error {
	m := newModel(harnessDir, resume)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

type model struct {
	root      string
	project   string
	sprints   []sprintRow
	activity  []string
	startTime time.Time
	width     int
	height    int
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
}

type tickMsg time.Time

func newModel(root string, resume bool) *model {
	m := &model{
		root:      root,
		project:   projectName(root),
		startTime: time.Now(),
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
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "r":
			m.refresh()
		}
	case tickMsg:
		m.refresh()
		return m, tick()
	}
	return m, nil
}

func (m *model) refresh() {
	m.sprints = m.loadSprints()
	m.activity = m.loadActivity()
}

func (m *model) loadSprints() []sprintRow {
	dir := filepath.Join(m.root, "contracts")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var rows []sprintRow
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "sprint-") {
			continue
		}
		var n int
		fmt.Sscanf(e.Name(), "sprint-%d.md", &n)
		row := sprintRow{
			Number:   n,
			Contract: "draft",
			Build:    "-",
			QA:       "-",
			Score:    "-",
			Time:     "-",
		}
		if c, err := planner.Parse(filepath.Join(dir, e.Name())); err == nil {
			row.Goal = c.Title
			if errs := c.Validate(); len(errs) == 0 {
				row.Contract = "AGREED"
			}
		}

		ev := filepath.Join(m.root, "evaluations", fmt.Sprintf("sprint-%03d.md", n))
		if b, err := os.ReadFile(ev); err == nil {
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
				row.Score = fmt.Sprintf("%d", er.TotalScore)
				row.QA = er.Verdict
				row.Time = compactSeconds(er.DurationSeconds)
				row.Findings = countFindings(er)
			}
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

func (m *model) activityFromLatestReport() []string {
	b, err := os.ReadFile(filepath.Join(m.root, "reports", "latest.json"))
	if err != nil {
		return nil
	}
	var er evaluator.EvaluationResult
	if json.Unmarshal(b, &er) != nil {
		return nil
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
	activity := panelStyle.Width(width).Render(m.renderActivity(width - 4))
	status := statusBarStyle.Width(width).Render(m.renderStatus())
	return lipgloss.JoinVertical(lipgloss.Left, header, sprints, activity, status)
}

func (m *model) renderHeader(width int) string {
	left := titleStyle.Render("harness")
	right := subtitleStyle.Render("Autonomous Development Pipeline")
	line := lipgloss.JoinHorizontal(lipgloss.Center, left, right)
	return lipgloss.NewStyle().
		Width(width).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccent).
		Render(line)
}

func (m *model) renderSprints(width int) string {
	var sb strings.Builder
	sb.WriteString(panelTitleStyle.Render("Sprints"))
	sb.WriteString("\n")
	sb.WriteString(renderSprintHeader())
	sb.WriteString("\n")
	if len(m.sprints) == 0 {
		sb.WriteString(mutedStyle.Render("No sprints yet. Run: harness sprint new \"first goal\""))
		return sb.String()
	}
	maxRows := maxInt(1, minInt(8, len(m.sprints)))
	start := len(m.sprints) - maxRows
	for _, r := range m.sprints[start:] {
		sb.WriteString(renderSprintRow(r, width))
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

func renderSprintHeader() string {
	return headerCellStyle.Render(fmt.Sprintf("%-4s %-34s %-12s %-9s %-9s %-7s %-7s %-8s",
		"#", "Goal", "Contract", "Build", "QA", "Score", "Time", "Findings"))
}

func renderSprintRow(r sprintRow, width int) string {
	goalWidth := maxInt(18, width-68)
	if goalWidth > 42 {
		goalWidth = 42
	}
	goal := truncate(defaultString(r.Goal, "-"), goalWidth)
	line := fmt.Sprintf("%-4d %-*s %-12s %-9s %-9s %-7s %-7s %-8d",
		r.Number,
		goalWidth,
		goal,
		statusText(r.Contract),
		statusText(r.Build),
		statusText(r.QA),
		r.Score,
		r.Time,
		r.Findings,
	)
	switch strings.ToUpper(r.QA) {
	case "PASS":
		return goodStyle.Render(line)
	case "FAIL":
		return badStyle.Render(line)
	default:
		return mutedStyle.Render(line)
	}
}

func (m *model) renderActivity(width int) string {
	var sb strings.Builder
	sb.WriteString(panelTitleStyle.Render("Activity"))
	sb.WriteString("\n")
	limit := 8
	if m.height > 0 {
		limit = maxInt(4, minInt(12, m.height-16))
	}
	lines := m.activity
	if len(lines) > limit {
		lines = lines[:limit]
	}
	for _, line := range lines {
		rendered := renderActivityLine(truncate(line, width-2))
		sb.WriteString(rendered)
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

func renderActivityLine(line string) string {
	low := strings.ToLower(line)
	switch {
	case strings.Contains(low, " fail") || strings.Contains(low, "fail ") ||
		strings.Contains(low, "critical") || strings.Contains(low, " high "):
		return badStyle.Render(line)
	case strings.Contains(low, "pass") || strings.Contains(low, "satisfied"):
		return goodStyle.Render(line)
	case strings.Contains(low, "miss") || strings.Contains(low, "missing"):
		return warnStyle.Render(line)
	default:
		return mutedStyle.Render(line)
	}
}

func (m *model) renderStatus() string {
	state := "idle"
	if latest := latestSprint(m.sprints); latest != nil && latest.QA == "FAIL" {
		state = "attention"
	}
	if latest := latestSprint(m.sprints); latest != nil && latest.QA == "PASS" {
		state = "ready"
	}
	active := 0
	if latest := latestSprint(m.sprints); latest != nil {
		active = latest.Number
	}
	return fmt.Sprintf("%s   project %s   sprint %d/%d   avg score %s   elapsed %s   [q quit | r refresh]",
		state,
		m.project,
		active,
		maxInt(active, len(m.sprints)),
		averageScore(m.sprints),
		time.Since(m.startTime).Round(time.Second),
	)
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
