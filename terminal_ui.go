package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	approvalBoxStyle = lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(lipgloss.Color("#FFD700")).
		Padding(1, 3)
	approvalTitleStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFD700")).Bold(true)
	approvalCmdStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#87FF87")).Bold(true)
	approvalHelpStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#757575")).
		PaddingTop(1)
)

// TermApprovalMsg is sent when the user approves or denies a command.
type TermApprovalMsg struct {
	Command  string
	Approved bool
	CharID   string
}

type approvalModel struct {
	command string
	charID  string
	cursor  int // 0 = approve, 1 = deny
}

func newApprovalModel(charID, command string) approvalModel {
	return approvalModel{command: command, charID: charID, cursor: 1} // default: deny
}

func (m approvalModel) Init() tea.Cmd { return nil }

func (m approvalModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "left", "h":
			m.cursor = 0
		case "right", "l":
			m.cursor = 1
		case "tab":
			m.cursor = 1 - m.cursor
		case "enter":
			approved := m.cursor == 0
			return m, func() tea.Msg {
				return TermApprovalMsg{Command: m.command, Approved: approved, CharID: m.charID}
			}
		case "esc", "n", "N":
			return m, func() tea.Msg {
				return TermApprovalMsg{Command: m.command, Approved: false, CharID: m.charID}
			}
		case "y", "Y":
			return m, func() tea.Msg {
				return TermApprovalMsg{Command: m.command, Approved: true, CharID: m.charID}
			}
		}
	}
	return m, nil
}

func (m approvalModel) View() string {
	var s strings.Builder
	s.WriteString(approvalTitleStyle.Render("⚡ COMMAND EXECUTION REQUEST") + "\n\n")
	s.WriteString(fmt.Sprintf("Character workspace: %s\n", m.charID))
	s.WriteString(fmt.Sprintf("Command: %s\n\n", approvalCmdStyle.Render(m.command)))

	approveStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#87FF87")).Bold(true)
	denyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5F5F")).Bold(true)

	if m.cursor == 0 {
		approveStyle = approveStyle.Reverse(true)
	} else {
		denyStyle = denyStyle.Reverse(true)
	}

	s.WriteString(approveStyle.Render("  ✅ APPROVE  ") + "   " + denyStyle.Render("  ❌ DENY  "))
	s.WriteString("\n\n" + approvalHelpStyle.Render("[Y/N] or [←/→] then [ENTER] • [ESC] to deny"))
	return approvalBoxStyle.Render(s.String())
}
