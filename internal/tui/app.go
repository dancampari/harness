// Package tui renders the live Harness terminal interface.
package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	animationInterval = 120 * time.Millisecond
	refreshInterval   = 750 * time.Millisecond
)

// Run launches the TUI. If resume is true, state is loaded from .harness/.
func Run(harnessDir string, resume bool, version string) error {
	m := newModel(harnessDir, resume, version)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

type tickMsg time.Time

type commandDoneMsg struct {
	input  string
	output string
	err    string
}

type openDoneMsg struct {
	path string
	err  string
}

func tick() tea.Cmd {
	return tea.Tick(animationInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}
