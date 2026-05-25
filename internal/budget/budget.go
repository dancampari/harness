// Package budget estimates the agent context cost of the files an agent
// must read at session start: .specs/project/PROJECT.md,
// .specs/project/STATE.md, the agent protocol, the brownfield codebase
// context bundle, and the current feature's spec/design/tasks artifacts.
//
// The estimate is a rough byte/token proxy, not a real tokenizer. The
// goal is to make context drift visible so users can prune long-running
// project memory before the agent hits its window limit.
package budget

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// TokensPerByte is the heuristic we apply to convert raw bytes into an
// approximate token count for English/code mixed text. Four bytes per
// token is the standard rule-of-thumb used by OpenAI's tokenizer docs
// and matches Anthropic's own published guidance within a few percent.
const TokensPerByte = 0.25

// File records the size cost of one harness artifact.
type File struct {
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
}

// Snapshot is the result of inspecting a harness directory.
type Snapshot struct {
	HarnessDir    string `json:"harness_dir"`
	Files         []File `json:"files"`
	TotalBytes    int64  `json:"total_bytes"`
	TokenEstimate int64  `json:"token_estimate"`
	// SoftLimitTokens is the threshold beyond which the snapshot is
	// considered to be eating into useful working memory. Doctor warns
	// above this value.
	SoftLimitTokens int64 `json:"soft_limit_tokens"`
}

// DefaultSoftLimit is the threshold Doctor uses to warn. Forty thousand
// tokens leaves ~160k for the agent's actual work in a 200k-token model
// window.
const DefaultSoftLimit int64 = 40_000

// Inspect walks the standard set of Harness context files for a sprint
// and returns a Snapshot of the byte cost. sprintNumber may be zero,
// in which case only the persistent project memory, protocol, and
// codebase context files are counted.
func Inspect(harnessDir string, sprintNumber int) (*Snapshot, error) {
	if harnessDir == "" {
		return nil, fmt.Errorf("harnessDir is required")
	}
	snap := &Snapshot{
		HarnessDir:      harnessDir,
		SoftLimitTokens: DefaultSoftLimit,
	}

	specsRoot := specsRootFromHarness(harnessDir)

	// PROJECT.md / STATE.md live under .specs/project/ after migration;
	// the pre-Phase-2 names .harness/spec.md and .harness/progress.md
	// are still counted so legacy projects continue to surface drift.
	persistent := []string{
		filepath.Join(specsRoot, "project", "PROJECT.md"),
		filepath.Join(specsRoot, "project", "STATE.md"),
		filepath.Join(harnessDir, "spec.md"),
		filepath.Join(harnessDir, "progress.md"),
		filepath.Join(harnessDir, "agent-protocol.md"),
	}
	for _, p := range persistent {
		addIfExists(snap, p)
	}

	// Brownfield codebase mapping: every .md file under .specs/codebase
	// (canonical) or .harness/context (legacy).
	for _, dir := range []string{
		filepath.Join(specsRoot, "codebase"),
		filepath.Join(harnessDir, "context"),
	} {
		_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if filepath.Ext(d.Name()) == ".md" {
				addIfExists(snap, path)
			}
			return nil
		})
	}

	if sprintNumber > 0 {
		slug := fmt.Sprintf("sprint-%03d", sprintNumber)
		featureDir := filepath.Join(specsRoot, "features", slug)
		for _, p := range []string{
			filepath.Join(featureDir, "spec.md"),
			filepath.Join(featureDir, "design.md"),
			filepath.Join(featureDir, "tasks.md"),
			filepath.Join(harnessDir, "contracts", slug+".md"),
			filepath.Join(harnessDir, "design", slug+".md"),
			filepath.Join(harnessDir, "tasks", slug+".md"),
		} {
			addIfExists(snap, p)
		}
	}

	sort.Slice(snap.Files, func(i, j int) bool { return snap.Files[i].Path < snap.Files[j].Path })
	snap.TokenEstimate = int64(float64(snap.TotalBytes) * TokensPerByte)
	return snap, nil
}

// OverBudget reports whether the snapshot exceeds its soft limit.
func (s *Snapshot) OverBudget() bool {
	return s.TokenEstimate > s.SoftLimitTokens
}

func addIfExists(s *Snapshot, path string) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return
	}
	s.Files = append(s.Files, File{Path: path, Bytes: info.Size()})
	s.TotalBytes += info.Size()
}

// specsRootFromHarness mirrors the same .specs/ sibling derivation used
// by agreement.Manager so both sides see the same workspace layout.
func specsRootFromHarness(harnessDir string) string {
	clean := filepath.Clean(harnessDir)
	parent := filepath.Dir(clean)
	if parent == "" || parent == "." {
		return ".specs"
	}
	return filepath.Join(parent, ".specs")
}
