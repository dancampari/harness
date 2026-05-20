package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

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

func (m *model) openReport() tea.Cmd {
	path := m.data.ReportPath
	if path == "" {
		selected := m.selectedRun()
		path = selected.ReportPath
	}
	if path == "" {
		m.lastNotice = "no report available"
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
