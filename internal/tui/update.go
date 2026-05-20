package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clampNavigation()
		return m, tea.ClearScreen
	case tea.KeyMsg:
		if m.commandMode {
			return m.updateCommandInput(msg)
		}
		return m.updateKey(msg)
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
			m.addCommandLog("command failed: " + msg.input)
			for _, line := range firstNonEmptyLines(msg.output+"\n"+msg.err, 3) {
				m.addCommandLog(line)
			}
		} else {
			m.addCommandLog("command done: " + msg.input)
			for _, line := range firstNonEmptyLines(msg.output, 3) {
				m.addCommandLog(line)
			}
		}
		m.refresh()
	case openDoneMsg:
		if msg.err != "" {
			m.lastNotice = "open report failed: " + msg.err
		} else if msg.path != "" {
			m.lastNotice = "opened report: " + msg.path
		}
	}
	return m, nil
}

func (m *model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if m.helpVisible {
		switch key {
		case "esc", "?", "q":
			m.helpVisible = false
			return m, nil
		}
	}
	if m.detailOpen {
		switch key {
		case "esc", "enter", "q":
			m.detailOpen = false
			return m, nil
		}
	}
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.helpVisible = false
		m.detailOpen = false
	case "?":
		m.helpVisible = true
	case "r":
		m.refresh()
	case "tab":
		m.nextView()
	case "shift+tab", "backtab":
		m.prevView()
	case "1", "2", "3", "4", "5", "6":
		m.setViewByKey(key)
	case "d":
		m.activeView = viewDoctor
	case "o":
		return m, m.openReport()
	case "enter":
		m.detailOpen = true
	case ":":
		m.commandMode = true
		m.commandInput = ""
	case "up", "k":
		m.moveSelection(-1)
	case "down", "j":
		m.moveSelection(1)
	case "pgup":
		m.moveSelection(-5)
	case "pgdown":
		m.moveSelection(5)
	case "home":
		m.runCursor = 0
		m.setScroll(m.activeView, 0)
	case "end":
		if len(m.data.Runs) > 0 {
			m.runCursor = len(m.data.Runs) - 1
		}
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

func (m *model) nextView() {
	m.activeView = viewID((int(m.activeView) + 1) % len(viewLabels))
}

func (m *model) prevView() {
	next := int(m.activeView) - 1
	if next < 0 {
		next = len(viewLabels) - 1
	}
	m.activeView = viewID(next)
}

func (m *model) setViewByKey(key string) {
	var n int
	if _, err := fmt.Sscanf(key, "%d", &n); err == nil && n >= 1 && n <= len(viewLabels) {
		m.activeView = viewID(n - 1)
	}
}

func (m *model) moveSelection(delta int) {
	switch m.activeView {
	case viewRuns:
		if len(m.data.Runs) > 0 {
			m.runCursor = minInt(maxInt(0, m.runCursor+delta), len(m.data.Runs)-1)
		}
	case viewReport, viewLogs, viewSkills, viewDoctor:
		m.setScroll(m.activeView, m.scrollFor(m.activeView)+delta)
	default:
		m.setScroll(m.activeView, m.scrollFor(m.activeView)+delta)
	}
}
