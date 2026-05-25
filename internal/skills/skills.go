// Package skills ships the agent-readable skill packs the harness
// installs into a consuming project. Two packs ship today:
//
//   - tlc-spec-driven: vendored verbatim from the canonical
//     Tech Lead's Club spec-driven skill at
//     github.com/tech-leads-club/agent-skills. TLC is the
//     methodology — the agent reads it to know how to specify,
//     design, break into tasks, implement, and validate features.
//
//   - harness-gate: the deterministic protocol unique to the
//     harness — agreement hash, planner/tester roles, QA dimensions,
//     events.jsonl, run-progress.json, edit guard. The agent reads it
//     to know how the harness enforces TLC's method as code.
//
// Both are embedded into the binary via go:embed so the install is
// deterministic and offline. To bump TLC, replace the files under
// tlc/tlc-spec-driven/ and rebuild.
package skills

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed all:tlc/tlc-spec-driven
var tlcFS embed.FS

//go:embed all:gate/harness-gate
var gateFS embed.FS

// LegacyDirs lists skill directories from earlier versions of the
// harness that should be removed on install/upgrade so a project ends
// up with only the canonical pair (tlc-spec-driven + harness-gate).
var LegacyDirs = []string{
	"spec-driven",
	"contract-authoring",
	"contract-review",
}

// Pack is one installable skill pack.
type Pack struct {
	// Name is the directory it installs to under .harness/skills/.
	Name string
	// fs is the embedded source.
	fs embed.FS
	// prefix is the directory inside the embedded FS to walk from.
	prefix string
}

// Packs returns the two packs shipped by this binary in install order:
// TLC first (methodology the agent reads), harness-gate second (the
// gate protocol layered on top).
func Packs() []Pack {
	return []Pack{
		{Name: "tlc-spec-driven", fs: tlcFS, prefix: "tlc/tlc-spec-driven"},
		{Name: "harness-gate", fs: gateFS, prefix: "gate/harness-gate"},
	}
}

// Install writes both packs into harnessDir/skills/, overwriting any
// existing files. It also removes any LegacyDirs left behind by older
// harness versions so the agent never sees stale parallel hierarchies.
func Install(harnessDir string) error {
	root := filepath.Join(harnessDir, "skills")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("mkdir skills: %w", err)
	}
	for _, legacy := range LegacyDirs {
		_ = os.RemoveAll(filepath.Join(root, legacy))
	}
	for _, pack := range Packs() {
		if err := pack.extract(root); err != nil {
			return fmt.Errorf("install %s: %w", pack.Name, err)
		}
	}
	return nil
}

// Installed reports whether both canonical packs are present and carry
// a SKILL.md. The doctor uses this to verify a project is up to date.
func Installed(harnessDir string) bool {
	root := filepath.Join(harnessDir, "skills")
	for _, pack := range Packs() {
		if _, err := os.Stat(filepath.Join(root, pack.Name, "SKILL.md")); err != nil {
			return false
		}
	}
	return true
}

// VendoredHash returns the SHA-256 of every file the named pack would
// install. It is the canonical content fingerprint of the vendored
// version compiled into THIS binary. Doctor compares it to
// InstalledHash for the same pack to detect drift between what the
// binary ships and what is on disk in a project.
//
// The hash is computed by walking the embedded FS in lexicographic
// order and feeding each file's relative path and bytes into the
// digest. Path ordering keeps the hash stable across rebuilds.
func VendoredHash(packName string) (string, error) {
	var pack *Pack
	for _, p := range Packs() {
		if p.Name == packName {
			pCopy := p
			pack = &pCopy
			break
		}
	}
	if pack == nil {
		return "", fmt.Errorf("unknown pack %q", packName)
	}
	h := sha256.New()
	walkErr := fs.WalkDir(pack.fs, pack.prefix, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(path, pack.prefix), "/")
		body, readErr := pack.fs.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		fmt.Fprintf(h, "%s\n", rel)
		h.Write(body)
		return nil
	})
	if walkErr != nil {
		return "", walkErr
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// InstalledHash returns the SHA-256 of the named pack as it exists on
// disk under harnessDir/skills/<packName>/. Mirrors VendoredHash's
// ordering so a byte-perfect install yields the same digest. Returns
// an empty string with no error when the pack directory does not exist
// — doctor treats that as "not installed" rather than drift.
func InstalledHash(harnessDir, packName string) (string, error) {
	root := filepath.Join(harnessDir, "skills", packName)
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	h := sha256.New()
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		fmt.Fprintf(h, "%s\n", rel)
		h.Write(body)
		return nil
	})
	if walkErr != nil {
		return "", walkErr
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// extract walks the embedded FS and copies every file under
// pack.prefix into root/pack.Name/, preserving the relative layout.
func (p Pack) extract(root string) error {
	return fs.WalkDir(p.fs, p.prefix, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(path, p.prefix)
		rel = strings.TrimPrefix(rel, "/")
		target := filepath.Join(root, p.Name, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		body, err := p.fs.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, body, 0o644)
	})
}
