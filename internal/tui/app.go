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

// commandStartedMsg is emitted once a streamed command's process has
// been spawned. It carries the live stream handle, or a start error
// when the process could not be launched.
type commandStartedMsg struct {
	input  string
	stream *commandStream
	err    string
}

// commandLineMsg carries one line of streamed command output. The
// stream pointer lets the model ignore lines from a superseded run.
type commandLineMsg struct {
	stream *commandStream
	line   string
}

// commandExitMsg is emitted once the streamed command's process exits.
type commandExitMsg struct {
	input string
	err   string
}

type openDoneMsg struct {
	path string
	err  string
}

func tick() tea.Cmd {
	return tea.Tick(animationInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}
