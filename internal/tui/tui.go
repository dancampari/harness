// Package tui renders the live "Autonomous Development Pipeline" view.
//
// The screen has three regions:
//  1. Sprints table: Goal / Contract / Build / QA / Score columns
//  2. Activity log: the latest project progress lines
//  3. Status bar: active sprint, average score, elapsed time
//
// The model re-reads .harness/reports/ on a ticker, so the TUI stays fresh
// even when another process advances the sprint.
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

// Run launches the TUI. If resume is true, it loads existing state.
func Run(harnessDir string, resume bool) error {
	m := newModel(harnessDir, resume)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

type model struct {
	root      string
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
}

type tickMsg time.Time

func newModel(root string, resume bool) *model {
	m := &model{
		root:      root,
		startTime: time.Now(),
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
			}
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Number < rows[j].Number })
	return rows
}

func (m *model) loadActivity() []string {
	b, err := os.ReadFile(filepath.Join(m.root, "progress.md"))
	if err != nil {
		return nil
	}
	lines := strings.Split(string(b), "\n")
	if len(lines) > 8 {
		lines = lines[len(lines)-8:]
	}
	return lines
}

func (m *model) View() string {
	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("12")).
		Padding(0, 1)
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Padding(0, 1).
		MarginBottom(1)

	title := header.Render("harness - Autonomous Development Pipeline")

	var sb strings.Builder
	sb.WriteString("Sprints\n")
	sb.WriteString(fmt.Sprintf("%-3s  %-40s  %-10s  %-8s  %-8s  %-6s\n",
		"#", "Goal", "Contract", "Build", "QA", "Score"))
	for _, r := range m.sprints {
		goal := r.Goal
		if len(goal) > 38 {
			goal = goal[:35] + "..."
		}
		sb.WriteString(fmt.Sprintf("%-3d  %-40s  %-10s  %-8s  %-8s  %-6s\n",
			r.Number, goal, r.Contract, r.Build, r.QA, r.Score))
	}
	if len(m.sprints) == 0 {
		sb.WriteString("(no sprints yet - run: harness sprint new \"<goal>\")\n")
	}
	sprintsBox := box.Render(sb.String())

	var ab strings.Builder
	ab.WriteString("Activity\n")
	if len(m.activity) == 0 {
		ab.WriteString("Waiting for agent activity...\n")
	} else {
		for _, line := range m.activity {
			ab.WriteString(line + "\n")
		}
	}
	activityBox := box.Render(ab.String())

	avg := averageScore(m.sprints)
	statusLine := fmt.Sprintf("active   sprint %d/%d   avg score %s   elapsed %s   [q quit | r refresh]",
		len(m.sprints), 10, avg, time.Since(m.startTime).Round(time.Second))
	statusBar := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Render(statusLine)

	return title + "\n" + sprintsBox + activityBox + statusBar
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
