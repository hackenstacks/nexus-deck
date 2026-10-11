package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// extractJSON finds the first balanced {...} JSON object in s.
// Handles markdown code fences and prose wrappers from LLM output.
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	for _, fence := range []string{"```json", "```JSON", "```"} {
		if strings.HasPrefix(s, fence) {
			s = strings.TrimPrefix(s, fence)
			if idx := strings.LastIndex(s, "```"); idx >= 0 {
				s = s[:idx]
			}
			s = strings.TrimSpace(s)
			break
		}
	}
	start := strings.Index(s, "{")
	if start == -1 {
		return ""
	}
	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}

type charGenState int

const (
	charGenListView charGenState = iota
	charGenCreateView
	charGenEditView
)

// characterItem represents a single character in the list.
type characterItem struct {
	characterID string
	name        string
	description string
}

func (i characterItem) FilterValue() string { return i.name }
func (i characterItem) Title() string       { return i.name }
func (i characterItem) Description() string  { return i.description }

// LLMCharacterMsg is sent when the LLM successfully generates character data.
type LLMCharacterMsg struct {
	Name string
	Description string
	Personality string
	Scenario string
	FirstMessage string
	Err error
}

// GenerateCharacterCmd is a command that initiates the LLM character generation.
func GenerateCharacterCmd(user *User, apiConfig APIConfig, llmPrompt string) tea.Cmd {
	return func() tea.Msg {
		if user == nil || user.EncryptionKey == nil {
			return LLMCharacterMsg{Err: fmt.Errorf("user not logged in or encryption key not available")}
		}

		llmService, err := LLMFactory(apiConfig.SelectedLLMProvider, apiConfig)
		if err != nil {
			return LLMCharacterMsg{Err: fmt.Errorf("failed to create LLM service: %w", err)}
		}

		// The prompt for the LLM to generate character details
		fullPrompt := fmt.Sprintf(`You are a creative character generator. Generate a character with the following details in JSON format:
{
  "name": "",
  "description": "",
  "personality": "",
  "scenario": "",
  "firstMessage": ""
}
The character should be %s.`, llmPrompt)

		commonConfig, ok := apiConfig.GetLLMProviderConfig(apiConfig.SelectedLLMProvider)
		if !ok {
			return LLMCharacterMsg{Err: fmt.Errorf("could not get config for selected LLM provider: %s", apiConfig.SelectedLLMProvider)}
		}
		model := commonConfig.DefaultLLMModel
		if model == "" {
			switch apiConfig.SelectedLLMProvider {
			case ProviderAIHorde:
				model = "koboldcpp/HF_SPACE_Tiefighter_13B"
			case ProviderPollinations:
				model = "openai"
			case ProviderMistral:
				model = "mistral-tiny"
			case ProviderGroq:
				model = "llama3-8b-8192"
			case ProviderHuggingFace:
				model = "mistralai/Mixtral-8x7B-Instruct-v0.1"
			case ProviderOpenAI:
				model = "gpt-3.5-turbo"
			case ProviderClaude:
				model = "claude-3-haiku-20240307"
			default:
				models, err := llmService.GetAvailableModels(context.Background())
				if err == nil && len(models) > 0 {
					model = models[0]
				} else {
					return LLMCharacterMsg{Err: fmt.Errorf("no default LLM model specified for provider: %s", apiConfig.SelectedLLMProvider)}
				}
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
		defer cancel()

		rawResponse, err := llmService.GenerateResponse(ctx, fullPrompt, model, apiConfig)
		if err != nil {
			return LLMCharacterMsg{Err: fmt.Errorf("LLM generation failed: %w", err)}
		}

		// Extract JSON from the LLM response — handles markdown code fences,
		// prose wrappers, and type mismatches (array vs object for lorebook/tags).
		jsonStr := extractJSON(rawResponse)
		if jsonStr == "" {
			return LLMCharacterMsg{Err: fmt.Errorf("no JSON found in LLM response: %s", rawResponse)}
		}

		// Use a lenient struct so fields like lorebook/tags don't cause unmarshal
		// failures when the LLM returns an array instead of an object.
		var loose struct {
			Name        string      `json:"name"`
			Description string      `json:"description"`
			Personality string      `json:"personality"`
			Scenario    string      `json:"scenario"`
			FirstMessage string     `json:"firstMessage"`
			Lorebook    interface{} `json:"lorebook"`
			Tags        interface{} `json:"tags"`
		}
		if err = json.Unmarshal([]byte(jsonStr), &loose); err != nil {
			return LLMCharacterMsg{Err: fmt.Errorf("failed to parse LLM response: %w\nRaw: %s", err, rawResponse)}
		}
		generatedChar := Character{
			Name:         loose.Name,
			Description:  loose.Description,
			Personality:  loose.Personality,
			Scenario:     loose.Scenario,
			FirstMessage: loose.FirstMessage,
			Lorebook:     make(map[string]string),
		}
		// Safely convert lorebook — LLMs sometimes return [] instead of {}
		if lm, ok := loose.Lorebook.(map[string]interface{}); ok {
			for k, v := range lm {
				if s, ok := v.(string); ok {
					generatedChar.Lorebook[k] = s
				}
			}
		}

		return LLMCharacterMsg{
			Name: generatedChar.Name,
			Description: generatedChar.Description,
			Personality: generatedChar.Personality,
			Scenario: generatedChar.Scenario,
			FirstMessage: generatedChar.FirstMessage,
		}
	}
}

type characterGeneratorModel struct {
	user         *User
	state        charGenState
	characterList list.Model
	selectedChar *Character
	
	// Input fields
	nameInput        textinput.Model
	descriptionInput textarea.Model
	personalityInput textarea.Model
	scenarioInput    textarea.Model
	firstMessageInput textarea.Model
	
	focusedInput int
	spinner      spinner.Model
	generating   bool
	err          error
	saveSuccess  bool
}

func initialCharacterGeneratorModel(u *User) characterGeneratorModel {
	// 1. Setup Character List
	characters, _ := ListCharacters(u)
	items := make([]list.Item, len(characters))
	for i, charInfo := range characters {
		items[i] = characterItem{
			characterID: charInfo.ID,
			name:        fmt.Sprintf("%d. %s", i+1, charInfo.Name),
			description: "Military Grade AI Soul",
		}
	}

	l := list.New(items, list.NewDefaultDelegate(), 76, 20)
	l.Title = " 🎭 THE STUDIO — Character Registry "
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)

	// 2. Setup Inputs
	ni := textinput.New()
	ni.Placeholder = "Character Name"
	
	di := textarea.New()
	di.Placeholder = "Physical and cognitive description..."
	di.SetHeight(3)

	pi := textarea.New()
	pi.Placeholder = "Personality traits and behaviors..."
	pi.SetHeight(3)

	si := textarea.New()
	si.Placeholder = "World context and starting scenario..."
	si.SetHeight(3)

	fm := textarea.New()
	fm.Placeholder = "The first words this character speaks..."
	fm.SetHeight(3)

	// 3. Setup Spinner
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(ActiveTheme.Accent)

	return characterGeneratorModel{
		user:          u,
		state:         charGenListView,
		characterList: l,
		nameInput:     ni,
		descriptionInput: di,
		personalityInput: pi,
		scenarioInput: si,
		firstMessageInput: fm,
		spinner:       s,
		focusedInput: 0,
	}
}

func (m characterGeneratorModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m characterGeneratorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.characterList.SetSize(msg.Width-4, msg.Height-8)
		m.nameInput.Width = msg.Width - 10
		m.descriptionInput.SetWidth(msg.Width - 10)
		m.personalityInput.SetWidth(msg.Width - 10)
		m.scenarioInput.SetWidth(msg.Width - 10)
		m.firstMessageInput.SetWidth(msg.Width - 10)

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+n":
			if m.state == charGenListView {
				m.state = charGenCreateView
				m.resetInputs()
				m.nameInput.Focus()
				return m, nil
			}
		case "ctrl+1", "ctrl+2", "ctrl+3", "ctrl+4", "ctrl+5":
			if m.state != charGenListView {
				m.focusedInput = int(msg.String()[5] - '1')
				m.updateInputFocus()
				return m, nil
			}
		case "ctrl+g":
			if m.state != charGenListView {
				m.generating = true
				apiCfg, _ := LoadAPIConfig(m.user)
				return m, GenerateCharacterCmd(m.user, apiCfg, m.nameInput.Value())
			}
		case "esc":
			if m.state != charGenListView {
				m.state = charGenListView
				return m, nil
			}
		case "enter":
			if m.state == charGenEditView || m.state == charGenCreateView {
				if m.focusedInput < 4 {
					m.focusedInput++
					m.updateInputFocus()
					return m, nil
				}
				// Save Logic
				char := m.selectedChar
				if char == nil { char = NewCharacter() }
				char.Name = m.nameInput.Value()
				char.Description = m.descriptionInput.Value()
				char.Personality = m.personalityInput.Value()
				char.Scenario = m.scenarioInput.Value()
				char.FirstMessage = m.firstMessageInput.Value()
				SaveCharacter(m.user, char)
				m.saveSuccess = true
				m.state = charGenListView
				m.characterList.SetItems(m.loadCharacterItems())
				return m, nil
			}
		}

	case LLMCharacterMsg:
		m.generating = false
		if msg.Err == nil {
			m.nameInput.SetValue(msg.Name)
			m.descriptionInput.SetValue(msg.Description)
			m.personalityInput.SetValue(msg.Personality)
			m.scenarioInput.SetValue(msg.Scenario)
			m.firstMessageInput.SetValue(msg.FirstMessage)
		}
	}

	// Dynamic Theme Sync
	m.characterList.Styles.Title = TitleStyle()
	
	// Sub-component updates
	m.characterList, cmd = m.characterList.Update(msg)
	cmds = append(cmds, cmd)
	m.spinner, cmd = m.spinner.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m characterGeneratorModel) View() string {
	var body string
	
	switch m.state {
	case charGenListView:
		body = m.characterList.View()
	case charGenCreateView, charGenEditView:
		body = m.renderEditor()
	}

	return body
}

func (m characterGeneratorModel) renderEditor() string {
	var sb strings.Builder
	
	title := " ✍️  FORGING NEW CHARACTER "
	if m.state == charGenEditView {
		title = fmt.Sprintf(" 📝 EDITING: %s ", m.nameInput.Value())
	}
	sb.WriteString(TitleStyle().Render(title) + "\n\n")

	inputs := []string{
		"🏷️  ctrl+1. NAME:", m.nameInput.View(),
		"📖 ctrl+2. DESCRIPTION:", m.descriptionInput.View(),
		"🎭 ctrl+3. PERSONALITY:", m.personalityInput.View(),
		"🗺️  ctrl+4. SCENARIO:", m.scenarioInput.View(),
		"💬 ctrl+5. FIRST MESSAGE:", m.firstMessageInput.View(),
	}

	for i := 0; i < len(inputs); i += 2 {
		labelStyle := lipgloss.NewStyle().Foreground(ActiveTheme.Secondary)
		if m.focusedInput == i/2 {
			labelStyle = lipgloss.NewStyle().Foreground(ActiveTheme.Primary).Bold(true)
		}
		sb.WriteString(labelStyle.Render(inputs[i]) + "\n")
		sb.WriteString(inputs[i+1] + "\n\n")
	}

	if m.generating {
		sb.WriteString(m.spinner.View() + SuccessStyle().Render(" AI IS THINKING... ") + "\n")
	}

	return sb.String()
}

func (m *characterGeneratorModel) updateInputFocus() {
	m.nameInput.Blur()
	m.descriptionInput.Blur()
	m.personalityInput.Blur()
	m.scenarioInput.Blur()
	m.firstMessageInput.Blur()

	switch m.focusedInput {
	case 0: m.nameInput.Focus()
	case 1: m.descriptionInput.Focus()
	case 2: m.personalityInput.Focus()
	case 3: m.scenarioInput.Focus()
	case 4: m.firstMessageInput.Focus()
	}
}

func (m *characterGeneratorModel) resetInputs() {
	m.nameInput.SetValue("")
	m.descriptionInput.SetValue("")
	m.personalityInput.SetValue("")
	m.scenarioInput.SetValue("")
	m.firstMessageInput.SetValue("")
	m.selectedChar = nil
	m.focusedInput = 0
}

func (m *characterGeneratorModel) loadCharacterItems() []list.Item {
	characters, _ := ListCharacters(m.user)
	items := make([]list.Item, len(characters))
	for i, charInfo := range characters {
		items[i] = characterItem{
			characterID: charInfo.ID,
			name:        fmt.Sprintf("%d. %s", i+1, charInfo.Name),
			description: "Military Grade AI Soul",
		}
	}
	return items
}
