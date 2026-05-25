package harness

import (
	"strings"
	"testing"

	"github.com/dancampari/harness/internal/planner"
)

func TestFindTaskByRefAcceptsZeroPaddedAndPlain(t *testing.T) {
	plan := &planner.TaskPlan{Tasks: []planner.Task{
		{Number: 3, Title: "wire user-auth"},
		{Number: 12, Title: "wire billing"},
	}}
	if task, ok := findTaskByRef(plan, "003"); !ok || task.Number != 3 {
		t.Fatalf("expected to find task 3 via 003, got ok=%v %+v", ok, task)
	}
	if task, ok := findTaskByRef(plan, "12"); !ok || task.Number != 12 {
		t.Fatalf("expected to find task 12, got ok=%v %+v", ok, task)
	}
	if _, ok := findTaskByRef(plan, "999"); ok {
		t.Fatal("did not expect to find non-existent task")
	}
}

func TestComposeCommitMessageHonoursType(t *testing.T) {
	task := planner.Task{
		Number:        7,
		Title:         "implement billing checkout",
		Where:         []string{"src/billing/checkout.ts", "src/billing/checkout.test.ts"},
		Tests:         "billing checkout happy path",
		RequirementID: "REQ-005",
	}
	msg := composeCommitMessage("fix", task)
	for _, want := range []string{
		"fix: implement billing checkout",
		"Implements task 7",
		"src/billing/checkout.ts",
		"REQ: REQ-005",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("expected commit message to contain %q, got:\n%s", want, msg)
		}
	}
}

func TestComposeCommitMessageDefaultsTypeAndTitle(t *testing.T) {
	task := planner.Task{Number: 1}
	msg := composeCommitMessage("", task)
	if !strings.Contains(msg, "feat: task 1") {
		t.Fatalf("expected default feat type + fallback title, got:\n%s", msg)
	}
}
