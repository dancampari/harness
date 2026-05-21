// Package workspace computes a deterministic content hash of the working
// tree. The hash pins a QA report to the exact code state that produced it
// so harness sprint score can refuse to consolidate when the working tree
// has drifted since QA ran.
//
// The hash is intentionally git-independent: it works in repos that have
// not been initialised, but it ignores directories that are never the
// product source (.git, .harness, dependency caches, build outputs).
package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
)

// SkippedDirs are walked but their contents are excluded from the hash.
// Anything inside these directories is considered transient and should
// not affect the agreement between QA and consolidation.
var SkippedDirs = map[string]bool{
	".git":          true,
	".harness":      true,
	"node_modules":  true,
	"dist":          true,
	"build":         true,
	"coverage":      true,
	"target":        true,
	".next":         true,
	".cache":        true,
	".turbo":        true,
	".pytest_cache": true,
	"__pycache__":   true,
	"venv":          true,
	".venv":         true,
}

// MaxFileBytes caps per-file content read so very large generated assets
// do not dominate hashing time. Files above the cap contribute their size
// and relative path instead of their full content.
const MaxFileBytes = 4 * 1024 * 1024 // 4 MiB

// Hash returns a 64-character hex digest of the workspace rooted at root.
// The function is best-effort: unreadable files are skipped silently.
func Hash(root string) (string, error) {
	type entry struct {
		rel string
		sha string
	}
	var entries []entry

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if d.IsDir() {
			if SkippedDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return nil
		}
		if info.Size() > MaxFileBytes {
			h := sha256.Sum256([]byte(rel + "\nlarge:" + strconv.FormatInt(info.Size(), 10)))
			entries = append(entries, entry{rel: rel, sha: hex.EncodeToString(h[:])})
			return nil
		}
		f, openErr := os.Open(path)
		if openErr != nil {
			return nil
		}
		sh := sha256.New()
		_, _ = io.WriteString(sh, rel+"\n")
		_, _ = io.Copy(sh, f)
		_ = f.Close()
		entries = append(entries, entry{rel: rel, sha: hex.EncodeToString(sh.Sum(nil))})
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].rel < entries[j].rel })
	h := sha256.New()
	for _, e := range entries {
		_, _ = io.WriteString(h, e.rel+":"+e.sha+"\n")
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
