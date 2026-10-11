package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var selectableProviders = []Provider{
	ProviderPollinations, ProviderGemini, ProviderOpenAI, ProviderClaude,
	ProviderMistral, ProviderGroq, ProviderOllama, ProviderOpenRouter,
}

// apiSettingsModel manages the API configuration UI.
type apiSettingsModel struct {
	user        *User
	apiConfig   APIConfig
	currentView apiSettingsView
	providerList list.Model
	apiKeyInput  textinput.Model
	baseURLInput textinput.Model
	customNameInput textinput.Model
	customTypeInput textinput.Model // "llm" or "image"
	customModelInput textinput.Model
	selectedProvider Provider
	err          error
	saveSuccess  bool

	// Model selection
	llmModelsList list.Model
	imageModelsList list.Model
	currentModelListType string // "llm" or "image"
	modelSelectionOrigin Provider

	// Selection Focus for sub-views
	focusedRow int // 0: Presets, 1: Aspect, 2: Negative, 3: Vision Prov, 4: RAG Prov
	
	visionProviderIndex int
	ragProviderIndex    int
}

type apiSettingsView int

const (
	providerListView apiSettingsView = iota
	providerDetailsView
	customProviderDetailsView
	modelSelectionView
	imageStyleSettingsView
	customModelInputView
)

type providerItem struct {
	provider Provider
	title    string
	desc     string
}

func (i providerItem) FilterValue() string { return i.title }
func (i providerItem) Title() string       { return i.title }
func (i providerItem) Description() string  { return i.desc }

func initialApiSettingsModel(u *User) apiSettingsModel {
	apiConfig, _ := LoadAPIConfig(u)

	providers := []list.Item{
		providerItem{provider: ProviderOllama, title: "1. Ollama 🤖", desc: "Local AI Sovereignty"},
		providerItem{provider: ProviderAIHorde, title: "2. AIHorde 🌐", desc: "Community Models (Free)"},
		providerItem{provider: ProviderPollinations, title: "3. Pollinations.ai 🌳", desc: "Creative Models (Free)"},
		providerItem{provider: ProviderGemini, title: "4. Google Gemini ✨", desc: "Gemini 1.5 Flash/Pro"},
		providerItem{provider: ProviderMistral, title: "5. Mistral AI 💨", desc: "High-performance French AI"},
		providerItem{provider: ProviderGroq, title: "6. Groq ⚡", desc: "Ultra-fast LPU inference"},
		providerItem{provider: ProviderOpenAI, title: "7. OpenAI 🤖", desc: "GPT-4o, DALL-E 3"},
		providerItem{provider: ProviderClaude, title: "8. Anthropic Claude 📜", desc: "Claude 3.5 Sonnet"},
		providerItem{provider: ProviderCustomLLM, title: "0. Custom API 🔧", desc: "Your own sovereign endpoint"},
		providerItem{provider: ProviderSecurity, title: "🎨 Image Style 🎨", desc: "Global Visual Presets"},
	}

	delegate := list.NewDefaultDelegate()
	l := list.New(providers, delegate, 76, 20)
	l.Title = " ⚙️  API CONFIGURATION "
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)

	ak := textinput.New()
	ak.Placeholder = "Paste API Key Here"
	ak.Width = 60

	bu := textinput.New()
	bu.Placeholder = "https://api.endpoint.com/v1"
	bu.Width = 60

	cn := textinput.New()
	cn.Placeholder = "Custom API Name"
	cn.Width = 40

	ct := textinput.New()
	ct.Placeholder = "llm or image"
	ct.Width = 10

	cm := textinput.New()
	cm.Placeholder = "Enter custom model name..."
	cm.Width = 40

	m := apiSettingsModel{
		user:         u,
		apiConfig:    apiConfig,
		currentView:  providerListView,
		providerList: l,
		apiKeyInput:  ak,
		baseURLInput: bu,
		customNameInput: cn,
		customTypeInput: ct,
		customModelInput: cm,
		llmModelsList: list.New([]list.Item{}, list.NewDefaultDelegate(), 76, 15),
		imageModelsList: list.New([]list.Item{}, list.NewDefaultDelegate(), 76, 15),
	}

	// Sync indexes with loaded config
	for i, p := range selectableProviders {
		if p == apiConfig.SelectedVisionLLMProvider { m.visionProviderIndex = i }
		if p == apiConfig.SelectedRAGLLMProvider { m.ragProviderIndex = i }
	}

	return m
}

func (m apiSettingsModel) Init() tea.Cmd { return nil }

func (m apiSettingsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	// Dynamic Style Sync
	m.providerList.Styles.Title = TitleStyle()
	m.llmModelsList.Styles.Title = TitleStyle()
	m.imageModelsList.Styles.Title = TitleStyle()

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Check if any text input is focused
		anyInputFocused := m.apiKeyInput.Focused() || m.baseURLInput.Focused() || 
			m.customNameInput.Focused() || m.customTypeInput.Focused() || 
			m.customModelInput.Focused()

		// When any text field is focused, pass all keys directly to the input — 
		// except global control keys like ctrl+c, esc, enter, tab.
		if anyInputFocused {
			switch msg.String() {
			case "ctrl+c", "ctrl+q":
				return m, tea.Quit
			case "esc":
				m.apiKeyInput.Blur()
				m.baseURLInput.Blur()
				m.customNameInput.Blur()
				m.customTypeInput.Blur()
				m.customModelInput.Blur()
				m.focusedRow = 0
				return m, nil
			case "tab":
				// Handle internal navigation between fields
				if m.currentView == providerDetailsView {
					if m.apiKeyInput.Focused() {
						m.apiKeyInput.Blur()
						m.baseURLInput.Focus()
					} else {
						m.baseURLInput.Blur()
						m.apiKeyInput.Focus()
					}
				} else if m.currentView == imageStyleSettingsView {
					m.apiKeyInput.Blur()
					m.focusedRow = (m.focusedRow + 1) % 5
					if m.focusedRow == 2 { m.apiKeyInput.Focus() }
				} else if m.currentView == customProviderDetailsView {
					if m.customNameInput.Focused() {
						m.customNameInput.Blur(); m.customTypeInput.Focus()
					} else if m.customTypeInput.Focused() {
						m.customTypeInput.Blur(); m.baseURLInput.Focus()
					} else if m.baseURLInput.Focused() {
						m.baseURLInput.Blur(); m.apiKeyInput.Focus()
					} else {
						m.apiKeyInput.Blur(); m.customNameInput.Focus()
					}
				}
				return m, nil
			case "enter":
				// Allow Enter to save/apply even while focused
				switch m.currentView {
				case providerDetailsView, customProviderDetailsView:
					return m, m.saveCurrentProvider()
				case customModelInputView:
					modelName := m.customModelInput.Value()
					if modelName != "" {
						cmd := m.applyModelSelection(modelName)
						m.currentView = providerDetailsView
						m.customModelInput.Blur()
						return m, cmd
					}
					m.currentView = modelSelectionView
					m.customModelInput.Blur()
					return m, nil
				}
			default:
				// Pass-through to focused input
				var inputCmd tea.Cmd
				if m.apiKeyInput.Focused() {
					m.apiKeyInput, inputCmd = m.apiKeyInput.Update(msg)
					if m.currentView == imageStyleSettingsView { m.apiConfig.ImageStyle.NegativePrompt = m.apiKeyInput.Value() }
				} else if m.baseURLInput.Focused() {
					m.baseURLInput, inputCmd = m.baseURLInput.Update(msg)
				} else if m.customNameInput.Focused() {
					m.customNameInput, inputCmd = m.customNameInput.Update(msg)
				} else if m.customTypeInput.Focused() {
					m.customTypeInput, inputCmd = m.customTypeInput.Update(msg)
				} else if m.customModelInput.Focused() {
					m.customModelInput, inputCmd = m.customModelInput.Update(msg)
				}
				return m, inputCmd
			}
		}

		switch msg.String() {
		case "ctrl+c", "ctrl+q":
			return m, tea.Quit
		case "esc":
			m.err = nil
			m.saveSuccess = false
			if m.currentView == providerListView {
				return m, func() tea.Msg { return BackToMainAppMsg{} }
			}
			if m.currentView == customModelInputView {
				m.currentView = modelSelectionView
				return m, nil
			}
			m.currentView = providerListView
			return m, nil

		case "enter":
			if m.currentView == providerListView {
				if m.providerList.SelectedItem() == nil {
					return m, nil
				}
				m.handleProviderSelection()
				return m, nil
			} else if m.currentView == modelSelectionView {
				return m, m.saveSelectedModel()
			}
		
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			if m.currentView == imageStyleSettingsView {
				m.handleImageStyleNumeric(msg.String())
				return m, nil
			}

		case "q", "w", "e", "r", "t":
			if m.currentView == imageStyleSettingsView {
				m.handleAspectRatioKey(msg.String())
				return m, nil
			}

		case "left", "right", "h", "l":
			if m.currentView == imageStyleSettingsView {
				if m.focusedRow == 3 { // Vision Provider
					m.cycleVisionProvider(msg.String() == "right" || msg.String() == "l")
					return m, nil
				} else if m.focusedRow == 4 { // RAG Provider
					m.cycleRAGProvider(msg.String() == "right" || msg.String() == "l")
					return m, nil
				}
			}

		case "up", "down", "k", "j":
			if m.currentView == imageStyleSettingsView {
				if msg.String() == "up" || msg.String() == "k" {
					m.focusedRow = (m.focusedRow - 1 + 5) % 5
				} else {
					m.focusedRow = (m.focusedRow + 1) % 5
				}
				if m.focusedRow == 2 {
					m.apiKeyInput.Focus()
				} else {
					m.apiKeyInput.Blur()
				}
				return m, nil
			}

		case "s", "ctrl+s":
			if m.currentView == imageStyleSettingsView {
				err := SaveAPIConfig(m.user, m.apiConfig)
				if err != nil {
					m.err = err
					return m, nil
				}
				m.saveSuccess = true
				m.currentView = providerListView
				return m, nil
			}
		
		case "m":
			if m.currentView == providerDetailsView {
				m.currentModelListType = "llm"
				m.modelSelectionOrigin = m.selectedProvider
				m.populateModels()
				m.currentView = modelSelectionView
				return m, nil
			}

		case "i":
			if m.currentView == providerDetailsView && providerSupportsImages(m.selectedProvider) {
				m.currentModelListType = "image"
				m.modelSelectionOrigin = m.selectedProvider
				m.populateModels()
				m.currentView = modelSelectionView
				return m, nil
			}
		
		case "tab":
			if m.currentView == providerDetailsView {
				m.apiKeyInput.Focus()
				return m, nil
			}
			if m.currentView == imageStyleSettingsView {
				m.focusedRow = (m.focusedRow + 1) % 5
				if m.focusedRow == 2 { m.apiKeyInput.Focus() }
				return m, nil
			}
			if m.currentView == customProviderDetailsView {
				m.customNameInput.Focus()
				return m, nil
			}
		}
	}

	// Delegation
	if m.currentView == providerListView {
		m.providerList, cmd = m.providerList.Update(msg)
		cmds = append(cmds, cmd)
	} else if m.currentView == modelSelectionView {
		if m.currentModelListType == "llm" {
			m.llmModelsList, cmd = m.llmModelsList.Update(msg)
		} else {
			m.imageModelsList, cmd = m.imageModelsList.Update(msg)
		}
		cmds = append(cmds, cmd)
	} else if m.currentView == customModelInputView {
		m.customModelInput, cmd = m.customModelInput.Update(msg)
		cmds = append(cmds, cmd)
	} else {
		m.apiKeyInput, cmd = m.apiKeyInput.Update(msg)
		cmds = append(cmds, cmd)
		m.baseURLInput, cmd = m.baseURLInput.Update(msg)
		cmds = append(cmds, cmd)
		m.customNameInput, cmd = m.customNameInput.Update(msg)
		cmds = append(cmds, cmd)
		m.customTypeInput, cmd = m.customTypeInput.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m *apiSettingsModel) handleProviderSelection() {
	item := m.providerList.SelectedItem()
	if item == nil { return }
	selected := item.(providerItem)
	m.selectedProvider = selected.provider
	
	if m.selectedProvider == ProviderSecurity {
		m.currentView = imageStyleSettingsView
		m.apiKeyInput.SetValue(m.apiConfig.ImageStyle.NegativePrompt)
		m.focusedRow = 0
		m.apiKeyInput.Blur()
		return
	}

	config, _ := m.apiConfig.GetLLMProviderConfig(m.selectedProvider)
	m.apiKeyInput.SetValue(config.APIKey)
	m.baseURLInput.SetValue(config.BaseURL)
	
	if m.selectedProvider == ProviderCustomLLM || m.selectedProvider == ProviderCustomImage {
		m.currentView = customProviderDetailsView
		m.customNameInput.Focus()
	} else {
		m.currentView = providerDetailsView
		m.apiKeyInput.Focus()
	}
}

func (m *apiSettingsModel) cycleVisionProvider(next bool) {
	if next {
		m.visionProviderIndex = (m.visionProviderIndex + 1) % len(selectableProviders)
	} else {
		m.visionProviderIndex = (m.visionProviderIndex - 1 + len(selectableProviders)) % len(selectableProviders)
	}
	m.apiConfig.SelectedVisionLLMProvider = selectableProviders[m.visionProviderIndex]
}

func (m *apiSettingsModel) cycleRAGProvider(next bool) {
	if next {
		m.ragProviderIndex = (m.ragProviderIndex + 1) % len(selectableProviders)
	} else {
		m.ragProviderIndex = (m.ragProviderIndex - 1 + len(selectableProviders)) % len(selectableProviders)
	}
	m.apiConfig.SelectedRAGLLMProvider = selectableProviders[m.ragProviderIndex]
}

// ProviderChangedMsg is sent to the Deck when the user saves a new LLM provider.
// The Deck handles this by reinitialising the chatModel's llmService.
type ProviderChangedMsg struct {
	Provider  Provider
	APIConfig APIConfig
}

func (m *apiSettingsModel) saveCurrentProvider() tea.Cmd {
	m.saveSuccess = false
	key := m.apiKeyInput.Value()
	url := m.baseURLInput.Value()

	// Image-only providers do not change the LLM selection
	imageOnly := m.selectedProvider == ProviderAIHorde ||
		m.selectedProvider == ProviderPollinations ||
		m.selectedProvider == ProviderImageRouter

	switch m.selectedProvider {
	case ProviderGemini:
		m.apiConfig.Gemini.APIKey = key
		if url != "" { m.apiConfig.Gemini.BaseURL = url }
		m.apiConfig.Gemini.Enabled = key != ""
		if m.apiConfig.Gemini.DefaultLLMModel == "" { m.apiConfig.Gemini.DefaultLLMModel = "gemini-2.0-flash" }
	case ProviderOpenAI:
		m.apiConfig.OpenAI.APIKey = key
		m.apiConfig.OpenAI.BaseURL = url
		m.apiConfig.OpenAI.Enabled = key != ""
		if m.apiConfig.OpenAI.DefaultLLMModel == "" { m.apiConfig.OpenAI.DefaultLLMModel = "gpt-4o" }
	case ProviderClaude:
		m.apiConfig.Claude.APIKey = key
		m.apiConfig.Claude.Enabled = key != ""
		if m.apiConfig.Claude.DefaultLLMModel == "" { m.apiConfig.Claude.DefaultLLMModel = "claude-3-5-sonnet-latest" }
	case ProviderMistral:
		m.apiConfig.Mistral.APIKey = key
		if url != "" { m.apiConfig.Mistral.BaseURL = url }
		m.apiConfig.Mistral.Enabled = key != ""
		if m.apiConfig.Mistral.DefaultLLMModel == "" { m.apiConfig.Mistral.DefaultLLMModel = "mistral-large-latest" }
	case ProviderGroq:
		m.apiConfig.Groq.APIKey = key
		m.apiConfig.Groq.Enabled = key != ""
		if m.apiConfig.Groq.DefaultLLMModel == "" { m.apiConfig.Groq.DefaultLLMModel = "llama3-70b-8192" }
	case ProviderOpenRouter:
		m.apiConfig.OpenRouter.APIKey = key
		m.apiConfig.OpenRouter.Enabled = key != ""
	case ProviderOllama:
		m.apiConfig.Ollama.BaseURL = url
		m.apiConfig.Ollama.Enabled = true
		if m.apiConfig.Ollama.DefaultLLMModel == "" { m.apiConfig.Ollama.DefaultLLMModel = "llama3" }
	case ProviderPollinations:
		m.apiConfig.Pollinations.APIKey = key
		m.apiConfig.Pollinations.Enabled = true
		if m.apiConfig.Pollinations.DefaultLLMModel == "" { m.apiConfig.Pollinations.DefaultLLMModel = "openai" }
		if m.apiConfig.Pollinations.DefaultImageModel == "" { m.apiConfig.Pollinations.DefaultImageModel = "flux" }
	case ProviderAIHorde:
		m.apiConfig.AIHorde.APIKey = key
		m.apiConfig.AIHorde.Enabled = true
		if m.apiConfig.AIHorde.DefaultLLMModel == "" { m.apiConfig.AIHorde.DefaultLLMModel = "koboldcpp/HF_SPACE_Tiefighter_13B" }
	}

	// Switch active LLM provider to the one just configured (unless image-only)
	if !imageOnly {
		m.apiConfig.SelectedLLMProvider = m.selectedProvider
	}

	err := SaveAPIConfig(m.user, m.apiConfig)
	if err != nil {
		m.err = err
		return nil
	}
	m.saveSuccess = true
	m.currentView = providerListView

	if !imageOnly {
		cfg := m.apiConfig
		return func() tea.Msg { return ProviderChangedMsg{Provider: m.selectedProvider, APIConfig: cfg} }
	}
	return nil
}

// ModelChangedMsg is sent to the Deck when the user picks a new model in Settings.
type ModelChangedMsg struct {
	Provider Provider
	Model    LLMModel
	APIConfig APIConfig
}

func (m *apiSettingsModel) saveSelectedModel() tea.Cmd {
	var modelName string
	var item list.Item
	
	if m.currentModelListType == "llm" {
		item = m.llmModelsList.SelectedItem()
	} else {
		item = m.imageModelsList.SelectedItem()
	}

	if item == nil { return nil }
	modelName = item.(providerItem).title

	if modelName == "✏️  Enter custom model..." {
		m.customModelInput.SetValue("")
		m.customModelInput.Focus()
		m.currentView = customModelInputView
		return nil
	}

	cmd := m.applyModelSelection(modelName)
	m.currentView = providerDetailsView
	return cmd
}

func (m *apiSettingsModel) applyModelSelection(modelName string) tea.Cmd {
	if m.currentModelListType == "llm" {
		switch m.modelSelectionOrigin {
		case ProviderGemini:     m.apiConfig.Gemini.DefaultLLMModel     = LLMModel(modelName)
		case ProviderOpenAI:     m.apiConfig.OpenAI.DefaultLLMModel     = LLMModel(modelName)
		case ProviderMistral:    m.apiConfig.Mistral.DefaultLLMModel    = LLMModel(modelName)
		case ProviderGroq:       m.apiConfig.Groq.DefaultLLMModel       = LLMModel(modelName)
		case ProviderClaude:     m.apiConfig.Claude.DefaultLLMModel     = LLMModel(modelName)
		case ProviderOllama:     m.apiConfig.Ollama.DefaultLLMModel     = LLMModel(modelName)
		case ProviderOpenRouter: m.apiConfig.OpenRouter.DefaultLLMModel = LLMModel(modelName)
		case ProviderHuggingFace: m.apiConfig.HuggingFace.DefaultLLMModel = LLMModel(modelName)
		case ProviderAIHorde:    m.apiConfig.AIHorde.DefaultLLMModel   = LLMModel(modelName)
		case ProviderPollinations: m.apiConfig.Pollinations.DefaultLLMModel = LLMModel(modelName)
		}
	} else {
		switch m.modelSelectionOrigin {
		case ProviderOpenAI:     m.apiConfig.OpenAI.DefaultImageModel     = ImageModel(modelName)
		case ProviderAIHorde:    m.apiConfig.AIHorde.DefaultImageModel    = ImageModel(modelName)
		case ProviderPollinations: m.apiConfig.Pollinations.DefaultImageModel = ImageModel(modelName)
		}
	}

	SaveAPIConfig(m.user, m.apiConfig)
	m.saveSuccess = true

	// Notify deck so the live chat session uses the new model immediately
	if m.currentModelListType == "llm" {
		cfg := m.apiConfig
		return func() tea.Msg {
			return ModelChangedMsg{Provider: m.modelSelectionOrigin, Model: LLMModel(modelName), APIConfig: cfg}
		}
	}
	return nil
}

func (m *apiSettingsModel) populateModels() {
	var items []list.Item

	if m.currentModelListType == "llm" {
		switch m.modelSelectionOrigin {
		case ProviderGemini:
			items = []list.Item{
				providerItem{title: "gemini-2.0-flash",     desc: "Fast — recommended"},
				providerItem{title: "gemini-1.5-pro",       desc: "Complex reasoning"},
				providerItem{title: "gemini-1.5-flash",     desc: "Stable fast"},
				providerItem{title: "gemini-2.5-pro-preview-03-25", desc: "Latest Pro"},
			}
		case ProviderMistral:
			items = []list.Item{
				providerItem{title: "mistral-large-latest",  desc: "Flagship"},
				providerItem{title: "mistral-small-latest",  desc: "Fast"},
				providerItem{title: "codestral-latest",      desc: "Code specialist"},
				providerItem{title: "open-mistral-7b",       desc: "Open — cheapest"},
			}
		case ProviderGroq:
			items = []list.Item{
				providerItem{title: "llama3-70b-8192",      desc: "Llama 3 70B — best"},
				providerItem{title: "llama3-8b-8192",       desc: "Llama 3 8B — fast"},
				providerItem{title: "mixtral-8x7b-32768",   desc: "Mixtral 8x7B"},
				providerItem{title: "gemma2-9b-it",         desc: "Google Gemma 2"},
			}
		case ProviderOpenAI:
			items = []list.Item{
				providerItem{title: "gpt-4o",       desc: "Flagship multimodal"},
				providerItem{title: "gpt-4o-mini",  desc: "Fast + cheap"},
				providerItem{title: "gpt-4-turbo",  desc: "Long context"},
				providerItem{title: "gpt-3.5-turbo", desc: "Cheap"},
			}
		case ProviderClaude:
			items = []list.Item{
				providerItem{title: "claude-opus-4-6",   desc: "Most capable"},
				providerItem{title: "claude-sonnet-4-6", desc: "Balanced — recommended"},
				providerItem{title: "claude-haiku-4-5-20251001",  desc: "Fast + cheap"},
			}
		case ProviderOllama:
			items = []list.Item{
				providerItem{title: "llama3",            desc: "General"},
				providerItem{title: "mistral",             desc: "Mistral local"},
				providerItem{title: "phi3",                desc: "Microsoft Phi3"},
				providerItem{title: "qwen2.5-coder:1.5b", desc: "Code — small"},
				providerItem{title: "llava",               desc: "Vision-capable"},
				providerItem{title: "moondream",           desc: "Vision-capable"},
			}
		case ProviderOpenRouter:
			items = []list.Item{
				providerItem{title: "mistralai/mistral-7b-instruct:free",       desc: "Free"},
				providerItem{title: "meta-llama/llama-3-8b-instruct:free",    desc: "Free"},
				providerItem{title: "google/gemma-3-27b-it:free",         desc: "Free"},
			}
		case ProviderHuggingFace:
			items = []list.Item{
				providerItem{title: "mistralai/Mistral-7B-Instruct-v0.2", desc: "Mistral 7B"},
				providerItem{title: "meta-llama/Meta-Llama-3-8B-Instruct", desc: "Llama 3 8B"},
				providerItem{title: "HuggingFaceH4/zephyr-7b-beta",        desc: "Zephyr 7B"},
			}
		case ProviderAIHorde:
			items = []list.Item{
				providerItem{title: "koboldcpp/HF_SPACE_Tiefighter-13B", desc: "Popular"},
				providerItem{title: "mistralai/Mistral-7B-Instruct-v0.2", desc: "Mistral"},
				providerItem{title: "PygmalionAI/mythalion-13b",          desc: "RP focused"},
			}
		case ProviderPollinations:
			items = []list.Item{
				providerItem{title: "openai",       desc: "GPT-5 Mini — Fast & Balanced"},
				providerItem{title: "openai-fast",  desc: "GPT-5 Nano — Ultra Fast"},
				providerItem{title: "openai-large", desc: "GPT-5.2 Elite — Intelligence"},
				providerItem{title: "qwen-coder",   desc: "Qwen3 Coder — specialized"},
				providerItem{title: "mistral",      desc: "Mistral Small — Efficient"},
				providerItem{title: "deepseek",     desc: "DeepSeek V3.2 — Agentic"},
				providerItem{title: "gemini",       desc: "Gemini 3 Flash — Fast"},
				providerItem{title: "claude",       desc: "Claude 3.5 Sonnet — Premium"},
			}
		}
		items = append(items, providerItem{title: "✏️  Enter custom model...", desc: "Type any model name"})
		m.llmModelsList.SetItems(items)
	} else {
		// Image models
		switch m.modelSelectionOrigin {
		case ProviderPollinations:
			items = []list.Item{
				providerItem{title: "flux",           desc: "Flux Schnell — Default"},
				providerItem{title: "zimage",         desc: "Z-Image Turbo — Upscaled"},
				providerItem{title: "gptimage",       desc: "GPT Image 1 Mini — OpenAI style"},
				providerItem{title: "wan-image",      desc: "Wan 2.7 Image — Creative"},
				providerItem{title: "klein",          desc: "Flux.2 Klein 4B — Fast"},
				providerItem{title: "flux-pro",       desc: "Flux Pro Quality — Best"},
			}
		case ProviderOpenAI:
			items = []list.Item{
				providerItem{title: "dall-e-3", desc: "Highest quality"},
				providerItem{title: "dall-e-2", desc: "Faster"},
			}
		case ProviderAIHorde:
			items = []list.Item{
				providerItem{title: "stable_diffusion", desc: "SD 1.5"},
				providerItem{title: "Dreamshaper",       desc: "Popular"},
				providerItem{title: "Deliberate",        desc: "Realistic"},
				providerItem{title: "AnimagineXL",       desc: "Anime XL"},
			}
		}
		items = append(items, providerItem{title: "✏️  Enter custom model...", desc: "Type any model name"})
		m.imageModelsList.SetItems(items)
	}
}

func (m apiSettingsModel) View() string {
	var body string
	var status string

	if m.err != nil {
		status = ErrorStyle().Render(fmt.Sprintf(" ⚠️ ERROR: %v", m.err)) + "\n"
	} else if m.saveSuccess {
		status = SuccessStyle().Render(" ✅ Settings saved.") + "\n"
	}

	switch m.currentView {
	case providerListView:
		body = m.providerList.View()
	case providerDetailsView:
		body = m.renderDetails()
	case customProviderDetailsView:
		body = m.renderCustomDetails()
	case modelSelectionView:
		if m.currentModelListType == "llm" {
			body = m.llmModelsList.View()
		} else {
			body = m.imageModelsList.View()
		}
	case imageStyleSettingsView:
		body = m.renderImageStyle()
	case customModelInputView:
		body = m.renderCustomModelInput()
	}

	return BorderedBox(true).Render(lipgloss.JoinVertical(lipgloss.Left, body, status))
}

func (m apiSettingsModel) renderDetails() string {
	var sb strings.Builder
	sb.WriteString(TitleStyle().Render(fmt.Sprintf(" 📡 %s SETTINGS ", m.selectedProvider)) + "\n\n")
	
	sb.WriteString(lipgloss.NewStyle().Foreground(ActiveTheme.Secondary).Render(" 🔑 API KEY:") + "\n")
	sb.WriteString(m.apiKeyInput.View() + "\n\n")
	
	sb.WriteString(lipgloss.NewStyle().Foreground(ActiveTheme.Secondary).Render(" 🌐 ENDPOINT:") + "\n")
	sb.WriteString(m.baseURLInput.View() + "\n\n")

	// Show current models generically
	config, _ := m.apiConfig.GetLLMProviderConfig(m.selectedProvider)
	
	llmModel := string(config.DefaultLLMModel)
	if llmModel == "" { llmModel = "[Not set]" }
	sb.WriteString(fmt.Sprintf(" 🤖 LLM Model:   %s   [m] to change\n", lipgloss.NewStyle().Foreground(ActiveTheme.Primary).Render(llmModel)))

	if providerSupportsImages(m.selectedProvider) {
		imgModel := string(config.DefaultImageModel)
		if imgModel == "" { imgModel = "[Not set]" }
		sb.WriteString(fmt.Sprintf(" 🖼️ Image Model: %s   [i] to change\n", lipgloss.NewStyle().Foreground(ActiveTheme.Primary).Render(imgModel)))
	}

	sb.WriteString("\n" + MutedStyle().Render(" [m] Model | [i] Image | [ENTER] Save | [ESC] Back"))
	return sb.String()
}

func (m apiSettingsModel) renderCustomDetails() string {
	var sb strings.Builder
	sb.WriteString(TitleStyle().Render(" 🔧 CUSTOM API SOVEREIGNTY ") + "\n\n")
	
	sb.WriteString(lipgloss.NewStyle().Foreground(ActiveTheme.Secondary).Render(" 🏷️ NAME:") + "\n")
	sb.WriteString(m.customNameInput.View() + "\n\n")
	
	sb.WriteString(lipgloss.NewStyle().Foreground(ActiveTheme.Secondary).Render(" 🏗️ TYPE (llm/image):") + "\n")
	sb.WriteString(m.customTypeInput.View() + "\n\n")

	sb.WriteString(lipgloss.NewStyle().Foreground(ActiveTheme.Secondary).Render(" 🌐 ENDPOINT URL:") + "\n")
	sb.WriteString(m.baseURLInput.View() + "\n\n")

	sb.WriteString(MutedStyle().Render(" [ENTER] Save | [ESC] Back"))
	return sb.String()
}

func (m apiSettingsModel) renderImageStyle() string {
	var sb strings.Builder
	sb.WriteString(TitleStyle().Render(" 🎨 GLOBAL IMAGE STYLE 🎨 ") + "\n\n")

	// STYLE PRESETS
	rowStyle := lipgloss.NewStyle()
	if m.focusedRow == 0 { rowStyle = rowStyle.Background(ActiveTheme.Background).Foreground(ActiveTheme.Primary).Bold(true) }
	
	sb.WriteString(lipgloss.NewStyle().Foreground(ActiveTheme.Secondary).Render(" 🎌 STYLE PRESETS (1-8):") + "\n")
	styles := []string{"Anime", "Digital Art", "Photorealistic", "Oil Painting", "Watercolor", "Cyberpunk", "Fantasy", "Sketch"}
	for i, s := range styles {
		indicator := "  "
		if m.apiConfig.ImageStyle.StylePreset == strings.ToLower(s) { indicator = " ✨" }
		sb.WriteString(fmt.Sprintf(" %d. %s%s  ", i+1, s, indicator))
		if (i+1)%4 == 0 { sb.WriteString("\n") }
	}
	sb.WriteString("\n")

	// ASPECT RATIO
	rowStyle = lipgloss.NewStyle()
	if m.focusedRow == 1 { rowStyle = rowStyle.Background(ActiveTheme.Background).Foreground(ActiveTheme.Primary).Bold(true) }
	sb.WriteString(lipgloss.NewStyle().Foreground(ActiveTheme.Secondary).Render(" 📐 ASPECT RATIO (q/w/e/r/t):") + "\n")
	ratios := []string{"[q] Portrait (512x768)", "[w] Square (512x512)", "[e] Landscape (768x512)", "[r] HD Portrait (768x1024)", "[t] HD Landscape (1024x768)"}
	for i, r := range ratios {
		indicator := "  "
		if (i == 0 && m.apiConfig.ImageStyle.Width == 512 && m.apiConfig.ImageStyle.Height == 768) ||
		   (i == 1 && m.apiConfig.ImageStyle.Width == 512 && m.apiConfig.ImageStyle.Height == 512) ||
		   (i == 2 && m.apiConfig.ImageStyle.Width == 768 && m.apiConfig.ImageStyle.Height == 512) ||
		   (i == 3 && m.apiConfig.ImageStyle.Width == 768 && m.apiConfig.ImageStyle.Height == 1024) ||
		   (i == 4 && m.apiConfig.ImageStyle.Width == 1024 && m.apiConfig.ImageStyle.Height == 768) {
			indicator = " 📐"
		}
		sb.WriteString(fmt.Sprintf(" %s%s\n", r, indicator))
	}
	sb.WriteString("\n")

	// NEGATIVE PROMPT
	rowStyle = lipgloss.NewStyle()
	if m.focusedRow == 2 { rowStyle = rowStyle.Background(ActiveTheme.Background).Foreground(ActiveTheme.Primary).Bold(true) }
	sb.WriteString(rowStyle.Render(" 🚫 NEGATIVE PROMPT (TAB to focus):") + "\n")
	sb.WriteString(m.apiKeyInput.View() + "\n\n")

	// VISION & RAG SECTION
	sb.WriteString(lipgloss.NewStyle().Foreground(ActiveTheme.Secondary).Render(" ─── 🔭 Vision & RAG Providers ───────────────────────") + "\n")
	
	// VISION PROVIDER
	rowStyle = lipgloss.NewStyle()
	if m.focusedRow == 3 { rowStyle = rowStyle.Background(ActiveTheme.Background).Foreground(ActiveTheme.Primary).Bold(true) }
	sb.WriteString(rowStyle.Render(fmt.Sprintf(" 🔭 Vision Provider:  [ ← %s → ]", m.apiConfig.SelectedVisionLLMProvider)) + "\n")

	// RAG PROVIDER
	rowStyle = lipgloss.NewStyle()
	if m.focusedRow == 4 { rowStyle = rowStyle.Background(ActiveTheme.Background).Foreground(ActiveTheme.Primary).Bold(true) }
	sb.WriteString(rowStyle.Render(fmt.Sprintf(" 🧠 RAG Provider:     [ ← %s → ]", m.apiConfig.SelectedRAGLLMProvider)) + "\n\n")

	sb.WriteString(MutedStyle().Render(" [TAB] Navigation | [s] Save | [ESC] Back"))
	return sb.String()
}

func (m apiSettingsModel) renderCustomModelInput() string {
	var sb strings.Builder
	sb.WriteString(TitleStyle().Render(" ✏️  CUSTOM MODEL IDENTIFIER ") + "\n\n")
	sb.WriteString(" Enter the exact model name or path:\n\n")
	sb.WriteString(m.customModelInput.View() + "\n\n")
	sb.WriteString(MutedStyle().Render(" [ENTER] Apply | [ESC] Cancel"))
	return sb.String()
}

func (m *apiSettingsModel) handleImageStyleNumeric(key string) {
	num := int(key[0] - '0')
	
	styles := []string{"anime", "digital art", "photorealistic", "oil painting", "watercolor", "cyberpunk", "fantasy", "sketch"}
	if num >= 1 && num <= 8 {
		m.apiConfig.ImageStyle.StylePreset = styles[num-1]
		
		// Preset-specific ratio defaults
		switch m.apiConfig.ImageStyle.StylePreset {
		case "cyberpunk", "photorealistic":
			m.apiConfig.ImageStyle.Width = 768
			m.apiConfig.ImageStyle.Height = 512
			m.apiConfig.ImageStyle.AspectRatio = "landscape"
		case "anime", "fantasy":
			m.apiConfig.ImageStyle.Width = 512
			m.apiConfig.ImageStyle.Height = 768
			m.apiConfig.ImageStyle.AspectRatio = "portrait"
		default:
			m.apiConfig.ImageStyle.Width = 512
			m.apiConfig.ImageStyle.Height = 512
			m.apiConfig.ImageStyle.AspectRatio = "square"
		}
	}
	
	m.apiConfig.ImageStyle.NegativePrompt = m.apiKeyInput.Value()
}

func (m *apiSettingsModel) handleAspectRatioKey(key string) {
	switch key {
	case "q": // Portrait
		m.apiConfig.ImageStyle.Width = 512
		m.apiConfig.ImageStyle.Height = 768
		m.apiConfig.ImageStyle.AspectRatio = "portrait"
	case "w": // Square
		m.apiConfig.ImageStyle.Width = 512
		m.apiConfig.ImageStyle.Height = 512
		m.apiConfig.ImageStyle.AspectRatio = "square"
	case "e": // Landscape
		m.apiConfig.ImageStyle.Width = 768
		m.apiConfig.ImageStyle.Height = 512
		m.apiConfig.ImageStyle.AspectRatio = "landscape"
	case "r": // HD Portrait
		m.apiConfig.ImageStyle.Width = 768
		m.apiConfig.ImageStyle.Height = 1024
		m.apiConfig.ImageStyle.AspectRatio = "hd_portrait"
	case "t": // HD Landscape
		m.apiConfig.ImageStyle.Width = 1024
		m.apiConfig.ImageStyle.Height = 768
		m.apiConfig.ImageStyle.AspectRatio = "hd_landscape"
	}
}

// providerSupportsImages returns true for providers that do image generation.
func providerSupportsImages(p Provider) bool {
	switch p {
	case ProviderAIHorde, ProviderPollinations, ProviderOpenAI, ProviderHuggingFace, ProviderImageRouter:
		return true
	}
	return false
}

// providerSupportsLLM returns true for providers that do text/LLM.
func providerSupportsLLM(p Provider) bool {
	switch p {
	case ProviderImageRouter: // image-only
		return false
	}
	return true
}
