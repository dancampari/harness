package harness

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestSelectPromptUsesArrowNavigationAndEnter(t *testing.T) {
	model := selectPromptModel{
		title: "Pick",
		options: []promptOption{
			{Label: "Claude Code", Value: "claude"},
			{Label: "Codex", Value: "codex"},
			{Label: "Cursor IDE", Value: "cursor"},
		},
		cursor: 0,
	}

	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model = next.(selectPromptModel)
	if model.cursor != 1 {
		t.Fatalf("expected cursor at 1, got %d", model.cursor)
	}

	next, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	model = next.(selectPromptModel)
	if model.selected != "codex" {
		t.Fatalf("expected codex selection, got %q", model.selected)
	}
	if cmd == nil {
		t.Fatal("expected enter to quit prompt")
	}
}

func TestSelectPromptEscCancels(t *testing.T) {
	model := selectPromptModel{
		title:   "Pick",
		options: []promptOption{{Label: "Project only", Value: "project"}},
	}
	next, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEsc})
	model = next.(selectPromptModel)
	if !model.cancelled {
		t.Fatal("expected prompt to be cancelled")
	}
	if cmd == nil {
		t.Fatal("expected esc to quit prompt")
	}
}
