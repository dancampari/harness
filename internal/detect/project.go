package detect

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ProjectInfo is the auto-detected shape of the repository where harness runs.
type ProjectInfo struct {
	Name           string
	Stack          string
	PackageManager string
	Frameworks     []string
	CodingCLIs     []string
}

// DetectProject inspects common project markers without executing tools.
func DetectProject(root string) ProjectInfo {
	info := ProjectInfo{
		Name:           filepath.Base(absOr(root, root)),
		Stack:          DetectStack(root),
		PackageManager: detectPackageManager(root),
		Frameworks:     detectFrameworks(root),
		CodingCLIs:     detectCodingCLIs(root),
	}
	if name := detectProjectName(root, info.Stack); name != "" {
		info.Name = name
	}
	return info
}

func detectPackageManager(root string) string {
	switch {
	case HasFile(root, "pnpm-lock.yaml"):
		return "pnpm"
	case HasFile(root, "yarn.lock"):
		return "yarn"
	case HasFile(root, "bun.lockb"), HasFile(root, "bun.lock"):
		return "bun"
	case HasFile(root, "package-lock.json"):
		return "npm"
	case HasFile(root, "package.json"):
		return "npm"
	}
	return ""
}

func detectFrameworks(root string) []string {
	frameworks := map[string]bool{}
	if HasFile(root, "next.config.js") || HasFile(root, "next.config.mjs") ||
		HasFile(root, "next.config.ts") {
		frameworks["nextjs"] = true
	}
	if HasFile(root, "vite.config.js") || HasFile(root, "vite.config.ts") ||
		HasFile(root, "vite.config.mjs") {
		frameworks["vite"] = true
	}
	if HasFile(root, "playwright.config.ts") || HasFile(root, "playwright.config.js") {
		frameworks["playwright"] = true
	}
	for _, dep := range packageDeps(root) {
		switch dep {
		case "next":
			frameworks["nextjs"] = true
		case "react":
			frameworks["react"] = true
		case "vue":
			frameworks["vue"] = true
		case "svelte", "@sveltejs/kit":
			frameworks["svelte"] = true
		case "express":
			frameworks["express"] = true
		case "nestjs", "@nestjs/core":
			frameworks["nestjs"] = true
		}
	}
	return sortedKeys(frameworks)
}

func detectCodingCLIs(root string) []string {
	clis := map[string]bool{}
	if HasFile(root, "AGENTS.md") || HasFile(root, ".codex") {
		clis["codex"] = true
	}
	if HasFile(root, ".claude") || HasFile(root, "CLAUDE.md") {
		clis["claude"] = true
	}
	if HasFile(root, ".cursor") || HasFile(root, ".cursorrules") {
		clis["cursor"] = true
	}
	return sortedKeys(clis)
}

func detectProjectName(root, stack string) string {
	switch stack {
	case "node", "typescript":
		var pkg struct {
			Name string `json:"name"`
		}
		if readJSON(filepath.Join(root, "package.json"), &pkg) == nil {
			return pkg.Name
		}
	case "go":
		if b, err := os.ReadFile(filepath.Join(root, "go.mod")); err == nil {
			for _, line := range strings.Split(string(b), "\n") {
				if strings.HasPrefix(line, "module ") {
					parts := strings.Split(strings.TrimSpace(line), "/")
					return parts[len(parts)-1]
				}
			}
		}
	case "rust":
		if b, err := os.ReadFile(filepath.Join(root, "Cargo.toml")); err == nil {
			for _, line := range strings.Split(string(b), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "name") && strings.Contains(line, "=") {
					return strings.Trim(strings.TrimSpace(strings.SplitN(line, "=", 2)[1]), `"`)
				}
			}
		}
	}
	return ""
}

func packageDeps(root string) []string {
	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if readJSON(filepath.Join(root, "package.json"), &pkg) != nil {
		return nil
	}
	out := make([]string, 0, len(pkg.Dependencies)+len(pkg.DevDependencies))
	for name := range pkg.Dependencies {
		out = append(out, name)
	}
	for name := range pkg.DevDependencies {
		out = append(out, name)
	}
	return out
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})
	return json.Unmarshal(b, v)
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func absOr(path, fallback string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fallback
	}
	return abs
}
