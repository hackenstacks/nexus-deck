package main

import (
	"context"
)

// ChimeraLLMService is a composite LLM service that routes requests to the
// specialist model best suited for the task:
//
//   - Text chat  → textSvc  (fast/local: Mistral, Ollama, Groq, Pollinations)
//   - Vision     → visionSvc (multimodal: Gemini, GPT-4V, Claude 3)
//   - RAG/memory → ragSvc   (cheap summarizer: any fast model)
//
// It implements both LLMService and MultiModalLLMService, so the rest of the
// codebase can treat it as a single unified service.
type ChimeraLLMService struct {
	textSvc   LLMService
	visionSvc LLMService
	ragSvc    LLMService
}

func NewChimeraLLMService(text, vision, rag LLMService) *ChimeraLLMService {
	return &ChimeraLLMService{
		textSvc:   text,
		visionSvc: vision,
		ragSvc:    rag,
	}
}

// GenerateResponse routes to the text LLM — standard chat generation.
func (c *ChimeraLLMService) GenerateResponse(ctx context.Context, prompt string, model LLMModel, config APIConfig) (string, error) {
	return c.textSvc.GenerateResponse(ctx, prompt, model, config)
}

// GenerateResponseMultimodal routes to the vision LLM — used when images are
// present in the conversation. Falls back to text LLM if vision doesn't support
// the MultiModalLLMService interface.
func (c *ChimeraLLMService) GenerateResponseMultimodal(ctx context.Context, messages []MultiModalMessage, model LLMModel, config APIConfig) (string, error) {
	if mmSvc, ok := c.visionSvc.(MultiModalLLMService); ok {
		// Use vision model with its configured default model, not the text model
		visionModel := getVisionModel(&config)
		if visionModel != "" {
			model = visionModel
		}
		return mmSvc.GenerateResponseMultimodal(ctx, messages, model, config)
	}
	// Vision service doesn't support multimodal — collapse to text
	var textParts []string
	for _, msg := range messages {
		for _, p := range msg.Parts {
			if p.Type == "text" && p.Text != "" {
				textParts = append(textParts, p.Text)
			}
		}
	}
	collapsed := ""
	for _, t := range textParts {
		collapsed += t + "\n"
	}
	return c.textSvc.GenerateResponse(ctx, collapsed, model, config)
}

// AnalyzeImage routes to the vision LLM for single-image analysis
// (used by /upload and /share commands).
func (c *ChimeraLLMService) AnalyzeImage(ctx context.Context, imageData []byte, prompt string, model LLMModel, config APIConfig) (string, error) {
	visionModel := getVisionModel(&config)
	if visionModel != "" {
		model = visionModel
	}
	return c.visionSvc.AnalyzeImage(ctx, imageData, prompt, model, config)
}

// GetAvailableModels returns models from the text LLM provider.
func (c *ChimeraLLMService) GetAvailableModels(ctx context.Context) ([]LLMModel, error) {
	return c.textSvc.GetAvailableModels(ctx)
}

// Provider returns the text LLM provider (the primary identity of the chimera).
func (c *ChimeraLLMService) Provider() Provider {
	return c.textSvc.Provider()
}

// RAGSummarize exposes the RAG LLM for memory summarization.
// Called by the memory manager when it needs to summarize context.
func (c *ChimeraLLMService) RAGSummarize(ctx context.Context, prompt string, model LLMModel, config APIConfig) (string, error) {
	return c.ragSvc.GenerateResponse(ctx, prompt, model, config)
}

// getVisionModel returns the default model string for the configured vision provider.
// No hardcoded model names — reads from the provider's own config.
func getVisionModel(cfg *APIConfig) LLMModel {
	if cfg == nil {
		return ""
	}
	pcfg, ok := cfg.GetLLMProviderConfig(cfg.SelectedVisionLLMProvider)
	if ok && pcfg.DefaultLLMModel != "" {
		return pcfg.DefaultLLMModel
	}
	// Fall through to text provider model if vision not configured
	pcfg, ok = cfg.GetLLMProviderConfig(cfg.SelectedLLMProvider)
	if ok && pcfg.DefaultLLMModel != "" {
		return pcfg.DefaultLLMModel
	}
	return ""
}
