// Package tui renders the live Harness terminal interface.
package tui

import (
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"
)

const (
	animationInterval = 120 * time.Millisecond
	refreshInterval   = 750 * time.Millisecond
)

type Options struct {
	AltScreen bool
}

// Run launches the TUI. If resume is true, state is loaded from .harness/.
func Run(harnessDir string, resume bool, version string) error {
	return RunWithOptions(harnessDir, resume, version, Options{AltScreen: true})
}

// RunWithOptions launches the TUI with explicit terminal rendering options.
func RunWithOptions(harnessDir string, resume bool, version string, opts Options) error {
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return fmt.Errorf("harness run requires an interactive terminal (TTY); open PowerShell, Windows Terminal, or the VS Code terminal and run `harness run --resume`")
	}

	m := newModel(harnessDir, resume, version)
	programOptions := []tea.ProgramOption{}
	if opts.AltScreen {
		programOptions = append(programOptions, tea.WithAltScreen())
	}
	p := tea.NewProgram(m, programOptions...)
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
