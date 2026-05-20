package adapters

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
)

func hasNodeBin(root, name string) bool {
	return findNodeBin(root, name) != ""
}

func findNodeBin(root, name string) string {
	binDir := filepath.Join(root, "node_modules", ".bin")
	for _, candidate := range []string{name, name + ".cmd", name + ".ps1"} {
		path := filepath.Join(binDir, candidate)
		if _, err := os.Stat(path); err == nil {
			if abs, absErr := filepath.Abs(path); absErr == nil {
				return abs
			}
			return path
		}
	}
	return ""
}

func nodeToolCommand(ctx context.Context, root, name string, args ...string) *exec.Cmd {
	command := findNodeBin(root, name)
	if command == "" {
		command = name
	}
	return exec.CommandContext(ctx, command, args...)
}
