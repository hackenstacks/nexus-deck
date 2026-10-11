package main

import (
	"encoding/json"
	"fmt"
)

// Provider represents a specific API provider.
type Provider string

const (
	ProviderAIHorde      Provider = "AIHorde"
	ProviderPollinations Provider = "Pollinations.ai"
	ProviderImageRouter  Provider = "ImageRouter"
	ProviderMistral      Provider = "Mistral"
	ProviderGroq         Provider = "Groq"
	ProviderHuggingFace  Provider = "HuggingFace"
	ProviderOpenAI       Provider = "OpenAI"
	ProviderClaude       Provider = "Claude"
	ProviderOpenRouter   Provider = "OpenRouter"
	ProviderGemini       Provider = "Gemini"
	ProviderStability    Provider = "Stability"
	ProviderCustomLLM    Provider = "CustomLLM"
	ProviderCustomImage  Provider = "CustomImage"
	ProviderLocal        Provider = "Local"
	ProviderOllama       Provider = "Ollama"
	ProviderNexusRouter  Provider = "NexusRouter" // nexus-ai-router at localhost:8810 — local-first fleet manager
	ProviderSecurity     Provider = "Security"
	ProviderMock         Provider = "Mock"
)

// LLMModel represents a large language model.
type LLMModel string

// ImageModel represents an image generation model.
type ImageModel string

// Common API fields
type CommonAPIConfig struct {
	Enabled bool   `json:"enabled"`
	APIKey  string `json:"apiKey"`
	BaseURL string `json:"baseURL,omitempty"` // For custom or self-hosted instances
	DefaultLLMModel LLMModel `json:"defaultLLMModel,omitempty"`
	DefaultImageModel ImageModel `json:"defaultImageModel,omitempty"`
}

// AIHordeConfig for AIHorde API
type AIHordeConfig struct {
	CommonAPIConfig
	DefaultImageModel ImageModel `json:"defaultImageModel,omitempty"`
	// AI Horde has a concept of workers and kudos, but for a basic config, APIKey is enough
}

// PollinationsConfig for Pollinations.ai API
type PollinationsConfig struct {
	CommonAPIConfig
	TextBaseURL  string `json:"textBaseURL,omitempty"`
	ImageBaseURL string `json:"imageBaseURL,omitempty"`
}

// ImageRouterConfig for ImageRouter API (placeholder, assume it routes to other image APIs)
type ImageRouterConfig struct {
	CommonAPIConfig
	DefaultImageModel ImageModel `json:"defaultImageModel,omitempty"`
}

// MistralConfig for Mistral API
type MistralConfig struct {
	CommonAPIConfig
	DefaultLLMModel LLMModel `json:"defaultLLMModel,omitempty"`
}

// GroqConfig for Groq API
type GroqConfig struct {
	CommonAPIConfig
	DefaultLLMModel LLMModel `json:"defaultLLMModel,omitempty"`
}

// HuggingFaceConfig for HuggingFace Inference API
type HuggingFaceConfig struct {
	CommonAPIConfig
	DefaultLLMModel   LLMModel `json:"defaultLLMModel,omitempty"`
	DefaultImageModel ImageModel `json:"defaultImageModel,omitempty"`
	// HuggingFace often uses model names as part of the URL
}

// OpenAIConfig for OpenAI API
type OpenAIConfig struct {
	CommonAPIConfig
	DefaultLLMModel   LLMModel `json:"defaultLLMModel,omitempty"`
	DefaultImageModel ImageModel `json:"defaultImageModel,omitempty"` // DALL-E models
	OrganizationID    string   `json:"organizationId,omitempty"`
}

// ClaudeConfig for Anthropic Claude API
type ClaudeConfig struct {
	CommonAPIConfig
	DefaultLLMModel LLMModel `json:"defaultLLMModel,omitempty"`
}

// OllamaConfig for Ollama API
type OllamaConfig struct {
	CommonAPIConfig
}

// OpenRouterConfig for OpenRouter (100+ models via one key)
type OpenRouterConfig struct {
	CommonAPIConfig
}

// GeminiConfig for Google Gemini API
type GeminiConfig struct {
	CommonAPIConfig
}

// StabilityConfig for Stability AI image generation
type StabilityConfig struct {
	CommonAPIConfig
	DefaultImageModel ImageModel `json:"defaultImageModel,omitempty"`
}

// ImageStyleConfig for visual style persistence
type ImageStyleConfig struct {
	StylePreset    string `json:"stylePreset"`
	NegativePrompt string `json:"negativePrompt"`
	AspectRatio    string `json:"aspectRatio"`
	Width          int    `json:"width"`
	Height         int    `json:"height"`
}

// CustomAPIConfig for user-defined LLM or Image APIs
type CustomAPIConfig struct {
	CommonAPIConfig
	Name              string     `json:"name"` // User-defined name for the custom API
	Type              string     `json:"type"` // "llm" or "image"
	DefaultLLMModel   LLMModel   `json:"defaultLLMModel,omitempty"`
	DefaultImageModel ImageModel `json:"defaultImageModel,omitempty"`
	Endpoint          string     `json:"endpoint"` // Custom API endpoint
	// Potentially add fields for custom headers, auth methods etc.
}

// APIConfig holds the configurations for all supported API providers.
type APIConfig struct {
	AIHorde      AIHordeConfig      `json:"aihorde"`
	Pollinations PollinationsConfig `json:"pollinations"`
	ImageRouter  ImageRouterConfig  `json:"imagerouter"`
	Mistral      MistralConfig      `json:"mistral"`
	Groq         GroqConfig         `json:"groq"`
	HuggingFace  HuggingFaceConfig  `json:"huggingface"`
	OpenAI       OpenAIConfig       `json:"openai"`
	Claude       ClaudeConfig       `json:"claude"`
	Ollama       OllamaConfig       `json:"ollama"`
	OpenRouter   OpenRouterConfig   `json:"openrouter"`
	Gemini       GeminiConfig       `json:"gemini"`
	Stability    StabilityConfig    `json:"stability"`
	CustomAPIs   []CustomAPIConfig  `json:"customApis"` // Allow multiple custom APIs
	// Currently selected providers — each role can use a different model/provider
	SelectedLLMProvider       Provider `json:"selectedLLMProvider"`        // Main chat text generation
	SelectedVisionLLMProvider Provider `json:"selectedVisionLLMProvider"`  // Multimodal: image analysis + vision chat
	SelectedRAGLLMProvider    Provider `json:"selectedRAGLLMProvider"`     // Memory summarization + lore indexing
	SelectedImageProvider     Provider `json:"selectedImageProvider"`
	SelectedEmbeddingProvider Provider `json:"selectedEmbeddingProvider"`

	// Image Style settings
	ImageStyle ImageStyleConfig `json:"imageStyle"`

	// Auto-generation settings
	AutoImageGeneration bool `json:"autoImageGeneration"`
	AutoImageFrequency  int  `json:"autoImageFrequency"` // e.g., every 10 messages
}

// NewAPIConfig creates a new APIConfig with default values and known endpoints pre-filled.
func NewAPIConfig() APIConfig {
	return APIConfig{
		SelectedLLMProvider:       ProviderPollinations,
		SelectedVisionLLMProvider: ProviderPollinations, // Pollinations vision — primary choice
		SelectedRAGLLMProvider:    ProviderPollinations, // Fast/cheap model for memory summarization
		SelectedImageProvider:     ProviderPollinations,
		SelectedEmbeddingProvider: ProviderLocal,
		ImageStyle: ImageStyleConfig{
			StylePreset:    "digital art",
			NegativePrompt: "blurry, low quality, watermark, text, ugly, deformed",
			AspectRatio:    "square",
			Width:          512,
			Height:         512,
		},
		AutoImageGeneration:       false,
		AutoImageFrequency:        10,
		AIHorde: AIHordeConfig{
			CommonAPIConfig: CommonAPIConfig{
				Enabled: true, 
				BaseURL: "https://aihorde.net/api",
				DefaultLLMModel: "koboldcpp/HF_SPACE_Tiefighter_13B",
				DefaultImageModel: "stable_diffusion",
			},
		},
		Pollinations: PollinationsConfig{
			CommonAPIConfig: CommonAPIConfig{
				Enabled: true, 
				BaseURL: "https://enter.pollinations.ai/v1",
				DefaultLLMModel: "openai",
				DefaultImageModel: "flux",
			},
			TextBaseURL:     "https://enter.pollinations.ai",
			ImageBaseURL:    "https://enter.pollinations.ai",
		},
		Mistral: MistralConfig{
			CommonAPIConfig: CommonAPIConfig{
				Enabled: true,
				BaseURL: "https://api.mistral.ai/v1",
				DefaultLLMModel: "mistral-tiny",
			},
		},
		Groq: GroqConfig{
			CommonAPIConfig: CommonAPIConfig{
				Enabled: true,
				BaseURL: "https://api.groq.com/openai/v1",
				DefaultLLMModel: "llama3-8b-8192",
			},
		},
		OpenAI: OpenAIConfig{
			CommonAPIConfig: CommonAPIConfig{
				Enabled: true,
				BaseURL: "https://api.openai.com/v1",
				DefaultLLMModel: "gpt-3.5-turbo",
				DefaultImageModel: "dall-e-3",
			},
		},
		Claude: ClaudeConfig{
			CommonAPIConfig: CommonAPIConfig{
				Enabled: true,
				BaseURL: "https://api.anthropic.com/v1",
				DefaultLLMModel: "claude-3-haiku-20240307",
			},
		},
		Gemini: GeminiConfig{
			CommonAPIConfig: CommonAPIConfig{
				Enabled: true,
				BaseURL: "https://generativelanguage.googleapis.com/v1beta",
				DefaultLLMModel: "gemini-1.5-flash",
			},
		},
		Ollama: OllamaConfig{
			CommonAPIConfig: CommonAPIConfig{
				Enabled: true, 
				BaseURL: "http://127.0.0.1:11434/v1",
				DefaultLLMModel: "qwen2.5-coder:1.5b",
			},
		},
	}
}

// GetLLMProviderConfig retrieves the specific configuration for a given LLM Provider.
func (c *APIConfig) GetLLMProviderConfig(provider Provider) (CommonAPIConfig, bool) {
	switch provider {
	case ProviderMistral:
		return c.Mistral.CommonAPIConfig, true
	case ProviderGroq:
		return c.Groq.CommonAPIConfig, true
	case ProviderHuggingFace:
		return c.HuggingFace.CommonAPIConfig, true
	case ProviderOpenAI:
		return c.OpenAI.CommonAPIConfig, true
	case ProviderClaude:
		return c.Claude.CommonAPIConfig, true
	case ProviderOllama:
		return c.Ollama.CommonAPIConfig, true
	case ProviderPollinations:
		conf := c.Pollinations.CommonAPIConfig
		if c.Pollinations.TextBaseURL != "" {
			conf.BaseURL = c.Pollinations.TextBaseURL + "/v1"
		}
		return conf, true
	case ProviderGemini:
		return c.Gemini.CommonAPIConfig, true
	case ProviderAIHorde:
		return c.AIHorde.CommonAPIConfig, true
	case ProviderOpenRouter:
		return c.OpenRouter.CommonAPIConfig, true
	case ProviderLocal:
		return CommonAPIConfig{Enabled: true}, true
	case ProviderMock:
		return CommonAPIConfig{Enabled: true, APIKey: "mock-key"}, true
	default:
		// Check custom APIs
		for _, custom := range c.CustomAPIs {
			if custom.Name == string(provider) && custom.Type == "llm" {
				return custom.CommonAPIConfig, true
			}
		}
		return CommonAPIConfig{}, false
	}
}

// GetImageProviderConfig retrieves the specific configuration for a given Image Provider.
func (c *APIConfig) GetImageProviderConfig(provider Provider) (CommonAPIConfig, bool) {
	switch provider {
	case ProviderAIHorde:
		return c.AIHorde.CommonAPIConfig, true
	case ProviderPollinations:
		conf := c.Pollinations.CommonAPIConfig
		if c.Pollinations.ImageBaseURL != "" {
			conf.BaseURL = c.Pollinations.ImageBaseURL
		}
		return conf, true
	case ProviderImageRouter:
		return c.ImageRouter.CommonAPIConfig, true
	case ProviderHuggingFace:
		return c.HuggingFace.CommonAPIConfig, true
	case ProviderOpenAI: // DALL-E etc.
		return c.OpenAI.CommonAPIConfig, true
	case ProviderMock:
		return CommonAPIConfig{Enabled: true, APIKey: "mock-key"}, true
	default:
		// Check custom APIs
		for _, custom := range c.CustomAPIs {
			if custom.Name == string(provider) && custom.Type == "image" {
				return custom.CommonAPIConfig, true
			}
		}
		return CommonAPIConfig{}, false
	}
}

// GetEmbeddingProviderConfig retrieves the specific configuration for a given Embedding Provider.
func (c *APIConfig) GetEmbeddingProviderConfig(provider Provider) (CommonAPIConfig, bool) {
	switch provider {
	case ProviderOpenAI:
		return c.OpenAI.CommonAPIConfig, true
	case ProviderLocal:
		return CommonAPIConfig{Enabled: true}, true
	case ProviderMock:
		return CommonAPIConfig{Enabled: true, APIKey: "mock-key"}, true
	default:
		return CommonAPIConfig{}, false
	}
}

// GetCustomAPIConfig retrieves a custom API configuration by name and type.
func (c *APIConfig) GetCustomAPIConfig(name string, apiType string) (CustomAPIConfig, bool) {
	for _, custom := range c.CustomAPIs {
		if custom.Name == name && custom.Type == apiType {
			return custom, true
		}
	}
	return CustomAPIConfig{}, false
}

// GetVisionProviderModel returns the default model for the configured vision provider.
// No hardcoded model names — reads from the provider's own config.
func (c *APIConfig) GetVisionProviderModel() string {
	pcfg, ok := c.GetLLMProviderConfig(c.SelectedVisionLLMProvider)
	if ok && pcfg.DefaultLLMModel != "" {
		return string(pcfg.DefaultLLMModel)
	}
	// Fall through to text provider model as last resort
	pcfg, ok = c.GetLLMProviderConfig(c.SelectedLLMProvider)
	if ok && pcfg.DefaultLLMModel != "" {
		return string(pcfg.DefaultLLMModel)
	}
	return ""
}

// GetActiveImageModel returns the default model for the configured image provider.
func (c *APIConfig) GetActiveImageModel() string {
	pcfg, ok := c.GetImageProviderConfig(c.SelectedImageProvider)
	if ok && pcfg.DefaultImageModel != "" {
		return string(pcfg.DefaultImageModel)
	}
	return ""
}

// SaveAPIConfig saves the API configuration for a user.
func SaveAPIConfig(user *User, config APIConfig) error {
	if user == nil || user.Username == "" {
		return fmt.Errorf("invalid user for saving API config")
	}
	configBytes, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal API config: %w", err)
	}
	// Use the generic SaveEncryptedData
	return SaveEncryptedData(user, user.Username, "configs", "api_config", configBytes)
}

// LoadAPIConfig loads the API configuration for a user.
func LoadAPIConfig(user *User) (APIConfig, error) {
	if user == nil || user.Username == "" {
		return APIConfig{}, fmt.Errorf("invalid user for loading API config")
	}
	configBytes, err := LoadEncryptedData(user, user.Username, "configs", "api_config")
	if err != nil {
		// If config not found, return a new default one
		if err.Error() == fmt.Sprintf("data 'configs/api_config' not found for user '%s'", user.Username) {
			return NewAPIConfig(), nil
		}
		return APIConfig{}, fmt.Errorf("failed to load encrypted API config: %w", err)
	}
	var config APIConfig
	if err := json.Unmarshal(configBytes, &config); err != nil {
		return APIConfig{}, fmt.Errorf("failed to unmarshal API config: %w", err)
	}
	return config, nil
}