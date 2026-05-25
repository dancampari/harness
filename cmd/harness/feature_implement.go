package harness

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dancampari/harness/internal/agreement"
	"github.com/dancampari/harness/internal/events"
	"github.com/dancampari/harness/internal/planner"
	"github.com/spf13/cobra"
)

// newFeatureImplementCmd wraps the atomic-commit step TLC's
// implement.md prescribes for each task in tasks.md. The agent runs:
//
//	harness feature implement 003
//
// The harness:
//  1. Resolves the active sprint, refuses to run before AGREED.
//  2. Locates task #003 in the active tasks.md (canonical .specs/...
//     or legacy .harness/tasks/...).
//  3. Confirms the staged diff is non-empty (run `git add` first).
//  4. Composes a Conventional Commits message from the task's title
//     (defaults to `feat: <title>`; can be overridden with --type).
//  5. Runs `git commit` for the staged changes.
//  6. Emits a `feature.implement.committed` event.
//
// The command never amends, never force-pushes, and never bypasses
// hooks — it is a thin convenience that makes one-task-per-commit the
// path of least resistance.
func newFeatureImplementCmd() *cobra.Command {
	var commitType string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "implement <task-id>",
		Short: "Wrap the atomic commit for one task from the active feature's tasks.md (TLC implement.md)",
		Long: `Implements TLC's "one commit per task" gate: looks up the named task
in the active feature's tasks.md, composes a Conventional Commits
message from its title, and runs ` + "`git commit`" + ` for the currently
staged changes.

Prerequisites:
  - the active feature contract is AGREED (` + "`harness contract status`" + `);
  - the task exists in .specs/features/<slug>/tasks.md (or the legacy
    .harness/tasks/sprint-NNN.md path);
  - the diff to commit is already staged via ` + "`git add`" + `.

Pass --dry-run to print the command without executing it.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			taskRef := args[0]
			mgr := agreement.NewManager(".harness")
			number, err := mgr.CurrentSprintNumber()
			if err != nil {
				return err
			}
			if number == 0 {
				return fmt.Errorf("no active feature contract; run `harness feature new \"<goal>\"` first")
			}
			status, err := mgr.Status(number)
			if err != nil {
				return err
			}
			if !strings.EqualFold(status.State, "agreed") {
				return fmt.Errorf("feature sprint-%03d is %s; implement is allowed only after AGREED — run propose/approve first",
					number, strings.ToUpper(status.State))
			}
			tasksPath := mgr.TasksPath(number)
			plan, err := planner.ParseTasks(tasksPath)
			if err != nil {
				return fmt.Errorf("read tasks plan at %s: %w", tasksPath, err)
			}
			if plan == nil || len(plan.Tasks) == 0 {
				return fmt.Errorf("tasks.md at %s has no parseable task entries", tasksPath)
			}
			task, ok := findTaskByRef(plan, taskRef)
			if !ok {
				return fmt.Errorf("task %q not found in %s", taskRef, tasksPath)
			}
			staged, err := hasStagedChanges()
			if err != nil {
				return err
			}
			if !staged {
				return fmt.Errorf("nothing is staged for commit; run `git add` to stage the task's changes before calling implement")
			}
			message := composeCommitMessage(commitType, task)
			if dryRun {
				fmt.Println("--dry-run: would run")
				fmt.Printf("  git commit -m %q\n", message)
				return nil
			}
			if err := runGitCommit(message); err != nil {
				return err
			}
			events.Record(".harness", "feature.implement.committed", events.PhaseBuild,
				fmt.Sprintf("sprint-%03d · task %d · %s", number, task.Number, task.Title), "")
			fmt.Printf("✓ Committed task %d (%s) for sprint-%03d\n", task.Number, task.Title, number)
			return nil
		},
	}
	cmd.Flags().StringVar(&commitType, "type", "feat", "Conventional Commits type: feat|fix|refactor|test|docs|chore|perf|build")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the commit command without executing it")
	return cmd
}

// findTaskByRef resolves an argument like "003" or "3" to a Task. We
// match on the numeric portion to keep CLI ergonomic; future revisions
// can support slug-based task IDs without breaking this signature.
func findTaskByRef(plan *planner.TaskPlan, ref string) (planner.Task, bool) {
	ref = strings.TrimSpace(ref)
	ref = strings.TrimLeft(ref, "0")
	if ref == "" {
		ref = "0"
	}
	for _, t := range plan.Tasks {
		if fmt.Sprintf("%d", t.Number) == ref {
			return t, true
		}
	}
	return planner.Task{}, false
}

// hasStagedChanges returns true when the git index carries anything to
// commit. We delegate to `git diff --cached --quiet` whose exit code 1
// means "differences exist".
func hasStagedChanges() (bool, error) {
	cmd := exec.Command("git", "diff", "--cached", "--quiet")
	if err := cmd.Run(); err != nil {
		// Exit code 1 → there ARE staged changes; anything else is a
		// real failure.
		var exitErr *exec.ExitError
		if asExitError(err, &exitErr) {
			if exitErr.ExitCode() == 1 {
				return true, nil
			}
		}
		return false, fmt.Errorf("git diff --cached --quiet: %w", err)
	}
	return false, nil
}

func asExitError(err error, dst **exec.ExitError) bool {
	if err == nil {
		return false
	}
	if e, ok := err.(*exec.ExitError); ok {
		*dst = e
		return true
	}
	return false
}

func composeCommitMessage(commitType string, task planner.Task) string {
	commitType = strings.ToLower(strings.TrimSpace(commitType))
	if commitType == "" {
		commitType = "feat"
	}
	title := task.Title
	if title == "" {
		title = fmt.Sprintf("task %d", task.Number)
	}
	subject := fmt.Sprintf("%s: %s", commitType, title)
	body := fmt.Sprintf("Implements task %d from %s.\n\nWhere: %s\nTests: %s",
		task.Number,
		filepath.ToSlash(filepath.Base(activeTasksPathForCommitMessage())),
		joinForCommit(task.Where),
		valueOrPlaceholder(task.Tests),
	)
	if task.RequirementID != "" {
		body += "\nREQ: " + task.RequirementID
	}
	return subject + "\n\n" + body + "\n"
}

func joinForCommit(items []string) string {
	if len(items) == 0 {
		return "<none declared>"
	}
	return strings.Join(items, ", ")
}

func valueOrPlaceholder(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "<see tasks.md>"
	}
	return value
}

func activeTasksPathForCommitMessage() string {
	// Best-effort: format a relative-looking string for the commit
	// body. We do not stat anything here — this is purely cosmetic.
	if _, err := os.Stat(filepath.Join(".specs")); err == nil {
		return ".specs/features/<slug>/tasks.md"
	}
	return ".harness/tasks/<slug>.md"
}

func runGitCommit(message string) error {
	cmd := exec.Command("git", "commit", "-m", message)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
