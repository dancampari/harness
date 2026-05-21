package adapters

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

func hasProjectCommand(root, name string) bool {
	return findProjectCommand(root, name) != ""
}

func findProjectCommand(root, name string) string {
	candidates := []string{
		filepath.Join(root, ".venv", "Scripts", name+".exe"),
		filepath.Join(root, ".venv", "Scripts", name+".cmd"),
		filepath.Join(root, ".venv", "bin", name),
		filepath.Join(root, "venv", "Scripts", name+".exe"),
		filepath.Join(root, "venv", "Scripts", name+".cmd"),
		filepath.Join(root, "venv", "bin", name),
		filepath.Join(root, "node_modules", ".bin", name),
		filepath.Join(root, "node_modules", ".bin", name+".cmd"),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	if path, err := exec.LookPath(name); err == nil {
		return path
	}
	return ""
}

func commandOrName(root, name string) string {
	if path := findProjectCommand(root, name); path != "" {
		return path
	}
	return name
}

var toolLineRE = regexp.MustCompile(`^(.+?):(\d+)(?::\d+)?:\s*(.*)$`)

func parseToolLine(line string) (file string, lineNo int, message string) {
	m := toolLineRE.FindStringSubmatch(strings.TrimSpace(line))
	if len(m) != 4 {
		return "", 0, strings.TrimSpace(line)
	}
	n, _ := strconv.Atoi(m[2])
	return filepath.ToSlash(m[1]), n, strings.TrimSpace(m[3])
}

// readDirSafe lists entries in dir as names, returning an empty slice
// instead of an error when the directory does not exist. Used by
// adapters that opportunistically read project artifacts.
func readDirSafe(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		out = append(out, e.Name())
	}
	return out, nil
}

// readFileSafe reads path as a string, returning the empty string on
// any error so callers can degrade gracefully.
func readFileSafe(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
