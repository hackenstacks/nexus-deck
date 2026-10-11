package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

// LLMService defines the interface for interacting with Large Language Models.
type LLMService interface {
	GenerateResponse(ctx context.Context, prompt string, model LLMModel, config APIConfig) (string, error)
	AnalyzeImage(ctx context.Context, imageData []byte, prompt string, model LLMModel, config APIConfig) (string, error)
	GetAvailableModels(ctx context.Context) ([]LLMModel, error)
	Provider() Provider
}


// MultiModalLLMService is an optional capability interface for providers that support
// sending images directly in the chat conversation (Gemini, OpenAI, Claude, Ollama, etc.).
// Use a type assertion to check if the active LLMService supports this.
type MultiModalLLMService interface {
	GenerateResponseMultimodal(ctx context.Context, messages []MultiModalMessage, model LLMModel, config APIConfig) (string, error)
}

// ImageService defines the interface for interacting with Image Generation Models.
type ImageService interface {
	GenerateImage(ctx context.Context, prompt string, model ImageModel, config APIConfig) (string, error) // Returns URL or base64
	GetAvailableModels(ctx context.Context) ([]ImageModel, error)
	Provider() Provider
}

// EmbeddingService defines the interface for text embedding models.
type EmbeddingService interface {
	CreateEmbedding(ctx context.Context, text string) ([]float32, error)
	Provider() Provider
}

// Summarizer defines the interface for text summarization.
type Summarizer interface {
	Summarize(ctx context.Context, text string) (string, error)
	Provider() Provider
}

// --- Mock Implementations ---

// GetVisionService returns a vision-capable LLM service and its model.
// It tries the current provider first; if it doesn't support vision it falls back
// to Gemini, then OpenAI, since both use the OpenAI-compat multimodal endpoint.
// visionCapable returns true for providers with working AnalyzeImage implementations.
// AIHorde excluded — vision API not yet implemented.
func visionCapable(p Provider) bool {
	switch p {
	case ProviderGemini, ProviderOpenAI, ProviderClaude, ProviderOllama,
		ProviderOpenRouter, ProviderHuggingFace, ProviderPollinations:
		return true
	}
	return false
}

var (
	pollinationsVisionCache LLMModel
	lastPollinationsUpdate  time.Time
)

// pollinationsVisionModel queries the Pollinations /v1/models endpoint and returns
// the first model with vision:true. Uses a cache to prevent 429 errors.
func pollinationsVisionModel(apiKey string) (LLMModel, bool) {
	// 1. Check Cache (valid for 1 hour)
	if pollinationsVisionCache != "" && time.Since(lastPollinationsUpdate) < time.Hour {
		return pollinationsVisionCache, true
	}

	if apiKey == "" {
		// Without a key, we default to the known vision-capable router
		return "openai", true
	}
	
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("GET", "https://enter.pollinations.ai/v1/models", nil)
	if err != nil {
		return "openai", true // Fallback to hardcoded
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return "openai", true // Fallback
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		NexusLog("WARN", "vision", "Pollinations model list rate limited (429), using cache/fallback")
		if pollinationsVisionCache != "" {
			return pollinationsVisionCache, true
		}
		return "openai", true
	}

	// Response is a plain array: [{"name":"...","input_modalities":["text","image"],...}]
	var models []struct {
		Name            string   `json:"name"`
		InputModalities []string `json:"input_modalities"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&models); err != nil {
		return "openai", true
	}
	for _, m := range models {
		for _, mod := range m.InputModalities {
			if mod == "image" {
				pollinationsVisionCache = LLMModel(m.Name)
				lastPollinationsUpdate = time.Now()
				return pollinationsVisionCache, true
			}
		}
	}
	return "openai", true
}

func GetVisionService(current LLMService, cfg *APIConfig) (LLMService, LLMModel) {
	// 1. Primary Choice: Pollinations (New Default)
	// If Pollinations is selected OR if it's the only one with an API key
	apiKey := cfg.Pollinations.APIKey
	if (cfg.SelectedVisionLLMProvider == ProviderPollinations || cfg.SelectedVisionLLMProvider == "") && apiKey != "" {
		svc := NewOpenAILLMService(apiKey, "https://enter.pollinations.ai/v1")
		if model, ok := pollinationsVisionModel(apiKey); ok {
			NexusLog("INFO", "vision", fmt.Sprintf("Primary Vision: Pollinations model %s", model))
			return svc, model
		}
	}

	// 2. User Selection (if not Pollinations or if Pollinations search failed)
	vp := cfg.SelectedVisionLLMProvider
	if vp != "" && vp != ProviderPollinations && visionCapable(vp) {
		if svc, err := LLMFactory(vp, *cfg); err == nil {
			if pcfg, ok := cfg.GetLLMProviderConfig(vp); ok && pcfg.DefaultLLMModel != "" {
				return svc, pcfg.DefaultLLMModel
			}
		}
	}

	// 3. Fallback Chain: Gemini → OpenAI → Pollinations (retry)
	if cfg.Gemini.APIKey != "" && cfg.Gemini.Enabled {
		baseURL := cfg.Gemini.BaseURL
		if baseURL == "" {
			baseURL = "https://generativelanguage.googleapis.com/v1beta/openai"
		}
		return NewOpenAILLMService(cfg.Gemini.APIKey, baseURL), cfg.Gemini.DefaultLLMModel
	}
	if cfg.OpenAI.APIKey != "" && cfg.OpenAI.Enabled {
		return NewOpenAILLMService(cfg.OpenAI.APIKey, cfg.OpenAI.BaseURL), cfg.OpenAI.DefaultLLMModel
	}
	
	// Final attempt: Pollinations without a key (legacy/anon paths)
	if model, ok := pollinationsVisionModel(""); ok {
		return NewPollinationsLLMService("", ""), model
	}

	return current, ""
}

// MockLLMService is a mock implementation of LLMService for testing.
type MockLLMService struct{}

func (m *MockLLMService) GenerateResponse(ctx context.Context, prompt string, model LLMModel, config APIConfig) (string, error) {
	fmt.Printf("MockLLMService: Generating text for prompt '%s' with model '%s' (Config: %+v)\n", prompt, model, config)
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(50 * time.Millisecond):
		return fmt.Sprintf("Mock response for: '%s' (model: %s)", prompt, model), nil
	}
}

func (m *MockLLMService) AnalyzeImage(ctx context.Context, imageData []byte, prompt string, model LLMModel, config APIConfig) (string, error) {
	return "Mock description: A high-signal cryptographic artifact in a neon-lit terminal.", nil
}

func (m *MockLLMService) GetAvailableModels(ctx context.Context) ([]LLMModel, error) {
	return []LLMModel{"mock-llm-fast", "mock-llm-standard", "mock-llm-creative"}, nil
}

func (m *MockLLMService) Provider() Provider {
	return ProviderMock
}

// MockImageService is a mock implementation of ImageService for testing.
type MockImageService struct{}

func (m *MockImageService) GenerateImage(ctx context.Context, prompt string, model ImageModel, config APIConfig) (string, error) {
	fmt.Printf("MockImageService: Generating image for prompt '%s' with model '%s' (Config: %+v)\n", prompt, model, config)
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(100 * time.Millisecond):
		return "https://via.placeholder.com/150?text=MockImage", nil
	}
}

func (m *MockImageService) GetAvailableModels(ctx context.Context) ([]ImageModel, error) {
	return []ImageModel{"mock-image-v1", "mock-image-v2-hd"}, nil
}

func (m *MockImageService) Provider() Provider {
	return ProviderMock
}

// MockEmbeddingService is a mock implementation of EmbeddingService.
type MockEmbeddingService struct{}

func (m *MockEmbeddingService) CreateEmbedding(ctx context.Context, text string) ([]float32, error) {
	fmt.Printf("MockEmbeddingService: Creating embedding for text '%s'\n", text)
	return []float32{0.1, 0.2, 0.3, 0.4, 0.5}, nil
}

func (m *MockEmbeddingService) Provider() Provider {
	return ProviderMock
}

// LocalEmbeddingService is a simple, free embedding service that uses feature hashing.
type LocalEmbeddingService struct{}

func (s *LocalEmbeddingService) CreateEmbedding(ctx context.Context, text string) ([]float32, error) {
	// Simple feature hashing (hashing trick) to 1024 dimensions
	dims := 1024
	vector := make([]float32, dims)
	
	// Normalize and tokenize
	words := strings.Fields(strings.ToLower(text))
	if len(words) == 0 {
		return vector, nil
	}

	for _, word := range words {
		// Basic hash function
		hash := uint32(0)
		for i := 0; i < len(word); i++ {
			hash = hash*31 + uint32(word[i])
		}
		
		index := hash % uint32(dims)
		vector[index] += 1.0
	}

	// L2 Normalization
	var sumSq float32
	for _, v := range vector {
		sumSq += v * v
	}
	if sumSq > 0 {
		norm := float32(math.Sqrt(float64(sumSq)))
		for i := range vector {
			vector[i] /= norm
		}
	}

	return vector, nil
}

func (s *LocalEmbeddingService) Provider() Provider { return ProviderLocal }

// MockSummarizer is a mock implementation of Summarizer.
type MockSummarizer struct{}

func (m *MockSummarizer) Summarize(ctx context.Context, text string) (string, error) {
	fmt.Printf("MockSummarizer: Summarizing text '%s'\n", text)
	return fmt.Sprintf("Mock summary of: '%s'", text), nil
}

func (m *MockSummarizer) Provider() Provider {
	return ProviderMock
}

// --- AI Horde Implementation ---

const aihordeBaseURL = "https://aihorde.net/api/v2"
const aihordeAnonymousKey = "0000000000"

// AIHordeImageService implements ImageService for AI Horde.
type AIHordeImageService struct {
	client  *http.Client
	apiKey  string
	baseURL string
}

// NewAIHordeImageService creates a new AIHordeImageService.
func NewAIHordeImageService(apiKey, baseURL string) *AIHordeImageService {
	if baseURL == "" {
		baseURL = aihordeBaseURL
	}
	if apiKey == "" {
		apiKey = aihordeAnonymousKey
	}
	return &AIHordeImageService{
		client:  &http.Client{Timeout: 60 * time.Second},
		apiKey:  apiKey,
		baseURL: baseURL,
	}
}

// ImageGenerationRequest represents the payload for initiating an AI Horde image generation.
type aihordeImageGenerationRequest struct {
	Prompt string `json:"prompt"`
	Models []string `json:"models"`
	Params *struct {
		Width  int `json:"width"`
		Height int `json:"height"`
		N      int `json:"n"` // Number of images to generate
	} `json:"params,omitempty"`
	Nsfw bool `json:"nsfw"`
	Shared bool `json:"shared"`
	Rk bool `json:"r_k"` // 'r_k' is typically a boolean in requests
}

// ImageGenerationResponse represents the response from initiating a generation.
type aihordeImageGenerationResponse struct {
	ID      string `json:"id"`
	Message string `json:"message"`
}

// GenerationStatusResponse represents the response from checking generation status.
type aihordeGenerationStatusResponse struct {
	ID          string `json:"id"`
	State       string `json:"state"`
	QueuePos    int    `json:"queue_position"`
	WaitTime    int    `json:"wait_time"`
	Done        bool   `json:"done"`
	Generations []struct {
		ID        string `json:"id"`
		Image     string `json:"img"`
		Model     string `json:"model"`
		Seed      string `json:"seed"`
		WorkerID  string `json:"worker_id"`
		WorkerName string `json:"worker_name"`
	} `json:"generations"`
}

func (s *AIHordeImageService) GenerateImage(ctx context.Context, prompt string, model ImageModel, config APIConfig) (string, error) {
	width := config.ImageStyle.Width
	height := config.ImageStyle.Height
	if width == 0 { width = 512 }
	if height == 0 { height = 512 }

	reqPayload := aihordeImageGenerationRequest{
		Prompt: prompt,
		Models: []string{string(model)},
		Params: &struct {
			Width  int `json:"width"`
			Height int `json:"height"`
			N      int `json:"n"`
		}{
			Width:  width,
			Height: height,
			N:      1, // Request 1 image
		},
		Nsfw:   false, // Can be made configurable
		Shared: true,  // Can be made configurable
		Rk:     false, // Can be made configurable
	}

	reqBody, err := json.Marshal(reqPayload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal AI Horde request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/generate/async", s.baseURL), bytes.NewBuffer(reqBody))
	if err != nil {
		return "", fmt.Errorf("failed to create AI Horde request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", s.apiKey)
	req.Header.Set("Client-Agent", "char-gen-cli:v0.1:gemini-agent") // Good practice to identify client

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send AI Horde request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("AI Horde initiation error: Status %d, Response: %s", resp.StatusCode, string(bodyBytes))
	}

	var genResp aihordeImageGenerationResponse
	if err := json.NewDecoder(resp.Body).Decode(&genResp); err != nil {
		return "", fmt.Errorf("failed to decode AI Horde initiation response: %w", err)
	}

	if genResp.ID == "" {
		return "", fmt.Errorf("AI Horde did not return a job ID")
	}

	// Poll for status
	ticker := time.NewTicker(2 * time.Second) // Poll every 2 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
			statusReq, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/generate/status/%s", s.baseURL, genResp.ID), nil)
			if err != nil {
				return "", fmt.Errorf("failed to create AI Horde status request: %w", err)
			}
			statusReq.Header.Set("apikey", s.apiKey)
			statusReq.Header.Set("Client-Agent", "char-gen-cli:v0.1:gemini-agent")

			statusResp, err := s.client.Do(statusReq)
			if err != nil {
				return "", fmt.Errorf("failed to send AI Horde status request: %w", err)
			}
			defer statusResp.Body.Close()

			if statusResp.StatusCode != http.StatusOK {
				bodyBytes, _ := io.ReadAll(statusResp.Body)
				return "", fmt.Errorf("AI Horde status error: Status %d, Response: %s", statusResp.StatusCode, string(bodyBytes))
			}

			var genStatus aihordeGenerationStatusResponse
			if err := json.NewDecoder(statusResp.Body).Decode(&genStatus); err != nil {
				return "", fmt.Errorf("failed to decode AI Horde status response: %w", err)
			}

			if genStatus.Done {
				if len(genStatus.Generations) > 0 && genStatus.Generations[0].Image != "" {
					imageURL := genStatus.Generations[0].Image
					// Download the image and return as base64
					imgResp, err := http.Get(imageURL)
					if err != nil {
						return "", fmt.Errorf("failed to download image from AI Horde: %w", err)
					}
					defer imgResp.Body.Close()

					if imgResp.StatusCode != http.StatusOK {
						return "", fmt.Errorf("failed to download image, status: %d", imgResp.StatusCode)
					}

					imgBytes, err := io.ReadAll(imgResp.Body)
					if err != nil {
						return "", fmt.Errorf("failed to read image bytes: %w", err)
					}
					return base64.StdEncoding.EncodeToString(imgBytes), nil
				}
				return "", fmt.Errorf("AI Horde generation complete but no image URL found")
			}
		}
	}
}

// GetAvailableModels for AI Horde - this requires a separate API call to /api/v2/status/models
func (s *AIHordeImageService) GetAvailableModels(ctx context.Context) ([]ImageModel, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/status/models", s.baseURL), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create AI Horde models request: %w", err)
	}
	req.Header.Set("apikey", s.apiKey)
	req.Header.Set("Client-Agent", "char-gen-cli:v0.1:gemini-agent")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send AI Horde models request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("AI Horde models status error: Status %d, Response: %s", resp.StatusCode, string(bodyBytes))
	}

	var models []struct {
		Name      string `json:"name"`
		Type      string `json:"type"` // "image", "text" etc.
		Queued    int    `json:"queued"`
		Count     int    `json:"count"`
		NSFW      bool   `json:"nsfw"`
		Hires     bool   `json:"hires"`
		Preferred bool   `json:"preferred"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&models); err != nil {
		return nil, fmt.Errorf("failed to decode AI Horde models response: %w", err)
	}

	var imageModels []ImageModel
	for _, m := range models {
		if m.Type == "image" && m.Count > 0 { // Only add models that are image types and have workers
			imageModels = append(imageModels, ImageModel(m.Name))
		}
	}
	return imageModels, nil
}

func (s *AIHordeImageService) Provider() Provider {
	return ProviderAIHorde
}

// --- AI Horde LLM Implementation ---

type AIHordeLLMService struct {
	client  *http.Client
	apiKey  string
	baseURL string
}

func (s *AIHordeLLMService) AnalyzeImage(ctx context.Context, imageData []byte, prompt string, model LLMModel, config APIConfig) (string, error) {
	return "", fmt.Errorf("AI Horde vision not yet supported")
}

func NewAIHordeLLMService(apiKey, baseURL string) *AIHordeLLMService {
	if baseURL == "" {
		baseURL = aihordeBaseURL
	}
	if apiKey == "" {
		apiKey = aihordeAnonymousKey
	}
	return &AIHordeLLMService{
		client:  &http.Client{Timeout: 120 * time.Second},
		apiKey:  apiKey,
		baseURL: baseURL,
	}
}

type aihordeTextGenerationRequest struct {
	Prompt string   `json:"prompt"`
	Models []string `json:"models"`
	Params struct {
		MaxLength        int     `json:"max_length"`
		MaxContextLength int     `json:"max_context_length"`
		Temperature      float64 `json:"temperature"`
		TopP             float64 `json:"top_p"`
	} `json:"params"`
}

type aihordeTextGenerationResponse struct {
	ID      string `json:"id"`
	Message string `json:"message"`
}

type aihordeTextStatusResponse struct {
	Done        bool `json:"done"`
	Generations []struct {
		Text string `json:"text"`
	} `json:"generations"`
}

func (s *AIHordeLLMService) GenerateResponse(ctx context.Context, prompt string, model LLMModel, config APIConfig) (string, error) {
	reqPayload := aihordeTextGenerationRequest{
		Prompt: prompt,
		Models: []string{string(model)},
	}
	reqPayload.Params.MaxLength = 512
	reqPayload.Params.MaxContextLength = 2048
	reqPayload.Params.Temperature = 0.7
	reqPayload.Params.TopP = 0.9

	reqBody, err := json.Marshal(reqPayload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/generate/text/async", s.baseURL), bytes.NewBuffer(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", s.apiKey)
	req.Header.Set("Client-Agent", "char-gen-cli:v0.1:gemini-agent")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("AI Horde text initiation error: Status %d, Response: %s", resp.StatusCode, string(bodyBytes))
	}

	var genResp aihordeTextGenerationResponse
	if err := json.NewDecoder(resp.Body).Decode(&genResp); err != nil {
		return "", err
	}

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
			statusReq, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/generate/text/status/%s", s.baseURL, genResp.ID), nil)
			if err != nil {
				return "", err
			}
			statusReq.Header.Set("apikey", s.apiKey)
			statusReq.Header.Set("Client-Agent", "char-gen-cli:v0.1:gemini-agent")

			statusResp, err := s.client.Do(statusReq)
			if err != nil {
				return "", err
			}
			defer statusResp.Body.Close()

			var genStatus aihordeTextStatusResponse
			if err := json.NewDecoder(statusResp.Body).Decode(&genStatus); err != nil {
				return "", err
			}

			if genStatus.Done {
				if len(genStatus.Generations) > 0 {
					return genStatus.Generations[0].Text, nil
				}
				return "", fmt.Errorf("AI Horde text generation complete but no text found")
			}
		}
	}
}

func (s *AIHordeLLMService) GetAvailableModels(ctx context.Context) ([]LLMModel, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/status/models", s.baseURL), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("apikey", s.apiKey)
	req.Header.Set("Client-Agent", "char-gen-cli:v0.1:gemini-agent")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var models []struct {
		Name string `json:"name"`
		Type string `json:"type"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&models); err != nil {
		return nil, err
	}

	var llmModels []LLMModel
	for _, m := range models {
		if m.Type == "text" {
			llmModels = append(llmModels, LLMModel(m.Name))
		}
	}
	if len(llmModels) == 0 {
		return []LLMModel{"koboldcpp/HF_SPACE_Tiefighter_13B", "mistralai/Mistral-7B-Instruct-v0.2"}, nil
	}
	return llmModels, nil
}

func (s *AIHordeLLMService) Provider() Provider { return ProviderAIHorde }

// --- Mistral Implementation ---

const mistralBaseURL = "https://api.mistral.ai/v1"

// MistralLLMService implements LLMService for Mistral AI.
type MistralLLMService struct {
	client  *http.Client
	apiKey  string
	baseURL string
}

func (s *MistralLLMService) AnalyzeImage(ctx context.Context, imageData []byte, prompt string, model LLMModel, config APIConfig) (string, error) {
	return "", fmt.Errorf("Mistral vision not yet supported in this client")
}

// NewMistralLLMService creates a new MistralLLMService.
func NewMistralLLMService(apiKey, baseURL string) *MistralLLMService {
	if baseURL == "" {
		baseURL = mistralBaseURL
	}
	return &MistralLLMService{
		client:  &http.Client{Timeout: 300 * time.Second}, // Longer timeout for LLM
		apiKey:  apiKey,
		baseURL: baseURL,
	}
}

// Mistral Chat Completion Request
// openAIChatRequest is a standard OpenAI-compatible chat completion request.
// Used by Mistral, Gemini (OpenAI-compat), Groq, Pollinations, OpenRouter, etc.
type openAIChatRequest struct {
	Model       string           `json:"model"`
	Messages    []openAIChatMessage `json:"messages"`
	Temperature float64          `json:"temperature,omitempty"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
	Stream      bool             `json:"stream,omitempty"`
}

type openAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openAIChatResponse is a standard OpenAI-compatible chat completion response.
type openAIChatResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int              `json:"index"`
		Message      openAIChatMessage `json:"message"`
		FinishReason string           `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// Keep old names as aliases so we don't miss any references
type mistralChatCompletionRequest = openAIChatRequest
type mistralChatMessage = openAIChatMessage
type mistralChatCompletionResponse = openAIChatResponse

func (s *MistralLLMService) GenerateResponse(ctx context.Context, prompt string, model LLMModel, config APIConfig) (string, error) {
	if s.apiKey == "" {
		return "", fmt.Errorf("Mistral API Key is missing")
	}

	reqPayload := mistralChatCompletionRequest{
		Model: string(model),
		Messages: []mistralChatMessage{
			{Role: "user", Content: prompt},
		},
		Temperature: 0.7, // Default temperature
		MaxTokens:   500, // Default max tokens
		Stream:      false,
	}

	reqBody, err := json.Marshal(reqPayload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal Mistral request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/chat/completions", s.baseURL), bytes.NewBuffer(reqBody))
	if err != nil {
		return "", fmt.Errorf("failed to create Mistral request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send Mistral request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Mistral API error: Status %d, Response: %s", resp.StatusCode, string(bodyBytes))
	}

	var chatCompletionResponse mistralChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatCompletionResponse); err != nil {
		return "", fmt.Errorf("failed to decode Mistral response: %w", err)
	}

	if len(chatCompletionResponse.Choices) > 0 {
		return chatCompletionResponse.Choices[0].Message.Content, nil
	}

	return "", fmt.Errorf("no response from Mistral API")
}

// GetAvailableModels for Mistral - requires an API call to /v1/models
func (s *MistralLLMService) GetAvailableModels(ctx context.Context) ([]LLMModel, error) {
	return []LLMModel{"mistral-large-latest", "mistral-small-latest", "codestral-latest", "open-mistral-7b"}, nil
}

func (s *MistralLLMService) Provider() Provider {
	return ProviderMistral
}

// --- OpenAI Implementation ---

const openaiBaseURL = "https://api.openai.com/v1"

// OpenAIEmbeddingService implements EmbeddingService for OpenAI.
type OpenAIEmbeddingService struct {
	client  *http.Client
	apiKey  string
	baseURL string
}

// NewOpenAIEmbeddingService creates a new OpenAIEmbeddingService.
func NewOpenAIEmbeddingService(apiKey, baseURL string) *OpenAIEmbeddingService {
	if baseURL == "" {
		baseURL = openaiBaseURL
	}
	return &OpenAIEmbeddingService{
		client:  &http.Client{Timeout: 30 * time.Second},
		apiKey:  apiKey,
		baseURL: baseURL,
	}
}

// OpenAIEmbeddingRequest represents the payload for an OpenAI embedding API call.
type openaiEmbeddingRequest struct {
	Input string `json:"input"`
	Model string `json:"model"`
}

// OpenAIEmbeddingResponse represents the response from an OpenAI embedding API call.
type openaiEmbeddingResponse struct {
	Object string `json:"object"`
	Data   []struct {
		Object    string    `json:"object"`
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Model string `json:"model"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

func (s *OpenAIEmbeddingService) CreateEmbedding(ctx context.Context, text string) ([]float32, error) {
	// TODO: Make model configurable via APIConfig
	model := "text-embedding-ada-002" // Default OpenAI embedding model

	reqPayload := openaiEmbeddingRequest{
		Input: text,
		Model: model,
	}

	reqBody, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal OpenAI embedding request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/embeddings", s.baseURL), bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create OpenAI embedding request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send OpenAI embedding request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OpenAI embedding API error: Status %d, Response: %s", resp.StatusCode, string(bodyBytes))
	}

	var embeddingResponse openaiEmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&embeddingResponse); err != nil {
		return nil, fmt.Errorf("failed to decode OpenAI embedding response: %w", err)
	}

	if len(embeddingResponse.Data) > 0 {
		return embeddingResponse.Data[0].Embedding, nil
	}

	return nil, fmt.Errorf("no embedding data returned from OpenAI API")
}

func (s *OpenAIEmbeddingService) Provider() Provider {
	return ProviderOpenAI
}

// --- OpenAI LLM Implementation ---

// OpenAILLMService implements LLMService for OpenAI.
type OpenAILLMService struct {
	client  *http.Client
	apiKey  string
	baseURL string
}

func NewOpenAILLMService(apiKey, baseURL string) *OpenAILLMService {
	if baseURL == "" {
		baseURL = openaiBaseURL
	}
	return &OpenAILLMService{
		client:  &http.Client{Timeout: 300 * time.Second},
		apiKey:  apiKey,
		baseURL: baseURL,
	}
}

func (s *OpenAILLMService) GenerateResponse(ctx context.Context, prompt string, model LLMModel, config APIConfig) (string, error) {
	reqPayload := mistralChatCompletionRequest{ // Re-using the same structure as it's OpenAI compatible
		Model: string(model),
		Messages: []mistralChatMessage{
			{Role: "user", Content: prompt},
		},
		Temperature: 0.7,
		MaxTokens:   1000,
	}

	reqBody, err := json.Marshal(reqPayload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/chat/completions", s.baseURL), bytes.NewBuffer(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("LLM API error %d: %.300s", resp.StatusCode, string(body))
	}

	var chatResp mistralChatCompletionResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", fmt.Errorf("LLM response parse error: %w (body: %.200s)", err, string(body))
	}

	if len(chatResp.Choices) > 0 {
		return chatResp.Choices[0].Message.Content, nil
	}
	return "", fmt.Errorf("no response from LLM provider")
}

func (s *OpenAILLMService) AnalyzeImage(ctx context.Context, imageData []byte, prompt string, model LLMModel, config APIConfig) (string, error) {
	type part struct {
		Type     string `json:"type"`
		Text     string `json:"text,omitempty"`
		ImageURL *struct {
			URL string `json:"url"`
		} `json:"image_url,omitempty"`
	}

	imgBase64 := base64.StdEncoding.EncodeToString(imageData)
	dataURL := fmt.Sprintf("data:image/jpeg;base64,%s", imgBase64)

	reqPayload := struct {
		Model    string `json:"model"`
		Messages []struct {
			Role    string `json:"role"`
			Content []part `json:"content"`
		} `json:"messages"`
		MaxTokens int `json:"max_tokens"`
	}{
		Model: string(model),
		MaxTokens: 1000,
	}

	reqPayload.Messages = append(reqPayload.Messages, struct {
		Role    string `json:"role"`
		Content []part `json:"content"`
	}{
		Role: "user",
		Content: []part{
			{Type: "text", Text: prompt},
			{Type: "image_url", ImageURL: &struct {
				URL string `json:"url"`
			}{URL: dataURL}},
		},
	})

	reqBody, err := json.Marshal(reqPayload)
	if err != nil { return "", err }

	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/chat/completions", s.baseURL), bytes.NewBuffer(reqBody))
	if err != nil { return "", err }
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	resp, err := s.client.Do(req)
	if err != nil { return "", err }
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil { return "", err }

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("vision API error %d: %.300s", resp.StatusCode, string(body))
	}

	// Some providers (e.g. Gemini OpenAI-compat) return errors as a JSON array.
	// Detect this early and surface a readable message.
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		// Try to extract an error message from the first element
		var errArr []map[string]interface{}
		if json.Unmarshal(body, &errArr) == nil && len(errArr) > 0 {
			if msg, ok := errArr[0]["message"].(string); ok && msg != "" {
				return "", fmt.Errorf("vision provider error: %s", msg)
			}
			if errObj, ok := errArr[0]["error"].(map[string]interface{}); ok {
				if msg, ok := errObj["message"].(string); ok {
					return "", fmt.Errorf("vision provider error: %s", msg)
				}
			}
		}
		return "", fmt.Errorf("vision provider returned unexpected array response: %.200s", string(body))
	}

	var chatResp mistralChatCompletionResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", fmt.Errorf("vision response parse error: %w (body: %.200s)", err, string(body))
	}

	if len(chatResp.Choices) > 0 {
		return chatResp.Choices[0].Message.Content, nil
	}
	return "", fmt.Errorf("vision analysis returned no content")
}

// GenerateResponseMultimodal sends a structured message array to the API, including
// actual image bytes as base64 data-URIs. This lets the LLM see images natively
// rather than reading a text description of them.
func (s *OpenAILLMService) GenerateResponseMultimodal(ctx context.Context, messages []MultiModalMessage, model LLMModel, config APIConfig) (string, error) {
	type contentPart struct {
		Type     string `json:"type"`
		Text     string `json:"text,omitempty"`
		ImageURL *struct {
			URL string `json:"url"`
		} `json:"image_url,omitempty"`
	}
	type message struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}

	var msgs []message
	for _, mm := range messages {
		hasImage := false
		for _, p := range mm.Parts {
			if p.Type == "image" {
				hasImage = true
				break
			}
		}

		if !hasImage && len(mm.Parts) == 1 {
			// Pure text — use plain string content (broadest provider compatibility)
			content, _ := json.Marshal(mm.Parts[0].Text)
			msgs = append(msgs, message{Role: mm.Role, Content: content})
		} else {
			// Multimodal — use array of content parts
			var parts []contentPart
			for _, p := range mm.Parts {
				if p.Type == "text" {
					parts = append(parts, contentPart{Type: "text", Text: p.Text})
				} else if p.Type == "image" && len(p.ImageData) > 0 {
					mimeType := p.MimeType
					if mimeType == "" {
						mimeType = "image/jpeg"
					}
					imgB64 := base64.StdEncoding.EncodeToString(p.ImageData)
					dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, imgB64)
					parts = append(parts, contentPart{
						Type: "image_url",
						ImageURL: &struct {
							URL string `json:"url"`
						}{URL: dataURL},
					})
				}
			}
			content, _ := json.Marshal(parts)
			msgs = append(msgs, message{Role: mm.Role, Content: content})
		}
	}

	reqPayload := struct {
		Model       string    `json:"model"`
		Messages    []message `json:"messages"`
		MaxTokens   int       `json:"max_tokens"`
		Temperature float64   `json:"temperature"`
	}{
		Model:       string(model),
		Messages:    msgs,
		MaxTokens:   2000,
		Temperature: 0.7,
	}

	reqBody, err := json.Marshal(reqPayload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/chat/completions", s.baseURL), bytes.NewBuffer(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var chatResp mistralChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", err
	}
	if len(chatResp.Choices) > 0 {
		return chatResp.Choices[0].Message.Content, nil
	}
	return "", fmt.Errorf("no response from multimodal LLM provider")
}

func (s *OpenAILLMService) GetAvailableModels(ctx context.Context) ([]LLMModel, error) {
	return []LLMModel{"gpt-4o", "gpt-4o-mini", "gpt-4-turbo", "gpt-3.5-turbo"}, nil
}

func (s *OpenAILLMService) Provider() Provider { return ProviderOpenAI }

// --- Groq Implementation ---

const groqBaseURL = "https://api.groq.com/openai/v1"

type GroqLLMService struct {
	OpenAILLMService // Groq uses OpenAI compatible API
}

func (s *GroqLLMService) AnalyzeImage(ctx context.Context, imageData []byte, prompt string, model LLMModel, config APIConfig) (string, error) {
	return "", fmt.Errorf("Groq vision not yet supported")
}

func NewGroqLLMService(apiKey, baseURL string) *GroqLLMService {
	if baseURL == "" {
		baseURL = groqBaseURL
	}
	return &GroqLLMService{
		OpenAILLMService: *NewOpenAILLMService(apiKey, baseURL),
	}
}

func (s *GroqLLMService) GetAvailableModels(ctx context.Context) ([]LLMModel, error) {
	return []LLMModel{"llama3-70b-8192", "llama3-8b-8192", "mixtral-8x7b-32768", "gemma2-9b-it"}, nil
}

func (s *GroqLLMService) Provider() Provider { return ProviderGroq }

// --- Claude Implementation ---

const claudeBaseURL = "https://api.anthropic.com/v1"

type ClaudeLLMService struct {
	client  *http.Client
	apiKey  string
	baseURL string
}

func (s *ClaudeLLMService) AnalyzeImage(ctx context.Context, imageData []byte, prompt string, model LLMModel, config APIConfig) (string, error) {
	return "", fmt.Errorf("Claude vision not yet supported in this client")
}

func NewClaudeLLMService(apiKey, baseURL string) *ClaudeLLMService {
	if baseURL == "" {
		baseURL = claudeBaseURL
	}
	return &ClaudeLLMService{
		client:  &http.Client{Timeout: 300 * time.Second},
		apiKey:  apiKey,
		baseURL: baseURL,
	}
}

type claudeRequest struct {
	Model     string `json:"model"`
	MaxTokens int    `json:"max_tokens"`
	Messages  []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

func (s *ClaudeLLMService) GenerateResponse(ctx context.Context, prompt string, model LLMModel, config APIConfig) (string, error) {
	reqPayload := claudeRequest{
		Model:     string(model),
		MaxTokens: 1024,
		Messages: []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		}{
			{Role: "user", Content: prompt},
		},
	}

	reqBody, err := json.Marshal(reqPayload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/messages", s.baseURL), bytes.NewBuffer(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", s.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if len(result.Content) > 0 {
		return result.Content[0].Text, nil
	}
	return "", fmt.Errorf("no response from Claude")
}

// GenerateResponseMultimodal implements native vision for the Claude API.
// Claude uses a different content format than OpenAI — images are sent as
// {"type":"image","source":{"type":"base64","media_type":"image/jpeg","data":"..."}}
func (s *ClaudeLLMService) GenerateResponseMultimodal(ctx context.Context, messages []MultiModalMessage, model LLMModel, config APIConfig) (string, error) {
	type imageSource struct {
		Type      string `json:"type"`       // "base64"
		MediaType string `json:"media_type"` // "image/jpeg"
		Data      string `json:"data"`
	}
	type contentBlock struct {
		Type   string       `json:"type"`             // "text" or "image"
		Text   string       `json:"text,omitempty"`
		Source *imageSource `json:"source,omitempty"`
	}
	type claudeMsg struct {
		Role    string         `json:"role"`
		Content []contentBlock `json:"content"`
	}

	var systemText string
	var claudeMsgs []claudeMsg

	for _, mm := range messages {
		if mm.Role == "system" {
			// Claude takes system as a top-level string, not a message
			for _, p := range mm.Parts {
				systemText += p.Text
			}
			continue
		}
		var blocks []contentBlock
		for _, p := range mm.Parts {
			if p.Type == "text" {
				blocks = append(blocks, contentBlock{Type: "text", Text: p.Text})
			} else if p.Type == "image" && len(p.ImageData) > 0 {
				mimeType := p.MimeType
				if mimeType == "" {
					mimeType = "image/jpeg"
				}
				blocks = append(blocks, contentBlock{
					Type: "image",
					Source: &imageSource{
						Type:      "base64",
						MediaType: mimeType,
						Data:      base64.StdEncoding.EncodeToString(p.ImageData),
					},
				})
			}
		}
		if len(blocks) > 0 {
			claudeMsgs = append(claudeMsgs, claudeMsg{Role: mm.Role, Content: blocks})
		}
	}

	reqPayload := struct {
		Model     string      `json:"model"`
		MaxTokens int         `json:"max_tokens"`
		System    string      `json:"system,omitempty"`
		Messages  []claudeMsg `json:"messages"`
	}{
		Model:     string(model),
		MaxTokens: 2000,
		System:    systemText,
		Messages:  claudeMsgs,
	}

	reqBody, err := json.Marshal(reqPayload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/messages", s.baseURL), bytes.NewBuffer(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", s.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if len(result.Content) > 0 {
		return result.Content[0].Text, nil
	}
	return "", fmt.Errorf("no response from Claude multimodal")
}

func (s *ClaudeLLMService) GetAvailableModels(ctx context.Context) ([]LLMModel, error) {
	return []LLMModel{"claude-opus-4-6", "claude-sonnet-4-6", "claude-haiku-4-5-20251001"}, nil
}

func (s *ClaudeLLMService) Provider() Provider { return ProviderClaude }

// --- Ollama Implementation ---

const ollamaBaseURL = "http://127.0.0.1:11434/v1"

type OllamaLLMService struct {
	OpenAILLMService
}

func (s *OllamaLLMService) AnalyzeImage(ctx context.Context, imageData []byte, prompt string, model LLMModel, config APIConfig) (string, error) {
	return s.OpenAILLMService.AnalyzeImage(ctx, imageData, prompt, model, config)
}

func NewOllamaLLMService(baseURL string) *OllamaLLMService {
	if baseURL == "" {
		baseURL = ollamaBaseURL
	} else if !strings.HasSuffix(baseURL, "/v1") {
		baseURL = strings.TrimSuffix(baseURL, "/") + "/v1"
	}
	// Ollama needs no API key - pass "ollama" as placeholder
	return &OllamaLLMService{
		OpenAILLMService: *NewOpenAILLMService("ollama", baseURL),
	}
}

func (s *OllamaLLMService) GetAvailableModels(ctx context.Context) ([]LLMModel, error) {
	return []LLMModel{"llama3", "mistral", "phi3", "qwen2.5-coder:1.5b", "llava", "moondream"}, nil
}

func (s *OllamaLLMService) Provider() Provider { return ProviderOllama }

// DetectBestLLMProvider auto-detects the best available LLM provider.
func DetectBestLLMProvider(config APIConfig) Provider {
	// Try Ollama first
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://127.0.0.1:11434/api/tags")
	if err == nil && resp.StatusCode == 200 {
		return ProviderOllama
	}
	return ProviderAIHorde
}

// --- HuggingFace Implementation ---

const huggingfaceBaseURL = "https://api-inference.huggingface.co/v1"

type HuggingFaceLLMService struct {
	OpenAILLMService
}

func (s *HuggingFaceLLMService) AnalyzeImage(ctx context.Context, imageData []byte, prompt string, model LLMModel, config APIConfig) (string, error) {
	return s.OpenAILLMService.AnalyzeImage(ctx, imageData, prompt, model, config)
}

func NewHuggingFaceLLMService(apiKey, baseURL string) *HuggingFaceLLMService {
	if baseURL == "" {
		baseURL = huggingfaceBaseURL
	}
	return &HuggingFaceLLMService{
		OpenAILLMService: *NewOpenAILLMService(apiKey, baseURL),
	}
}

func (s *HuggingFaceLLMService) GetAvailableModels(ctx context.Context) ([]LLMModel, error) {
	return []LLMModel{"mistralai/Mixtral-8x7B-Instruct-v0.1", "HuggingFaceH4/zephyr-7b-beta", "meta-llama/Meta-Llama-3-8B-Instruct"}, nil
}

func (s *HuggingFaceLLMService) Provider() Provider { return ProviderHuggingFace }

// --- Pollinations Implementation ---

const pollinationsTextBaseURL = "https://enter.pollinations.ai/v1"
const pollinationsImageBaseURL = "https://enter.pollinations.ai"

type PollinationsLLMService struct {
	OpenAILLMService
}

func (s *PollinationsLLMService) AnalyzeImage(ctx context.Context, imageData []byte, prompt string, model LLMModel, config APIConfig) (string, error) {
	return s.OpenAILLMService.AnalyzeImage(ctx, imageData, prompt, model, config)
}

func NewPollinationsLLMService(apiKey, baseURL string) *PollinationsLLMService {
	if baseURL == "" {
		baseURL = pollinationsTextBaseURL
	}
	return &PollinationsLLMService{
		OpenAILLMService: *NewOpenAILLMService(apiKey, baseURL),
	}
}

func (s *PollinationsLLMService) GetAvailableModels(ctx context.Context) ([]LLMModel, error) {
	if s.apiKey == "" {
		return []LLMModel{"openai", "openai-fast", "mistral", "llama", "deepseek", "claude"}, nil
	}
	endpoint := "https://enter.pollinations.ai/v1/models"
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	
	resp, err := s.client.Do(req)
	if err != nil {
		// Fall back to known models if API unreachable
		return []LLMModel{"openai", "openai-fast", "openai-large", "qwen-coder", "mistral", "llama", "deepseek", "gemini", "claude"}, nil
	}
	defer resp.Body.Close()

	// Response: plain array [{"id":"...","object":"model",...}] in OpenAI format
	var modelsResponse struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&modelsResponse); err != nil || len(modelsResponse.Data) == 0 {
		return []LLMModel{"openai", "openai-large", "mistral", "llama", "deepseek", "claude"}, nil
	}
	result := make([]LLMModel, 0, len(modelsResponse.Data))
	for _, m := range modelsResponse.Data {
		if m.ID != "" {
			result = append(result, LLMModel(m.ID))
		}
	}
	return result, nil
}

func (s *PollinationsLLMService) Provider() Provider { return ProviderPollinations }

type PollinationsImageService struct {
	client  *http.Client
	apiKey  string
	baseURL string
}

func NewPollinationsImageService(apiKey, baseURL string) *PollinationsImageService {
	if baseURL == "" {
		baseURL = pollinationsImageBaseURL
	}
	return &PollinationsImageService{
		client:  &http.Client{Timeout: 60 * time.Second},
		apiKey:  apiKey,
		baseURL: baseURL,
	}
}

func (s *PollinationsImageService) GenerateImage(ctx context.Context, prompt string, model ImageModel, config APIConfig) (string, error) {
	// Use the OpenAI-compatible POST endpoint for Pollinations image generation
	// documentation: https://enter.pollinations.ai/api/docs
	
	width := config.ImageStyle.Width
	height := config.ImageStyle.Height
	if width == 0 { width = 1024 }
	if height == 0 { height = 1024 }

	reqPayload := struct {
		Prompt  string `json:"prompt"`
		Model   string `json:"model"`
		Size    string `json:"size"`
		Seed    int64  `json:"seed"`
		NoLogo  bool   `json:"nologo"`
		Format  string `json:"response_format"`
	}{
		Prompt:  prompt,
		Model:   string(model),
		Size:    fmt.Sprintf("%dx%d", width, height),
		Seed:    time.Now().Unix() % 1000000,
		NoLogo:  true,
		Format:  "b64_json",
	}

	if reqPayload.Model == "" {
		reqPayload.Model = "flux"
	}

	reqBody, err := json.Marshal(reqPayload)
	if err != nil {
		return "", err
	}

	// Important: Use the /v1/images/generations path
	endpoint := s.baseURL + "/v1/images/generations"
	// if baseURL already has /v1, don't double it
	if strings.HasSuffix(s.baseURL, "/v1") {
		endpoint = s.baseURL + "/images/generations"
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(reqBody))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	if s.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.apiKey)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Pollinations Image API error %d: %.300s", resp.StatusCode, string(body))
	}

	// Parse the OpenAI-compatible response
	var result struct {
		Data []struct {
			B64 string `json:"b64_json"`
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to decode Pollinations image response: %w", err)
	}

	if len(result.Data) > 0 {
		if result.Data[0].B64 != "" {
			return result.Data[0].B64, nil
		}
		// If they returned a URL instead, download it
		if result.Data[0].URL != "" {
			imgResp, err := http.Get(result.Data[0].URL)
			if err != nil { return "", err }
			defer imgResp.Body.Close()
			imgBytes, _ := io.ReadAll(imgResp.Body)
			return base64.StdEncoding.EncodeToString(imgBytes), nil
		}
	}

	return "", fmt.Errorf("no image returned from Pollinations")
}

func (s *PollinationsImageService) GetAvailableModels(ctx context.Context) ([]ImageModel, error) {
	if s.apiKey == "" {
		return []ImageModel{"flux", "zimage", "turbo", "gptimage"}, nil
	}
	endpoint := "https://enter.pollinations.ai/image/models"
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.apiKey)
	
	resp, err := s.client.Do(req)
	if err != nil {
		// Fall back to known models if API unreachable
		return []ImageModel{"flux", "zimage", "gptimage", "wan-image", "klein", "flux-pro"}, nil
	}
	defer resp.Body.Close()

	// Response is an array of model objects with at least a "name" field
	var models []struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&models); err != nil {
		return []ImageModel{"flux", "zimage", "turbo", "flux-realism", "flux-anime", "flux-3d", "flux-pro"}, nil
	}
	result := make([]ImageModel, 0, len(models))
	for _, m := range models {
		if m.Name != "" {
			result = append(result, ImageModel(m.Name))
		}
	}
	if len(result) == 0 {
		return []ImageModel{"flux", "zimage", "turbo", "flux-realism", "flux-anime", "flux-3d", "flux-pro"}, nil
	}
	return result, nil
}

func (s *PollinationsImageService) Provider() Provider { return ProviderPollinations }

// --- OpenAI Image Implementation ---

const openaiImageBaseURL = "https://api.openai.com/v1"

type OpenAIImageService struct {
	client  *http.Client
	apiKey  string
	baseURL string
}

func NewOpenAIImageService(apiKey, baseURL string) *OpenAIImageService {
	if baseURL == "" {
		baseURL = openaiImageBaseURL
	}
	return &OpenAIImageService{
		client:  &http.Client{Timeout: 60 * time.Second},
		apiKey:  apiKey,
		baseURL: baseURL,
	}
}

type openaiImageRequest struct {
	Prompt string `json:"prompt"`
	Model  string `json:"model,omitempty"`
	N      int    `json:"n,omitempty"`
	Size   string `json:"size,omitempty"`
}

type openaiImageResponse struct {
	Created int64 `json:"created"`
	Data    []struct {
		URL string `json:"url"`
	} `json:"data"`
}

func (s *OpenAIImageService) GenerateImage(ctx context.Context, prompt string, model ImageModel, config APIConfig) (string, error) {
	size := fmt.Sprintf("%dx%d", config.ImageStyle.Width, config.ImageStyle.Height)
	if config.ImageStyle.Width == 0 || config.ImageStyle.Height == 0 {
		size = "1024x1024"
	}

	reqPayload := openaiImageRequest{
		Prompt: prompt,
		Model:  string(model),
		N:      1,
		Size:   size,
	}

	reqBody, err := json.Marshal(reqPayload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/images/generations", s.baseURL), bytes.NewBuffer(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("OpenAI Image API error: Status %d, Response: %s", resp.StatusCode, string(bodyBytes))
	}

	var result openaiImageResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if len(result.Data) > 0 {
		imageURL := result.Data[0].URL
		// Download the image and return as base64
		imgResp, err := http.Get(imageURL)
		if err != nil {
			return "", fmt.Errorf("failed to download image: %w", err)
		}
		defer imgResp.Body.Close()

		imgBytes, err := io.ReadAll(imgResp.Body)
		if err != nil {
			return "", err
		}
		return base64.StdEncoding.EncodeToString(imgBytes), nil
	}
	return "", fmt.Errorf("no image returned from OpenAI")
}

func (s *OpenAIImageService) GetAvailableModels(ctx context.Context) ([]ImageModel, error) {
	return []ImageModel{"dall-e-3", "dall-e-2"}, nil
}

func (s *OpenAIImageService) Provider() Provider { return ProviderOpenAI }

// --- ImageRouter Implementation ---

const imagerouterBaseURL = "https://ir.api.together.xyz/v1"

type ImageRouterImageService struct {
	OpenAIImageService
}

func NewImageRouterImageService(apiKey, baseURL string) *ImageRouterImageService {
	if baseURL == "" {
		baseURL = imagerouterBaseURL
	}
	return &ImageRouterImageService{
		OpenAIImageService: *NewOpenAIImageService(apiKey, baseURL),
	}
}

func (s *ImageRouterImageService) GetAvailableModels(ctx context.Context) ([]ImageModel, error) {
	return []ImageModel{"black-forest-labs/FLUX.1-schnell-Free", "black-forest-labs/FLUX.1-dev"}, nil
}

func (s *ImageRouterImageService) Provider() Provider { return ProviderImageRouter }

// --- OpenRouter Implementation ---

type OpenRouterLLMService struct {
	OpenAILLMService
}

func NewOpenRouterLLMService(apiKey, baseURL string) *OpenRouterLLMService {
	if baseURL == "" {
		baseURL = "https://openrouter.ai/api/v1"
	}
	return &OpenRouterLLMService{
		OpenAILLMService: *NewOpenAILLMService(apiKey, baseURL),
	}
}

func (s *OpenRouterLLMService) GetAvailableModels(ctx context.Context) ([]LLMModel, error) {
	return []LLMModel{"mistralai/mistral-7b-instruct:free", "meta-llama/llama-3-8b-instruct:free", "google/gemma-3-27b-it:free", "deepseek/deepseek-r1:free"}, nil
}

func (s *OpenRouterLLMService) Provider() Provider { return ProviderOpenRouter }

// --- Gemini Implementation ---

type GeminiLLMService struct {
	OpenAILLMService
}

func NewGeminiLLMService(apiKey, baseURL string) *GeminiLLMService {
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com/v1beta/openai"
	}
	return &GeminiLLMService{
		OpenAILLMService: *NewOpenAILLMService(apiKey, baseURL),
	}
}

func (s *GeminiLLMService) GetAvailableModels(ctx context.Context) ([]LLMModel, error) {
	return []LLMModel{"gemini-2.0-flash", "gemini-1.5-pro", "gemini-1.5-flash", "gemini-2.5-pro-preview-03-25"}, nil
}

func (s *GeminiLLMService) Provider() Provider { return ProviderGemini }

// --- Factory Functions ---

// LLMFactory creates an LLMService based on the provider and API configuration.
func LLMFactory(p Provider, config APIConfig) (LLMService, error) {
	switch p {
	case ProviderMock:
		return &MockLLMService{}, nil
	case ProviderAIHorde:
		return NewAIHordeLLMService(config.AIHorde.APIKey, config.AIHorde.BaseURL), nil
	case ProviderMistral:
		return NewMistralLLMService(config.Mistral.APIKey, config.Mistral.BaseURL), nil
	case ProviderOllama:
		return NewOllamaLLMService(config.Ollama.BaseURL), nil
	case ProviderGroq:
		return NewGroqLLMService(config.Groq.APIKey, config.Groq.BaseURL), nil
	case ProviderClaude:
		return NewClaudeLLMService(config.Claude.APIKey, config.Claude.BaseURL), nil
	case ProviderOpenAI:
		return NewOpenAILLMService(config.OpenAI.APIKey, config.OpenAI.BaseURL), nil
	case ProviderHuggingFace:
		return NewHuggingFaceLLMService(config.HuggingFace.APIKey, config.HuggingFace.BaseURL), nil
	case ProviderPollinations:
		return NewPollinationsLLMService(config.Pollinations.APIKey, config.Pollinations.BaseURL), nil
	case ProviderOpenRouter:
		return NewOpenRouterLLMService(config.OpenRouter.APIKey, config.OpenRouter.BaseURL), nil
	case ProviderGemini:
		return NewGeminiLLMService(config.Gemini.APIKey, config.Gemini.BaseURL), nil
	case ProviderNexusRouter:
		// nexus-ai-router: local OpenAI-compat fleet manager (localhost:8810)
		// Handles its own provider failover (Ollama → Pollinations → cloud)
		endpoint := "http://localhost:8810/v1"
		return NewOpenAILLMService("nexus", endpoint), nil
	default:
		// Check custom APIs
		for _, custom := range config.CustomAPIs {
			if custom.Name == string(p) && custom.Type == "llm" {
				return NewOpenAILLMService(custom.APIKey, custom.Endpoint), nil
			}
		}
		// Default fallback to Pollinations for maximum "Military Grade" up-time
		return NewPollinationsLLMService(config.Pollinations.APIKey, config.Pollinations.BaseURL), nil
	}
}

// ImageFactory creates an ImageService based on the provider and API configuration.
func ImageFactory(p Provider, config APIConfig) (ImageService, error) {
	switch p {
	case ProviderMock:
		return &MockImageService{}, nil
	case ProviderAIHorde:
		return NewAIHordeImageService(config.AIHorde.APIKey, config.AIHorde.BaseURL), nil
	case ProviderPollinations:
		return NewPollinationsImageService(config.Pollinations.APIKey, config.Pollinations.ImageBaseURL), nil
	case ProviderOpenAI:
		return NewOpenAIImageService(config.OpenAI.APIKey, config.OpenAI.BaseURL), nil
	case ProviderImageRouter:
		return NewImageRouterImageService(config.ImageRouter.APIKey, config.ImageRouter.BaseURL), nil
	default:
		// Check custom APIs
		for _, custom := range config.CustomAPIs {
			if custom.Name == string(p) && custom.Type == "image" {
				return NewOpenAIImageService(custom.APIKey, custom.Endpoint), nil
			}
		}
		return nil, fmt.Errorf("unsupported Image provider: %s", p)
	}
}

// EmbeddingFactory creates an EmbeddingService.
func EmbeddingFactory(p Provider, config APIConfig) (EmbeddingService, error) {
	switch p {
	case ProviderLocal:
		return &LocalEmbeddingService{}, nil
	case ProviderMock:
		return &LocalEmbeddingService{}, nil // legacy compat only
	case ProviderOpenAI:
		return NewOpenAIEmbeddingService(config.OpenAI.APIKey, config.OpenAI.BaseURL), nil
	// TODO: Add cases for other real embedding providers
	default:
		return &LocalEmbeddingService{}, nil // Always fallback to local for free RAG
	}
}

// LLMSummarizer implements Summarizer by delegating to an LLMService.
type LLMSummarizer struct {
	llmService LLMService
	llmModel   LLMModel
	provider   Provider
	config     APIConfig
}

// NewLLMSummarizer creates a new LLMSummarizer.
func NewLLMSummarizer(llmService LLMService, llmModel LLMModel, config APIConfig) *LLMSummarizer {
	return &LLMSummarizer{
		llmService: llmService,
		llmModel:   llmModel,
		provider:   llmService.Provider(),
		config:     config,
	}
}

func (s *LLMSummarizer) Summarize(ctx context.Context, text string) (string, error) {
	summary, err := s.llmService.GenerateResponse(ctx, text, s.llmModel, s.config)
	if err != nil {
		return "", fmt.Errorf("LLM summarization failed: %w", err)
	}
	return summary, nil
}

func (s *LLMSummarizer) Provider() Provider {
	return s.provider
}

// SummarizerFactory creates a Summarizer using whatever provider is configured.
// Falls back to Pollinations if the requested provider fails.
func SummarizerFactory(p Provider, config APIConfig) (Summarizer, error) {
	if p == ProviderMock {
		return &MockSummarizer{}, nil
	}

	llmService, err := LLMFactory(p, config)
	if err != nil {
		// Fallback: try Pollinations (no key required)
		llmService, err = LLMFactory(ProviderPollinations, config)
		if err != nil {
			return nil, fmt.Errorf("summarizer: no usable LLM provider: %w", err)
		}
		p = ProviderPollinations
	}

	pcfg, ok := config.GetLLMProviderConfig(p)
	llmModel := LLMModel("")
	if ok && pcfg.DefaultLLMModel != "" {
		llmModel = pcfg.DefaultLLMModel
	}
	if llmModel == "" {
		if def, ok := hardcodedDefaults[p]; ok {
			llmModel = LLMModel(def)
		}
	}

	return NewLLMSummarizer(llmService, llmModel, config), nil
}