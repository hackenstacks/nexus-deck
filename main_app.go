package main

import (
	"fmt"
	"log"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	mainTitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00FF00")).
			Bold(true).
			Padding(1, 4).
			BorderStyle(lipgloss.DoubleBorder()).
			BorderForeground(lipgloss.Color("#00FF00"))

	mainItemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			PaddingLeft(2)

	mainSelectedItemStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#00FFFF")).
				Bold(true).
				PaddingLeft(2)
	
	mainStatusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#AAAAAA")).
			Italic(true).
			PaddingTop(1)
)

type mainAppModel struct {
	user        *User
	choices     []string
	cursor      int
	selectedApp AppState
}

func initialMainAppModel(user *User) mainAppModel {
	return mainAppModel{
		user: user,
		choices: []string{
			"1. 🎭 Character Generator",
			"2. 💬 Chat",
			"3. ⚙️  API Settings",
			"4. 📚 Library",
			"V. 🔐 Vault",
			"5. 🚪 Logout",
		},
	}
}

func (m mainAppModel) Init() tea.Cmd {
	return nil
}

func (m mainAppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		log.Printf("mainAppModel received key press: %s", msg.String())
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.choices)-1 {
				m.cursor++
			}
		case "1", "2", "3", "4", "5":
			m.cursor = int(msg.String()[0]-'1')
			return m.handleSelection()
		case "v", "V":
			m.cursor = 4 // Vault is index 4
			return m.handleSelection()
		case "enter":
			return m.handleSelection()
		}
	case BackToMainAppMsg:
		return m, nil
	}
	return m, nil
}

func (m mainAppModel) handleSelection() (tea.Model, tea.Cmd) {
	switch m.cursor {
	case 0:
		return m, func() tea.Msg { return ShowCharacterGeneratorMsg{} }
	case 1:
		return m, func() tea.Msg { return ShowChatMsg{} }
	case 2:
		return m, func() tea.Msg { return ShowApiSettingsMsg{} }
	case 3:
		return m, func() tea.Msg { return ShowLibraryMsg{} }
	case 4:
		return m, func() tea.Msg { return ShowVaultMsg{} }
	case 5:
		return m, func() tea.Msg { return LogoutMsg{} }
	}
	return m, nil
}

func (m mainAppModel) View() string {
	s := mainTitleStyle.Render(fmt.Sprintf("🛡️ NeXuS Advanced Character Generator 🛡️\nUser: %s", m.user.Username))
	s += "\n\n"

	for i, choice := range m.choices {
		if m.cursor == i {
			s += mainSelectedItemStyle.Render(fmt.Sprintf("👉 %s", choice))
		} else {
			s += mainItemStyle.Render(fmt.Sprintf("   %s", choice))
		}
		s += "\n"
	}

	s += mainStatusStyle.Render("\n[↑/↓] Navigate • [1-5/V/Enter] Select • [q] Quit")
	return s
	}