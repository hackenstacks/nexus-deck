package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
)

// Create Account styles
var (
	createAccountTitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#7FFF00")). // Chartreuse
				Padding(1, 4).
				Align(lipgloss.Center).
				Bold(true)

	createAccountPromptStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#ADD8E6")) // Light Blue

	createAccountFocusedStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#FFFF00")). // Yellow for focused input
					BorderBottom(true).
					BorderBottomForeground(lipgloss.Color("#FFFF00"))

	createAccountBlurredStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#696969")). // Dim Gray for unfocused input
					BorderBottom(true).
					BorderBottomForeground(lipgloss.Color("#696969"))

	createAccountErrorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FF0000")). // Red for errors
				Bold(true)

	createAccountHelpStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#757575")). // Grey
				PaddingTop(1)

	createAccountBoxStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#90EE90")). // Light Green
				Padding(1, 2).
				Width(40)
)


type createAccountModel struct {
	usernameInput textinput.Model
	passwordInput textinput.Model
	confirmPasswordInput  textinput.Model
	err           error
}

func initialCreateAccountModel() createAccountModel {

	ti := textinput.New()
	ti.Placeholder = "Enter desired username"
	ti.Focus()
	ti.CharLimit = 20
	ti.Width = 20
	ti.PromptStyle = createAccountPromptStyle
	ti.TextStyle = createAccountFocusedStyle // Initially focused
	// ti.Background = lipgloss.Color("#000000") // Black background

	tp := textinput.New()
	tp.Placeholder = "Enter password"
	tp.EchoMode = textinput.EchoPassword
	tp.EchoCharacter = '•'
	tp.CharLimit = 20
	tp.Width = 20
	tp.PromptStyle = createAccountPromptStyle
	tp.TextStyle = createAccountBlurredStyle // Initially blurred
	// tp.Background = lipgloss.Color("#000000") // Black background


	tc := textinput.New()
	tc.Placeholder = "Confirm password"
	tc.EchoMode = textinput.EchoPassword
	tc.EchoCharacter = '•'
	tc.CharLimit = 20
	tc.Width = 20
	tc.PromptStyle = createAccountPromptStyle
	tc.TextStyle = createAccountBlurredStyle // Initially blurred
	// tc.Background = lipgloss.Color("#000000") // Black background


	return createAccountModel{
		usernameInput: ti,
		passwordInput: tp,
		confirmPasswordInput:  tc,
	}
}

func (m createAccountModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m createAccountModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "esc":
			return m, func() tea.Msg { return BackToMainAppMsg{} }
		case "enter":
			if m.usernameInput.Focused() {
				m.usernameInput.Blur()
				m.passwordInput.Focus()
				m.usernameInput.TextStyle = createAccountBlurredStyle
				m.passwordInput.TextStyle = createAccountFocusedStyle
			} else if m.passwordInput.Focused() {
				m.passwordInput.Blur()
				m.confirmPasswordInput.Focus()
				m.passwordInput.TextStyle = createAccountBlurredStyle
				m.confirmPasswordInput.TextStyle = createAccountFocusedStyle
			} else if m.confirmPasswordInput.Focused() {
				m.err = nil // Clear previous errors
				username := m.usernameInput.Value()
				password := m.passwordInput.Value()
				confirm := m.confirmPasswordInput.Value()

				if password != confirm {
					m.err = fmt.Errorf("passwords do not match")
					return m, nil
				}

				if UserExists(username) {
					m.err = fmt.Errorf("username '%s' already exists", username)
					return m, nil
				}

				user := &User{
					Username: username,
				}

				if err := SaveUser(user, password); err != nil {
					m.err = fmt.Errorf("failed to create account: %w", err)
					return m, nil
				}
				// Account created successfully, go back to main menu or login
				return m, func() tea.Msg { return AccountCreatedMsg{} }
			}
		}
	}

	// Update the focused input
	if m.usernameInput.Focused() {
		newInput, newCmd := m.usernameInput.Update(msg)
		m.usernameInput = newInput
		cmds = append(cmds, newCmd)
	} else if m.passwordInput.Focused() {
		newInput, newCmd := m.passwordInput.Update(msg)
		m.passwordInput = newInput
		cmds = append(cmds, newCmd)
	} else if m.confirmPasswordInput.Focused() {
		newInput, newCmd := m.confirmPasswordInput.Update(msg)
		m.confirmPasswordInput = newInput
		cmds = append(cmds, newCmd)
	}

	return m, tea.Batch(cmds...)
}

func (m createAccountModel) View() string {
	var s strings.Builder
	s.WriteString(createAccountTitleStyle.Render("Create a New Account 🚀") + "\n\n")

	s.WriteString(createAccountPromptStyle.Render("Username:") + "\n")
	s.WriteString(m.usernameInput.View() + "\n\n")

	s.WriteString(createAccountPromptStyle.Render("Password:") + "\n")
	s.WriteString(m.passwordInput.View() + "\n\n")

	s.WriteString(createAccountPromptStyle.Render("Confirm Password:") + "\n")
	s.WriteString(m.confirmPasswordInput.View() + "\n\n")

	if m.err != nil {
		s.WriteString(createAccountErrorStyle.Render("⛔ Error: "+m.err.Error()) + "\n")
	}

	s.WriteString(createAccountHelpStyle.Render("tab to cycle, enter to submit, esc to go back"))

	return createAccountBoxStyle.Render(s.String())
}
