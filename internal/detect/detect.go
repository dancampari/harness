// Package detect identifies the project's language/build stack.
package detect

import (
	"os"
	"path/filepath"
)

// DetectStack returns a string identifier for the project's primary stack.
// The order of checks matters: typescript wins over node when tsconfig.json
// is present alongside package.json.
func DetectStack(root string) string {
	exists := func(name string) bool {
		_, err := os.Stat(filepath.Join(root, name))
		return err == nil
	}

	switch {
	case exists("tsconfig.json"):
		return "typescript"
	case exists("package.json"):
		return "node"
	case exists("pyproject.toml"), exists("requirements.txt"), exists("setup.py"):
		return "python"
	case exists("go.mod"):
		return "go"
	case exists("Cargo.toml"):
		return "rust"
	case exists("pom.xml"):
		return "java-maven"
	case exists("build.gradle"), exists("build.gradle.kts"):
		return "java-gradle"
	case exists("composer.json"):
		return "php"
	case exists("Gemfile"):
		return "ruby"
	case exists("mix.exs"):
		return "elixir"
	}
	return "unknown"
}

// HasFile reports whether a file or directory exists at the given path
// relative to root. Useful for adapters checking for tool config.
func HasFile(root, name string) bool {
	_, err := os.Stat(filepath.Join(root, name))
	return err == nil
}
