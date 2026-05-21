// Package budget estimates the agent context cost of the harness files
// an agent must read at session start: spec.md, progress.md, the agent
// protocol, the brownfield context bundle, and the current sprint's
// contract/design/tasks artifacts.
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
	HarnessDir   string `json:"harness_dir"`
	Files        []File `json:"files"`
	TotalBytes   int64  `json:"total_bytes"`
	TokenEstimate int64 `json:"token_estimate"`
	// SoftLimitTokens is the threshold beyond which the snapshot is
	// considered to be eating into useful working memory. Doctor warns
	// above this value.
	SoftLimitTokens int64 `json:"soft_limit_tokens"`
}

// DefaultSoftLimit is the threshold Doctor uses to warn. Forty thousand
// tokens leaves ~160k for the agent's actual work in a 200k-token model
// window.
const DefaultSoftLimit int64 = 40_000

// Inspect walks the standard set of harness context files for a sprint
// and returns a Snapshot of the byte cost. sprintNumber may be zero,
// in which case only the persistent files (spec, progress, protocol,
// context/*) are counted.
func Inspect(harnessDir string, sprintNumber int) (*Snapshot, error) {
	if harnessDir == "" {
		return nil, fmt.Errorf("harnessDir is required")
	}
	snap := &Snapshot{
		HarnessDir:      harnessDir,
		SoftLimitTokens: DefaultSoftLimit,
	}

	persistent := []string{
		filepath.Join(harnessDir, "spec.md"),
		filepath.Join(harnessDir, "progress.md"),
		filepath.Join(harnessDir, "agent-protocol.md"),
	}
	for _, p := range persistent {
		addIfExists(snap, p)
	}

	// Brownfield context bundle: every .md file under .harness/context.
	contextDir := filepath.Join(harnessDir, "context")
	_ = filepath.WalkDir(contextDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if filepath.Ext(d.Name()) == ".md" {
			addIfExists(snap, path)
		}
		return nil
	})

	if sprintNumber > 0 {
		for _, p := range []string{
			filepath.Join(harnessDir, "contracts", fmt.Sprintf("sprint-%03d.md", sprintNumber)),
			filepath.Join(harnessDir, "design", fmt.Sprintf("sprint-%03d.md", sprintNumber)),
			filepath.Join(harnessDir, "tasks", fmt.Sprintf("sprint-%03d.md", sprintNumber)),
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
