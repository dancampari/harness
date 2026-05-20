package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"
	"os"
)

type viewID int

const (
	viewOverview viewID = iota
	viewRuns
	viewReport
	viewLogs
	viewSkills
	viewDoctor
)

var viewLabels = []string{"Overview", "Runs", "Report", "Logs", "Skills", "Doctor"}

type model struct {
	root        string
	projectRoot string
	version     string
	startTime   time.Time

	data        DashboardData
	activeView  viewID
	runCursor   int
	scroll      map[viewID]int
	helpVisible bool
	detailOpen  bool

	width       int
	height      int
	frame       int
	lastRefresh time.Time

	commandMode  bool
	commandInput string
	commandBusy  bool
	commandRun   string
	commandLog   []string
	lastNotice   string
}

func newModel(root string, resume bool, version string) *model {
	m := &model{
		root:        root,
		projectRoot: projectRootForHarness(root),
		version:     displayVersion(version),
		startTime:   time.Now(),
		width:       118,
		height:      32,
		scroll:      map[viewID]int{},
	}
	if resume {
		// State is file-backed; refresh below is enough.
	}
	m.syncTerminalSize()
	m.refresh()
	return m
}

func (m *model) Init() tea.Cmd {
	return tick()
}

func (m *model) refresh() {
	m.data = loadDashboardData(m.root)
	m.clampNavigation()
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
	m.clampNavigation()
	return true
}

func (m *model) clampNavigation() {
	if m.activeView < viewOverview || int(m.activeView) >= len(viewLabels) {
		m.activeView = viewOverview
	}
	if len(m.data.Runs) == 0 {
		m.runCursor = 0
	} else {
		m.runCursor = minInt(maxInt(0, m.runCursor), len(m.data.Runs)-1)
	}
	for view, pos := range m.scroll {
		if pos < 0 {
			m.scroll[view] = 0
		}
	}
}

func (m *model) selectedRun() RunRecord {
	if len(m.data.Runs) == 0 {
		return m.data.Current
	}
	if m.runCursor < 0 || m.runCursor >= len(m.data.Runs) {
		return m.data.Runs[0]
	}
	return m.data.Runs[m.runCursor]
}

func (m *model) scrollFor(view viewID) int {
	if m.scroll == nil {
		m.scroll = map[viewID]int{}
	}
	return m.scroll[view]
}

func (m *model) setScroll(view viewID, value int) {
	if m.scroll == nil {
		m.scroll = map[viewID]int{}
	}
	m.scroll[view] = maxInt(0, value)
}
