package adapters

import (
	"os"
	"path/filepath"
)

func hasNodeBin(root, name string) bool {
	binDir := filepath.Join(root, "node_modules", ".bin")
	for _, candidate := range []string{name, name + ".cmd", name + ".ps1"} {
		if _, err := os.Stat(filepath.Join(binDir, candidate)); err == nil {
			return true
		}
	}
	return false
}
