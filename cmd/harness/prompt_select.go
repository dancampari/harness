package harness

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type promptOption struct {
	Label       string
	Description string
	Value       string
}

type selectPromptModel struct {
	title     string
	options   []promptOption
	cursor    int
	selected  string
	cancelled bool
}

var (
	promptTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230"))
	promptHelpStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	promptItemStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	promptDescStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	promptPickStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("114")).Bold(true)
)

func promptSelect(title string, options []promptOption, defaultIndex int) (string, error) {
	if len(options) == 0 {
		return "", fmt.Errorf("no options available")
	}
	if defaultIndex < 0 || defaultIndex >= len(options) {
		defaultIndex = 0
	}
	model := selectPromptModel{
		title:   title,
		options: options,
		cursor:  defaultIndex,
	}
	program := tea.NewProgram(model)
	result, err := program.Run()
	if err != nil {
		return "", err
	}
	final, ok := result.(selectPromptModel)
	if !ok {
		return "", fmt.Errorf("unexpected prompt result")
	}
	if final.cancelled {
		return "", fmt.Errorf("selection cancelled")
	}
	if final.selected == "" {
		return "", fmt.Errorf("no selection made")
	}
	return final.selected, nil
}

func (m selectPromptModel) Init() tea.Cmd {
	return nil
}

func (m selectPromptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k", "shift+tab":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j", "tab":
			if m.cursor < len(m.options)-1 {
				m.cursor++
			}
		case "home":
			m.cursor = 0
		case "end":
			m.cursor = len(m.options) - 1
		case "enter":
			m.selected = m.options[m.cursor].Value
			return m, tea.Quit
		case "esc", "ctrl+c":
			m.cancelled = true
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m selectPromptModel) View() string {
	var sb strings.Builder
	sb.WriteString(promptTitleStyle.Render(m.title))
	sb.WriteString("\n")
	for i, option := range m.options {
		cursor := "  "
		style := promptItemStyle
		if i == m.cursor {
			cursor = "> "
			style = promptPickStyle
		}
		sb.WriteString(cursor)
		sb.WriteString(style.Render(option.Label))
		if option.Description != "" {
			sb.WriteString(promptDescStyle.Render(" - " + option.Description))
		}
		sb.WriteString("\n")
	}
	sb.WriteString("\n")
	sb.WriteString(promptHelpStyle.Render("Use Up/Down arrows and Enter to select. Esc cancels."))
	sb.WriteString("\n")
	return sb.String()
}
