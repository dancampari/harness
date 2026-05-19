package adapters

import (
	"io/fs"
	"path/filepath"
	"strings"
)

func nodeSourceFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			switch name {
			case ".git", ".harness", "node_modules", "coverage", "dist", "build", ".next":
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(name))
		switch ext {
		case ".js", ".jsx", ".ts", ".tsx":
			if strings.HasSuffix(name, ".d.ts") {
				return nil
			}
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func slashRel(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}
