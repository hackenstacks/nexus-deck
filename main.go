package main

import (
	"fmt"
	"log"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

// globalProgram holds a reference to the bubbletea program for sending messages from goroutines.
var globalProgram *tea.Program

// AppState defines the high-level states of the application
type AppState int

const (
	LoginState AppState = iota
	CreateAccountState
	DeckState // The unified MNEMOSYNE Deck interface
)

// AppModel is the top-level model that manages different application states
type AppModel struct {
	state              AppState
	loginModel         loginModel
	createAccountModel createAccountModel
	deckModel          DeckModel // The unified cockpit
	user               *User     // Currently logged-in user
	width              int
	height             int
}

func initialAppModel() AppModel {
	// Start with login screen for proper end-to-end flow
	return AppModel{
		state:              LoginState,
		loginModel:         initialLoginModel(),
		createAccountModel: initialCreateAccountModel(),
	}
}

// Init initializes the top-level application model.
func (m AppModel) Init() tea.Cmd {
	return m.loginModel.Init()
}

// Update handles messages and updates the application state.
func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Propagate window size to active sub-model
		switch m.state {
		case LoginState:
			newModel, newCmd := m.loginModel.Update(msg)
			m.loginModel = newModel.(loginModel)
			cmds = append(cmds, newCmd)
		case CreateAccountState:
			newModel, newCmd := m.createAccountModel.Update(msg)
			m.createAccountModel = newModel.(createAccountModel)
			cmds = append(cmds, newCmd)
		case DeckState:
			newModel, newCmd := m.deckModel.Update(msg)
			m.deckModel = newModel.(DeckModel)
			cmds = append(cmds, newCmd)
		}

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			if m.user != nil && m.user.DB != nil {
				m.user.DB.Close()
			}
			return m, tea.Quit
		}
		// Delegate keys to sub-models
		switch m.state {
		case LoginState:
			newModel, newCmd := m.loginModel.Update(msg)
			m.loginModel = newModel.(loginModel)
			cmds = append(cmds, newCmd)
		case CreateAccountState:
			newModel, newCmd := m.createAccountModel.Update(msg)
			m.createAccountModel = newModel.(createAccountModel)
			cmds = append(cmds, newCmd)
		case DeckState:
			newModel, newCmd := m.deckModel.Update(msg)
			m.deckModel = newModel.(DeckModel)
			cmds = append(cmds, newCmd)
		}
		return m, tea.Batch(cmds...)

	case LoginSuccessMsg:
		m.user = msg.User
		ApplyEnvToConfig(m.user)
		m.state = DeckState
		m.deckModel = NewDeckModel(m.user)
		
		// Set initial dimensions for deck
		m.deckModel.width = m.width
		m.deckModel.height = m.height

		// Start API Server if requested
		for _, arg := range os.Args[1:] {
			if arg == "--api" {
				srv := NewAPIServer(m.user)
				go func() {
					if err := srv.Start(); err != nil {
						fmt.Printf("API server error: %v\n", err)
					}
				}()
				break
			}
		}

		return m, m.deckModel.Init()

	case ShowCreateAccountMsg:
		m.state = CreateAccountState
		m.createAccountModel = initialCreateAccountModel()
		return m, m.createAccountModel.Init()

	case AccountCreatedMsg:
		m.state = LoginState
		m.loginModel = initialLoginModel()
		return m, m.loginModel.Init()

	case LogoutMsg:
		if m.user != nil && m.user.DB != nil {
			m.user.DB.Close()
		}
		m.user = nil
		m.state = LoginState
		m.loginModel = initialLoginModel()
		return m, m.loginModel.Init()

	default:
		// Regular delegation
		switch m.state {
		case LoginState:
			newModel, newCmd := m.loginModel.Update(msg)
			m.loginModel = newModel.(loginModel)
			cmds = append(cmds, newCmd)
		case CreateAccountState:
			newModel, newCmd := m.createAccountModel.Update(msg)
			m.createAccountModel = newModel.(createAccountModel)
			cmds = append(cmds, newCmd)
		case DeckState:
			newModel, newCmd := m.deckModel.Update(msg)
			m.deckModel = newModel.(DeckModel)
			cmds = append(cmds, newCmd)
		}
	}
	return m, tea.Batch(cmds...)
}

// View renders the currently active application view.
func (m AppModel) View() string {
	switch m.state {
	case LoginState:
		return m.loginModel.View()
	case CreateAccountState:
		return m.createAccountModel.View()
	case DeckState:
		return m.deckModel.View()
	default:
		return "SYSTEM ERROR: Unknown State"
	}
}

func main() {
	if _, err := os.Stat("storage"); os.IsNotExist(err) {
		os.Mkdir("storage", 0755)
	}

	InitNexusLogger()
	defer CloseNexusLogger()

	f, err := tea.LogToFile("debug.log", "debug")
	if err != nil {
		fmt.Println("fatal:", err)
		os.Exit(1)
	}
	defer f.Close()
	log.SetOutput(f)

	p := tea.NewProgram(initialAppModel(),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	globalProgram = p

	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}
