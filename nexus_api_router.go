package main

// NexusApiRouter — Automagical self-healing LLM routing
//
// Ported from NeXuS SmartRouter (src/proxy/smart_router.py).
// Same philosophy: "If blocked at one provider, auto-route to another."
//
// Features:
//   - Per-provider health tracking (success/failure/latency)
//   - Score-based routing: priority + latency + (failures × 100)
//   - Automatic failover on 429, 503, missing key, or timeout
//   - Separate chains for Text, Vision, and RAG tasks
//   - Self-healing: failed providers re-enter rotation after backoff

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ProviderHealth mirrors NetworkHealth from smart_router.py — tracks the
// live health of a single LLM provider endpoint.
type ProviderHealth struct {
	provider     Provider
	priority     int // lower = preferred (local=10, free=20, paid=50)
	failureCount int
	successCount int
	latencyMs    float64
	isWorking    bool
	lastCheck    time.Time
	lastError    string
}

func (h *ProviderHealth) markSuccess(latencyMs float64) {
	h.isWorking = true
	h.successCount++
	h.failureCount = maxInt(0, h.failureCount-1)
	h.latencyMs = latencyMs
	h.lastCheck = time.Now()
	h.lastError = ""
}

func (h *ProviderHealth) markFailure(err error) {
	h.failureCount++
	h.lastCheck = time.Now()
	if err != nil {
		h.lastError = err.Error()
	}
	if h.failureCount >= 3 {
		h.isWorking = false
	}
}

// score returns a routing score — lower is better.
// Matches smart_router.py: priority + latency/100 + failure_count*100
func (h *ProviderHealth) score() float64 {
	if !h.isWorking {
		return 999999
	}
	return float64(h.priority) + h.latencyMs/100 + float64(h.failureCount)*100
}

// routeEntry pairs a live LLMService with its health tracker.
type routeEntry struct {
	service LLMService
	health  *ProviderHealth
}

// NexusApiRouter routes LLM requests across multiple providers with
// automatic failover and health-based ordering.
//
// It satisfies both LLMService and MultiModalLLMService, so it can
// directly replace the ChimeraLLMService as the unified service.
type NexusApiRouter struct {
	textChain   []*routeEntry
	visionChain []*routeEntry
	ragChain    []*routeEntry
	mu          sync.RWMutex
}

// NewNexusApiRouter builds a router from the current APIConfig.
// It populates chains for each task type from all enabled providers,
// ordered by cost tier (local < free < paid).
func NewNexusApiRouter(cfg *APIConfig) *NexusApiRouter {
	r := &NexusApiRouter{}

	// Priority tiers — matches NeXuS network priorities
	const (
		PriorityLocal   = 10 // Ollama: fastest, free, private
		PriorityFree    = 20 // Pollinations, Gemini free, AI Horde
		PriorityPaid    = 50 // OpenAI, Claude, Mistral
	)

	type providerDef struct {
		provider   Provider
		priority   int
		hasVision  bool
		isRAGAble  bool
	}

	defs := []providerDef{
		{ProviderOllama,       PriorityLocal, true,  true},
		{ProviderPollinations, PriorityFree,  true,  true}, // openai model = GPT-4o multimodal
		{ProviderGemini,       PriorityFree,  true,  true},
		{ProviderAIHorde,      PriorityFree,  false, true},
		{ProviderGroq,         PriorityFree,  false, true},
		{ProviderMistral,      PriorityPaid,  false, true},
		{ProviderOpenAI,       PriorityPaid,  true,  true},
		{ProviderOpenRouter,   PriorityPaid,  true,  true},
		{ProviderClaude,       PriorityPaid,  true,  true},
		{ProviderHuggingFace,  PriorityPaid,  false, true},
	}

	for _, def := range defs {
		pcfg, ok := cfg.GetLLMProviderConfig(def.provider)
		if !ok || !pcfg.Enabled {
			continue
		}
		// Skip providers with no API key (except local and free ones)
		if def.priority >= PriorityPaid && pcfg.APIKey == "" {
			continue
		}

		svc, err := LLMFactory(def.provider, *cfg)
		if err != nil {
			NexusLog("WARN", "router", fmt.Sprintf("skipping %s: %v", def.provider, err))
			continue
		}

		health := &ProviderHealth{
			provider:  def.provider,
			priority:  def.priority,
			isWorking: true,
		}
		entry := &routeEntry{service: svc, health: health}

		// Every provider goes in the text chain
		r.textChain = append(r.textChain, entry)

		// Vision-capable providers go in the vision chain
		if def.hasVision {
			r.visionChain = append(r.visionChain, entry)
		}

		// RAG chain (all capable providers, prefer cheap/fast)
		if def.isRAGAble {
			r.ragChain = append(r.ragChain, entry)
		}
	}

	// Sort each chain by priority (the health score handles dynamic reordering)
	sortChain(r.textChain)
	sortChain(r.visionChain)
	sortChain(r.ragChain)

	// Guarantee vision chain is never empty — add Pollinations as last-resort fallback
	// Pollinations openai endpoint accepts image_url messages; partial vision beats none
	if len(r.visionChain) == 0 && len(r.textChain) > 0 {
		fallback := r.textChain[len(r.textChain)-1]
		r.visionChain = append(r.visionChain, fallback)
		NexusLog("WARN", "router", "no dedicated vision provider — using text fallback for vision")
	}

	NexusLog("INFO", "router", fmt.Sprintf(
		"initialized — text:%d vision:%d rag:%d providers",
		len(r.textChain), len(r.visionChain), len(r.ragChain),
	))
	return r
}

func sortChain(chain []*routeEntry) {
	for i := 1; i < len(chain); i++ {
		for j := i; j > 0 && chain[j].health.priority < chain[j-1].health.priority; j-- {
			chain[j], chain[j-1] = chain[j-1], chain[j]
		}
	}
}

// orderedChain returns the chain sorted by current health score (live ordering).
func (r *NexusApiRouter) orderedChain(chain []*routeEntry) []*routeEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*routeEntry, len(chain))
	copy(out, chain)
	// Insertion sort by score (chains are short, usually ≤10 entries)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].health.score() < out[j-1].health.score(); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// tryChain executes fn against each entry in order, failing over on errors.
// Retryable errors: rate limit (429), server error (5xx), timeout.
func (r *NexusApiRouter) tryChain(chain []*routeEntry, fn func(*routeEntry) (string, error)) (string, error) {
	ordered := r.orderedChain(chain)
	var lastErr error
	for _, entry := range ordered {
		start := time.Now()
		result, err := fn(entry)
		latency := float64(time.Since(start).Milliseconds())
		if err == nil {
			r.mu.Lock()
			entry.health.markSuccess(latency)
			r.mu.Unlock()
			return result, nil
		}
		// Decide if this error is retryable (switch providers) or fatal (return immediately)
		if !isRetryableError(err) {
			return "", err
		}
		r.mu.Lock()
		entry.health.markFailure(err)
		r.mu.Unlock()
		NexusLog("WARN", "router", fmt.Sprintf(
			"provider %s failed (score %.0f): %v — trying next",
			entry.health.provider, entry.health.score(), err,
		))
		lastErr = err
	}
	return "", fmt.Errorf("all providers exhausted: %w", lastErr)
}

// isRetryableError returns true for errors that warrant trying another provider.
// Context cancellation (manual) and invalid API keys are NOT retryable.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	// Check for standard context deadline exceeded error
	if err == context.DeadlineExceeded {
		return true
	}

	msg := strings.ToLower(err.Error())
	// Non-retryable: manual context cancel, bad request, auth failures
	if strings.Contains(msg, "context canceled") ||
		strings.Contains(msg, "invalid api key") ||
		strings.Contains(msg, "400") {
		return false
	}
	// Retryable: rate limit, server error, timeout, network issues
	return strings.Contains(msg, "429") ||
		strings.Contains(msg, "503") ||
		strings.Contains(msg, "502") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "deadline exceeded") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "no response") ||
		strings.Contains(msg, "all providers")
}

// ─── LLMService implementation ─────────────────────────────────────────────

func (r *NexusApiRouter) GenerateResponse(ctx context.Context, prompt string, model LLMModel, config APIConfig) (string, error) {
	return r.tryChain(r.textChain, func(e *routeEntry) (string, error) {
		// Use the provider's own default model if caller passes empty model
		m := resolveModel(model, e.service, &config, false)
		return e.service.GenerateResponse(ctx, prompt, m, config)
	})
}

func (r *NexusApiRouter) AnalyzeImage(ctx context.Context, imageData []byte, prompt string, model LLMModel, config APIConfig) (string, error) {
	return r.tryChain(r.visionChain, func(e *routeEntry) (string, error) {
		m := resolveModel(model, e.service, &config, true)
		return e.service.AnalyzeImage(ctx, imageData, prompt, m, config)
	})
}

func (r *NexusApiRouter) GetAvailableModels(ctx context.Context) ([]LLMModel, error) {
	if len(r.textChain) == 0 {
		return nil, fmt.Errorf("no text providers configured")
	}
	return r.textChain[0].service.GetAvailableModels(ctx)
}

func (r *NexusApiRouter) Provider() Provider {
	if len(r.textChain) == 0 {
		return ProviderPollinations
	}
	return r.textChain[0].service.Provider()
}

// ─── MultiModalLLMService implementation ───────────────────────────────────

func (r *NexusApiRouter) GenerateResponseMultimodal(ctx context.Context, messages []MultiModalMessage, model LLMModel, config APIConfig) (string, error) {
	return r.tryChain(r.visionChain, func(e *routeEntry) (string, error) {
		mmSvc, ok := e.service.(MultiModalLLMService)
		if !ok {
			return "", fmt.Errorf("provider %s does not support multimodal", e.health.provider)
		}
		m := resolveModel(model, e.service, &config, true)
		return mmSvc.GenerateResponseMultimodal(ctx, messages, m, config)
	})
}

// ─── RAG-specific routing ───────────────────────────────────────────────────

// GenerateRAGResponse routes to the RAG chain — prefer cheap/fast models
// for memory summarization and lore indexing.
func (r *NexusApiRouter) GenerateRAGResponse(ctx context.Context, prompt string, config APIConfig) (string, error) {
	return r.tryChain(r.ragChain, func(e *routeEntry) (string, error) {
		m := resolveModel("", e.service, &config, false)
		return e.service.GenerateResponse(ctx, prompt, m, config)
	})
}

// ─── Status / diagnostics ──────────────────────────────────────────────────

// HealthReport returns a human-readable string of all provider health states.
func (r *NexusApiRouter) HealthReport() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	seen := map[Provider]bool{}
	var sb strings.Builder
	for _, entry := range r.textChain {
		if seen[entry.health.provider] {
			continue
		}
		seen[entry.health.provider] = true
		status := "✅"
		if !entry.health.isWorking {
			status = "🔴"
		} else if entry.health.failureCount > 0 {
			status = "⚠️"
		}
		sb.WriteString(fmt.Sprintf("%s %-14s score:%.0f lat:%.0fms fail:%d\n",
			status, entry.health.provider,
			entry.health.score(), entry.health.latencyMs, entry.health.failureCount,
		))
	}
	return sb.String()
}

// ─── Helpers ───────────────────────────────────────────────────────────────

// resolveModel returns the best model for the given entry and task type.
// If the caller passed a non-empty model, that takes precedence.
func resolveModel(requested LLMModel, svc LLMService, cfg *APIConfig, forVision bool) LLMModel {
	if requested != "" {
		return requested
	}
	if forVision {
		return getVisionModel(cfg)
	}
	pcfg, ok := cfg.GetLLMProviderConfig(svc.Provider())
	if ok && pcfg.DefaultLLMModel != "" {
		return pcfg.DefaultLLMModel
	}
	return ""
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
