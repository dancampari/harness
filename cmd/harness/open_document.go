package harness

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func openReportIfInteractive(path string) {
	if path == "" || !isTerminal(os.Stdin) || !isTerminal(os.Stdout) {
		return
	}
	if disabled := strings.TrimSpace(os.Getenv("HARNESS_OPEN_REPORT")); disabled == "0" || strings.EqualFold(disabled, "false") {
		return
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	if err := openDocument(abs); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not open report %s: %v\n", path, err)
		return
	}
	fmt.Printf("  Opened: %s\n", path)
}

func openDocument(path string) error {
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
