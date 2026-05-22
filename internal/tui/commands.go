package tui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

// commandStream is the live channel for one running command. The TUI
// drains it one message at a time via waitForStreamMsg; the producer
// goroutines push commandLineMsg per output line and a final
// commandExitMsg, then close the channel.
type commandStream struct {
	ch chan tea.Msg
}

// startCommandCmd returns a tea.Cmd that spawns the command and begins
// streaming its combined output. Process creation is deferred into the
// returned closure so callers that never run the Bubble Tea loop (unit
// tests inspecting executeCommand) do not spawn a subprocess.
func startCommandCmd(root, input string) tea.Cmd {
	return func() tea.Msg {
		stream, err := spawnCommandStream(root, input)
		if err != nil {
			return commandStartedMsg{input: input, err: err.Error()}
		}
		return commandStartedMsg{input: input, stream: stream}
	}
}

// spawnCommandStream starts the process and wires its stdout+stderr into
// a single ordered channel. stdout and stderr are scanned concurrently;
// cmd.Wait is called only after both pipes drain, per os/exec docs.
func spawnCommandStream(root, input string) (*commandStream, error) {
	projectRoot := projectRootForHarness(root)
	var cmd *exec.Cmd
	if strings.HasPrefix(input, "!") {
		cmd = shellCommand(strings.TrimSpace(strings.TrimPrefix(input, "!")))
	} else {
		args, err := harnessCommandArgs(input)
		if err != nil {
			return nil, err
		}
		exe, err := os.Executable()
		if err != nil {
			return nil, err
		}
		cmd = exec.Command(exe, args...)
	}
	cmd.Dir = projectRoot

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	stream := &commandStream{ch: make(chan tea.Msg, 256)}
	var wg sync.WaitGroup
	wg.Add(2)
	scan := func(r io.Reader) {
		defer wg.Done()
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 64*1024), 1<<20)
		for sc.Scan() {
			line := strings.TrimRight(sc.Text(), "\r")
			if strings.TrimSpace(line) == "" {
				continue
			}
			stream.ch <- commandLineMsg{stream: stream, line: line}
		}
	}
	go scan(stdout)
	go scan(stderr)
	go func() {
		wg.Wait() // both pipes fully drained before reaping the process
		exit := commandExitMsg{input: input}
		if err := cmd.Wait(); err != nil {
			exit.err = err.Error()
		}
		stream.ch <- exit
		close(stream.ch)
	}()
	return stream, nil
}

// waitForStreamMsg reads the next message from a live command stream.
// A closed channel yields nil, which Bubble Tea treats as a no-op.
func waitForStreamMsg(stream *commandStream) tea.Cmd {
	if stream == nil {
		return nil
	}
	return func() tea.Msg {
		msg, ok := <-stream.ch
		if !ok {
			return nil
		}
		return msg
	}
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

func (m *model) openReport() tea.Cmd {
	path := m.data.ReportPath
	if path == "" {
		selected := m.selectedRun()
		path = selected.ReportPath
	}
	if path == "" {
		m.addNotice("report.open.failed", "no report available")
		return nil
	}
	return func() tea.Msg {
		if err := openPath(path); err != nil {
			return openDoneMsg{path: path, err: err.Error()}
		}
		return openDoneMsg{path: path}
	}
}

func openPath(path string) error {
	abs, err := filepath.Abs(path)
	if err == nil {
		path = abs
	}
	if editor := strings.TrimSpace(os.Getenv("HARNESS_EDITOR")); editor != "" {
		return startCommand(editor, path)
	}
	for _, editor := range []string{"cursor", "code"} {
		if resolved, err := exec.LookPath(editor); err == nil {
			return startCommand(resolved, path)
		}
	}
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", path).Start()
	case "darwin":
		return exec.Command("open", path).Start()
	default:
		return exec.Command("xdg-open", path).Start()
	}
}

func startCommand(command string, args ...string) error {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return nil
	}
	cmd := exec.Command(fields[0], append(fields[1:], args...)...)
	return cmd.Start()
}

func (m *model) addCommandLog(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	m.commandLog = append(m.commandLog, line)
	if len(m.commandLog) > 8 {
		m.commandLog = m.commandLog[len(m.commandLog)-8:]
	}
}

// lastNonEmptyLines returns up to limit trailing non-empty lines from
// the streamed output, used to summarise a finished command in the
// rolling command log.
func lastNonEmptyLines(lines []string, limit int) []string {
	var out []string
	for i := len(lines) - 1; i >= 0 && len(out) < limit; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		out = append([]string{line}, out...)
	}
	return out
}
