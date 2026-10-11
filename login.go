package main

import (
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	loginTitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#EE82EE")). // Violet
			Padding(1, 4).
			Align(lipgloss.Center).
			Bold(true)

	loginPromptStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#E0BBE4")) // Mauve

	loginFocusedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFD700")). // Gold for focused input
				BorderBottom(true).
				BorderBottomForeground(lipgloss.Color("#FFD700"))

	loginBlurredStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#A9A9A9")). // Dark Gray for unfocused input
				BorderBottom(true).
				BorderBottomForeground(lipgloss.Color("#A9A9A9"))

	loginErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF4500")). // Orange Red for errors
			Bold(true)

	loginHelpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#757575")). // Grey
			PaddingTop(1)

	loginBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#DA70D6")). // Orchid
			Padding(1, 2).
			Width(40)

	loginButtonEnabledStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFFFF")).
				Background(lipgloss.Color("#8A2BE2")). // Blue Violet
				Padding(0, 2).
				Bold(true)

	loginButtonDisabledStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#A9A9A9")).
				Background(lipgloss.Color("#6A5ACD")). // Slate Blue
				Padding(0, 2)
)

type loginModel struct {
	focusIndex int
	inputs     []textinput.Model
	cursorMode cursor.Mode
	spinner    spinner.Model
	loggingIn  bool
	err        error
}

func initialLoginModel() loginModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD700"))

	m := loginModel{
		inputs:     make([]textinput.Model, 2),
		focusIndex: 0,
		spinner:    s,
	}

	var t textinput.Model
	for i := range m.inputs {
		t = textinput.New()
		t.CharLimit = 32
		t.PromptStyle = loginPromptStyle
		t.TextStyle = loginBlurredStyle

		switch i {
		case 0:
			t.Placeholder = "Username"
			t.Focus()
			t.TextStyle = loginFocusedStyle
		case 1:
			t.Placeholder = "Password"
			t.EchoMode = textinput.EchoPassword
			t.EchoCharacter = '•'
		}
		m.inputs[i] = t
	}

	return m
}

func (m loginModel) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, m.spinner.Tick)
}

type LoginResultMsg struct {
	User *User
	Err  error
}

func (m loginModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case LoginResultMsg:
		m.loggingIn = false
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		return m, func() tea.Msg { return LoginSuccessMsg{User: msg.User} }

	case tea.KeyMsg:
		if m.loggingIn {
			return m, nil
		}

		if msg.String() == "ctrl+c" || msg.String() == "q" {
			return m, tea.Quit
		}

		switch msg.String() {
		case "tab", "shift+tab", "enter", "up", "down":
			s := msg.String()

			if s == "enter" && m.focusIndex == len(m.inputs) {
				m.loggingIn = true
				m.err = nil
				username := m.inputs[0].Value()
				password := m.inputs[1].Value()
				return m, func() tea.Msg {
					user, err := LoadUser(username, password)
					return LoginResultMsg{User: user, Err: err}
				}
			}
			if s == "enter" && m.focusIndex == len(m.inputs)+1 {
				return m, func() tea.Msg { return ShowCreateAccountMsg{} }
			}

			if s == "up" || s == "shift+tab" {
				m.focusIndex--
			} else {
				m.focusIndex++
			}

			if m.focusIndex > len(m.inputs)+1 {
				m.focusIndex = 0
			} else if m.focusIndex < 0 {
				m.focusIndex = len(m.inputs)+1
			}

			cmds := make([]tea.Cmd, len(m.inputs))
			for i := 0; i <= len(m.inputs)-1; i++ {
				if i == m.focusIndex {
					cmds[i] = m.inputs[i].Focus()
					m.inputs[i].PromptStyle = loginPromptStyle
					m.inputs[i].TextStyle = loginFocusedStyle
					continue
				}
				m.inputs[i].Blur()
				m.inputs[i].PromptStyle = loginPromptStyle
				m.inputs[i].TextStyle = loginBlurredStyle
			}

			return m, tea.Batch(cmds...)
		default:
			if m.focusIndex < len(m.inputs) {
				var cmds []tea.Cmd
				for i := range m.inputs {
					if i == m.focusIndex {
						var cmd tea.Cmd
						m.inputs[i], cmd = m.inputs[i].Update(msg)
						cmds = append(cmds, cmd)
					} else {
						var cmd tea.Cmd
						m.inputs[i], cmd = m.inputs[i].Update(nil)
						cmds = append(cmds, cmd)
					}
				}
				return m, tea.Batch(cmds...)
			}
		}
	}

	cmd := m.updateInputs(msg)
	return m, cmd
}

func (m *loginModel) updateInputs(msg tea.Msg) tea.Cmd {
	cmds := make([]tea.Cmd, len(m.inputs))
	for i := range m.inputs {
		m.inputs[i], cmds[i] = m.inputs[i].Update(msg)
	}
	return tea.Batch(cmds...)
}

func (m loginModel) View() string {
	var s strings.Builder

	s.WriteString(loginTitleStyle.Render("Welcome to NeXuS Deck! 🛰️") + "\n\n")

	if m.loggingIn {
		s.WriteString("\n\n" + m.spinner.View() + "  Authenticating Neural Links...\n\n")
		return loginBoxStyle.Render(s.String())
	}

	s.WriteString(loginPromptStyle.Render("Username:") + "\n")
	s.WriteString(m.inputs[0].View() + "\n\n")

	s.WriteString(loginPromptStyle.Render("Password:") + "\n")
	s.WriteString(m.inputs[1].View() + "\n\n")

	submitButton := loginButtonDisabledStyle.Render("  Submit  ")
	if m.focusIndex == len(m.inputs) {
		submitButton = loginButtonEnabledStyle.Render("[ Submit ]")
	}
	s.WriteString(submitButton)

	createAccButton := loginButtonDisabledStyle.Render("  Create Account  ")
	if m.focusIndex == len(m.inputs)+1 {
		createAccButton = loginButtonEnabledStyle.Render("[ Create Account ]")
	}
	s.WriteString(lipgloss.NewStyle().PaddingLeft(2).Render(createAccButton))

	if m.err != nil {
		s.WriteString("\n\n")
		s.WriteString(loginErrorStyle.Render("⛔ Error: " + m.err.Error()))
	}

	s.WriteString(loginHelpStyle.Render("\n\nUse Tab/Shift+Tab, Up/Down to navigate, Enter to select, Ctrl+C to quit."))

	return loginBoxStyle.Render(s.String())
}
