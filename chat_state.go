package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

// Chat styles
var (
	chatTitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ADD8E6")). // Light Blue
			Padding(1, 4).
			Align(lipgloss.Center).
			Bold(true).
			BorderStyle(lipgloss.DoubleBorder()).
			BorderForeground(lipgloss.Color("#ADD8E6"))

	chatPromptStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00FFFF")).
			Bold(true)

	chatStatusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(lipgloss.Color("#333333")).
			Padding(0, 1)

	chatErrorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF0000")).
			Bold(true).
			Padding(0, 1)

	chatBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#ADD8E6")).
			Padding(0, 1)

	chatHelpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#757575")).
			PaddingTop(1)

	chatSelectedItemStyle = lipgloss.NewStyle().
				PaddingLeft(2).
				Foreground(lipgloss.Color("#00FFFF")).
				Bold(true)

	chatItemStyle = lipgloss.NewStyle().
			PaddingLeft(2).
			Foreground(lipgloss.Color("#ADD8E6"))
)

type MessageType string

const (
	MessageTypeUser      MessageType = "user"
	MessageTypeCharacter MessageType = "character"
	MessageTypeSystem    MessageType = "system"
	MessageTypeImage     MessageType = "image"
	MessageTypeCommand   MessageType = "command"
	MessageTypeSummary   MessageType = "summary"
	MessageTypeNarrator  MessageType = "narrator"
)

type APIProvider = Provider

type chatState int

const (
	selectingCharacter chatState = iota // always the first state shown
	chatting
	selectingLLMProvider
	selectingLLMModel
	selectingImageProvider
	selectingImageModel
	selectingSessionToImport
)

// Message represents a single chat turn.
type Message struct {
	ID        string            `json:"id"`
	Sender    string            `json:"sender"`
	Content   string            `json:"content"`
	Timestamp int64             `json:"timestamp"`
	Recipient string            `json:"recipient,omitempty"` // For DMs
	Type      MessageType       `json:"type"`                // Type of message (user, character, system, image, etc.)
	Metadata  map[string]string `json:"metadata,omitempty"`  // Additional data, e.g., image URL for image messages
	// Crypto fields
	Signature string `json:"signature,omitempty"` // ECDSA sig by character's private key
	SignedBy  string `json:"signedBy,omitempty"`  // character ID that signed this
}

// ChatMsg is a message that represents a single chat turn.
type ChatMsg struct {
	Message Message
	Error   error
}

// UpdateImageDescMsg updates a chat message's vision description after async analysis.
type UpdateImageDescMsg struct {
	MessageID   string
	Description string
}

// MultiModalPart is one content block inside a MultiModalMessage.
type MultiModalPart struct {
	Type      string // "text" or "image"
	Text      string
	ImageData []byte
	MimeType  string // e.g. "image/jpeg", "image/png"
}

// MultiModalMessage is a structured chat message that can carry image data.
type MultiModalMessage struct {
	Role  string           // "system", "user", or "assistant"
	Parts []MultiModalPart
}

// TextMsg returns a simple single-part text MultiModalMessage.
func TextMsg(role, text string) MultiModalMessage {
	return MultiModalMessage{Role: role, Parts: []MultiModalPart{{Type: "text", Text: text}}}
}

type chatModel struct {
	user             *User
	apiConfig        *APIConfig
	memoryManager    MemoryManager
	llmService       LLMService   // Text chat LLM (fast/local)
	visionLLMService LLMService   // Multimodal LLM for image analysis + vision chat
	ragLLMService    LLMService   // RAG LLM for memory summarization + lore indexing
	imageService     ImageService

	messages  []Message
	textInput textinput.Model
	viewport  viewport.Model
	err       error

	state                 chatState
	providerChoices       []string
	providerCursor        int
	selectedLLMProvider   APIProvider
	selectedImageProvider APIProvider
	selectedLLMModel      string   // The chosen LLM model for chat
	selectedImageModel    string   // The chosen Image model for chat
	llmModels             []string // Available LLM models for the selected provider in chat
	imageModels           []string // Available Image models for the selected provider in chat

	// Current chat participants (characters + user)
	participants        []string
	activeCharacterID   string // ID of the currently active character in conversation
	selectedCharacterID string // ID of the character the user is currently talking to
	systemPrompt        string // Stores the active system prompt for LLM interaction

	// UI specific
	chatHeight    int
	chatWidth     int
	turn          int    // 0 for user, 1 for first char, 2 for second, etc.
	chatSessionID string // Unique ID for the current chat session

	// Cache for loaded characters (for signing/verification)
	loadedCharacters map[string]*Character

	activeLorebooks    []Lorebook
	characterChoices   []CharacterInfo // populated on init for character picker

	pendingExec   string         // command awaiting approval
	approvalModel *approvalModel // non-nil when approval popup is active
}

func initialChatModel(loggedInUser *User, apiCfg *APIConfig, mm MemoryManager, llmSvc LLMService, visionSvc LLMService, imgSvc ImageService) chatModel {

	ti := textinput.New()
	ti.Placeholder = "Type your message or command..."
	ti.Focus()
	ti.Width = 80
	ti.Prompt = ">> "

	vp := viewport.New(80, 20)

	return chatModel{
		user:                  loggedInUser,
		apiConfig:             apiCfg,
		memoryManager:         mm,
		llmService:            llmSvc,
		visionLLMService:      visionSvc,
		ragLLMService:         llmSvc, // Default: same as chat LLM; set explicitly when RAG provider differs
		imageService:          imgSvc,
		messages:              []Message{},
		textInput:             ti,
		viewport:              vp,
		participants:          []string{loggedInUser.Username}, // User is always a participant
		systemPrompt:          "",
		state:                 selectingCharacter,
		selectedLLMProvider:   apiCfg.SelectedLLMProvider,
		selectedImageProvider: apiCfg.SelectedImageProvider,
		selectedLLMModel:      getDefaultModelForProvider(apiCfg),
		selectedImageModel:    "",
		llmModels:             []string{},
		imageModels:           []string{},
		turn:                  0,  // User starts
		chatSessionID:         "", // Initialize chat session ID
		loadedCharacters:      make(map[string]*Character),
	}
}

// getDefaultModelForProvider returns the default LLM model string for the
// currently selected provider, so chat works immediately without /provider.
// hardcodedDefaults is a last-resort fallback when config has no model saved.
// These are the FIRST item in each provider's GetAvailableModels() list —
// user can change them any time in Settings → Provider → [M] Model.
var hardcodedDefaults = map[Provider]string{
	ProviderPollinations: "openai",
	ProviderGemini:       "gemini-2.0-flash",
	ProviderMistral:      "mistral-large-latest",
	ProviderGroq:         "llama3-70b-8192",
	ProviderOpenAI:       "gpt-4o",
	ProviderClaude:       "claude-sonnet-4-6",
	ProviderOllama:       "llama3",
	ProviderOpenRouter:   "mistralai/mistral-7b-instruct:free",
	ProviderHuggingFace:  "HuggingFaceH4/zephyr-7b-beta",
	ProviderAIHorde:      "koboldcpp/HF_SPACE_Tiefighter_13B",
}
// Note: these are only used when the saved config has no model set (e.g. first
// run or config migration). User-selected models in Settings always take priority.

func getDefaultModelForProvider(cfg *APIConfig) string {
	if cfg == nil {
		return ""
	}
	var model string
	switch cfg.SelectedLLMProvider {
	case ProviderGemini:
		model = string(cfg.Gemini.DefaultLLMModel)
	case ProviderMistral:
		model = string(cfg.Mistral.DefaultLLMModel)
	case ProviderGroq:
		model = string(cfg.Groq.DefaultLLMModel)
	case ProviderOpenAI:
		model = string(cfg.OpenAI.DefaultLLMModel)
	case ProviderClaude:
		model = string(cfg.Claude.DefaultLLMModel)
	case ProviderOllama:
		model = string(cfg.Ollama.DefaultLLMModel)
	case ProviderOpenRouter:
		model = string(cfg.OpenRouter.DefaultLLMModel)
	case ProviderHuggingFace:
		model = string(cfg.HuggingFace.DefaultLLMModel)
	case ProviderPollinations:
		model = string(cfg.Pollinations.DefaultLLMModel)
	case ProviderAIHorde:
		model = string(cfg.AIHorde.DefaultLLMModel)
	default:
		if pcfg, ok := cfg.GetLLMProviderConfig(cfg.SelectedLLMProvider); ok {
			model = string(pcfg.DefaultLLMModel)
		}
	}
	// If config has no model stored (old saved config), use hardcoded fallback
	if model == "" {
		if def, ok := hardcodedDefaults[cfg.SelectedLLMProvider]; ok {
			return def
		}
	}
	return model
}

// SaveChatSession saves the current chat messages for a character.
func SaveChatSession(user *User, characterID string, messages []Message) error {
	if user == nil || characterID == "" {
		return fmt.Errorf("invalid user or character ID for saving chat session")
	}

	messagesBytes, err := json.Marshal(messages)
	if err != nil {
		return fmt.Errorf("failed to marshal chat messages: %w", err)
	}

	// Use characterID as the dataName for the chat session
	return SaveEncryptedData(user, user.Username, "chat_sessions", characterID, messagesBytes)
}

// LoadChatSession loads previous chat messages for a character.
func LoadChatSession(user *User, characterID string) ([]Message, error) {
	if user == nil || characterID == "" {
		return nil, fmt.Errorf("invalid user or character ID for loading chat session")
	}

	messagesBytes, err := LoadEncryptedData(user, user.Username, "chat_sessions", characterID)
	if err != nil {
		// If chat session not found, return empty messages rather than error
		if strings.Contains(err.Error(), "not found") {
			return []Message{}, nil
		}
		return nil, fmt.Errorf("failed to load encrypted chat messages: %w", err)
	}

	var messages []Message
	if err := json.Unmarshal(messagesBytes, &messages); err != nil {
		return nil, fmt.Errorf("failed to unmarshal chat messages: %w", err)
	}
	return messages, nil
}

// Message to explicitly set the active character for chat.
type SetChatCharacterMsg struct {
	CharacterID string
}

