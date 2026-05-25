package harness

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// migrateLegacyArtifactsToSpecs moves the pre-Phase-2 artifact layout to
// the TLC-style .specs/ tree, idempotently:
//
//	.harness/contracts/sprint-NNN.md   → .specs/features/sprint-NNN/spec.md
//	.harness/design/sprint-NNN.md      → .specs/features/sprint-NNN/design.md
//	.harness/tasks/sprint-NNN.md       → .specs/features/sprint-NNN/tasks.md
//	.harness/spec.md                   → .specs/project/PROJECT.md
//	.harness/progress.md               → .specs/project/STATE.md
//	.harness/context/*.md              → .specs/codebase/*.md
//
// Lock files (.harness/contracts/*.lock.json), approvals, reports,
// evaluations, repairs, and screenshots stay in .harness/ as runtime
// state. Lock files reference the spec by canonical path implicitly via
// agreement.Manager.ContractPath, which now prefers .specs/.
//
// The function returns one-line summaries describing what moved (so the
// caller can print them) and never overwrites a destination that already
// exists — that lets re-running `harness upgrade` be a no-op.
func migrateLegacyArtifactsToSpecs(harnessRoot string) ([]string, error) {
	specsRoot := siblingSpecsRoot(harnessRoot)
	var summary []string

	moves, err := planSprintArtifactMoves(harnessRoot, specsRoot)
	if err != nil {
		return nil, err
	}
	for _, mv := range moves {
		if err := moveIfAbsent(mv.from, mv.to); err != nil {
			return nil, err
		}
		summary = append(summary, fmt.Sprintf("migrated %s → %s",
			strings.ReplaceAll(mv.fromRel, `\`, "/"),
			strings.ReplaceAll(mv.toRel, `\`, "/")))
	}

	projectMoves := []struct {
		fromRel, toRel string
	}{
		{filepath.Join(harnessRoot, "spec.md"), filepath.Join(specsRoot, "project", "PROJECT.md")},
		{filepath.Join(harnessRoot, "progress.md"), filepath.Join(specsRoot, "project", "STATE.md")},
	}
	for _, mv := range projectMoves {
		if shouldMigrateFile(mv.fromRel, mv.toRel) {
			if err := moveIfAbsent(mv.fromRel, mv.toRel); err != nil {
				return nil, err
			}
			summary = append(summary, fmt.Sprintf("migrated %s → %s",
				strings.ReplaceAll(mv.fromRel, `\`, "/"),
				strings.ReplaceAll(mv.toRel, `\`, "/")))
		}
	}

	contextMoves, err := planContextMoves(harnessRoot, specsRoot)
	if err != nil {
		return nil, err
	}
	for _, mv := range contextMoves {
		if err := moveIfAbsent(mv.from, mv.to); err != nil {
			return nil, err
		}
		summary = append(summary, fmt.Sprintf("migrated %s → %s",
			strings.ReplaceAll(mv.fromRel, `\`, "/"),
			strings.ReplaceAll(mv.toRel, `\`, "/")))
	}

	return summary, nil
}

type plannedMove struct {
	from, to       string
	fromRel, toRel string
}

func planSprintArtifactMoves(harnessRoot, specsRoot string) ([]plannedMove, error) {
	var moves []plannedMove
	dirMap := []struct {
		legacySub string
		specsName string
	}{
		{"contracts", "spec.md"},
		{"design", "design.md"},
		{"tasks", "tasks.md"},
	}
	re := regexp.MustCompile(`^(sprint-\d+)\.md$`)
	for _, d := range dirMap {
		legacyDir := filepath.Join(harnessRoot, d.legacySub)
		entries, err := os.ReadDir(legacyDir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			match := re.FindStringSubmatch(entry.Name())
			if match == nil {
				continue
			}
			slug := match[1]
			from := filepath.Join(legacyDir, entry.Name())
			to := filepath.Join(specsRoot, "features", slug, d.specsName)
			if shouldMigrateFile(from, to) {
				moves = append(moves, plannedMove{
					from: from, to: to,
					fromRel: from, toRel: to,
				})
			}
		}
	}
	sort.Slice(moves, func(i, j int) bool { return moves[i].toRel < moves[j].toRel })
	return moves, nil
}

func planContextMoves(harnessRoot, specsRoot string) ([]plannedMove, error) {
	legacyDir := filepath.Join(harnessRoot, "context")
	entries, err := os.ReadDir(legacyDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var moves []plannedMove
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		from := filepath.Join(legacyDir, entry.Name())
		to := filepath.Join(specsRoot, "codebase", entry.Name())
		if shouldMigrateFile(from, to) {
			moves = append(moves, plannedMove{
				from: from, to: to,
				fromRel: from, toRel: to,
			})
		}
	}
	return moves, nil
}

// shouldMigrateFile returns true when the source exists and the
// destination does not. Both checks are required so the migration is
// idempotent: a second `harness upgrade` after the rename is a no-op.
func shouldMigrateFile(from, to string) bool {
	if _, err := os.Stat(from); err != nil {
		return false
	}
	if _, err := os.Stat(to); err == nil {
		return false
	}
	return true
}

func moveIfAbsent(from, to string) error {
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	if err := os.Rename(from, to); err != nil {
		// Cross-device renames fall through to a copy + remove. Rare on
		// project trees but the upgrade should never fail because of it.
		data, readErr := os.ReadFile(from)
		if readErr != nil {
			return err
		}
		if writeErr := os.WriteFile(to, data, 0o644); writeErr != nil {
			return writeErr
		}
		return os.Remove(from)
	}
	return nil
}
