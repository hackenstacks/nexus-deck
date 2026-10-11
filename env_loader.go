package main

import (
	"bufio"
	"os"
	"strings"
)

// loadDotEnv reads a .env file and returns a map of key→value.
// Skips blank lines and lines starting with #.
func loadDotEnv(path string) map[string]string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	env := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx < 1 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.Trim(strings.TrimSpace(line[idx+1:]), `"'`)
		// Strip inline comments
		if ci := strings.Index(val, " #"); ci >= 0 {
			val = strings.TrimSpace(val[:ci])
		}
		if key != "" {
			env[key] = val
		}
	}
	return env
}

// ApplyEnvToConfig reads .env from the current directory and applies
// any set values to the user's APIConfig, then saves it.
// Called once after login.
func ApplyEnvToConfig(user *User) {
	env := loadDotEnv(".env")
	if len(env) == 0 {
		return
	}

	cfg, err := LoadAPIConfig(user)
	if err != nil {
		cfg = NewAPIConfig()
	}

	has := func(key string) bool { v, ok := env[key]; return ok && v != "" }
	get := func(key string) string { return env[key] }

	// AI Horde
	if has("AI_HORDE_API_KEY") {
		cfg.AIHorde.APIKey = get("AI_HORDE_API_KEY")
		cfg.AIHorde.Enabled = true
	}

	// Pollinations
	if has("POLLINATIONS_AI_API_KEY") {
		cfg.Pollinations.APIKey = get("POLLINATIONS_AI_API_KEY")
		cfg.Pollinations.Enabled = true
		// unified gen endpoint — key now required for all generation
		cfg.Pollinations.TextBaseURL = "https://enter.pollinations.ai"
		cfg.Pollinations.BaseURL = "https://enter.pollinations.ai/v1"
		cfg.Pollinations.ImageBaseURL = "https://enter.pollinations.ai"
	}

	// OpenAI
	if has("OPENAI_API_KEY") {
		cfg.OpenAI.APIKey = get("OPENAI_API_KEY")
		cfg.OpenAI.Enabled = true
	}
	if has("OPENAI_ENDPOINT") {
		cfg.OpenAI.BaseURL = get("OPENAI_ENDPOINT")
	} else if has("OPENAI_API_ENDPOINT") {
		cfg.OpenAI.BaseURL = get("OPENAI_API_ENDPOINT")
	}

	// Anthropic Claude — key var is ANTHROPIC_API_KEY
	if has("ANTHROPIC_API_KEY") {
		cfg.Claude.APIKey = get("ANTHROPIC_API_KEY")
		cfg.Claude.Enabled = true
	}
	if has("ANTHROPIC_ENDPOINT") {
		cfg.Claude.BaseURL = get("ANTHROPIC_ENDPOINT")
	}

	// OpenRouter
	if has("OPENROUTER_API_KEY") {
		cfg.OpenRouter.APIKey = get("OPENROUTER_API_KEY")
		cfg.OpenRouter.Enabled = true
	}
	if has("OPENROUTER_ENDPOINT") {
		cfg.OpenRouter.BaseURL = get("OPENROUTER_ENDPOINT")
	} else if has("OPENROUTER_API_ENDPOINT") {
		cfg.OpenRouter.BaseURL = get("OPENROUTER_API_ENDPOINT")
	}

	// Groq
	if has("GROQ_API_KEY") {
		cfg.Groq.APIKey = get("GROQ_API_KEY")
		cfg.Groq.Enabled = true
	}
	if has("GROQ_ENDPOINT") {
		cfg.Groq.BaseURL = get("GROQ_ENDPOINT")
	} else if has("GROQ_API_ENDPOINT") {
		cfg.Groq.BaseURL = get("GROQ_API_ENDPOINT")
	}

	// Mistral
	if has("MISTRAL_API_KEY") {
		cfg.Mistral.APIKey = get("MISTRAL_API_KEY")
		cfg.Mistral.Enabled = true
	}
	if has("MISTRAL_ENDPOINT") {
		cfg.Mistral.BaseURL = get("MISTRAL_ENDPOINT")
	} else if has("MISTRAL_API_ENDPOINT") {
		cfg.Mistral.BaseURL = get("MISTRAL_API_ENDPOINT")
	}

	// Gemini
	if has("GEMINI_API_KEY") {
		cfg.Gemini.APIKey = get("GEMINI_API_KEY")
		cfg.Gemini.Enabled = true
	}
	if has("GEMINI_ENDPOINT") {
		cfg.Gemini.BaseURL = get("GEMINI_ENDPOINT")
	} else if has("GEMINI_API_ENDPOINT") {
		cfg.Gemini.BaseURL = get("GEMINI_API_ENDPOINT")
	}

	// Stability AI
	if has("STABILITY_AI_API_KEY") {
		cfg.Stability.APIKey = get("STABILITY_AI_API_KEY")
		cfg.Stability.Enabled = true
	}
	if has("STABILITY_ENDPOINT") {
		cfg.Stability.BaseURL = get("STABILITY_ENDPOINT")
	} else if has("STABILITY_API_ENDPOINT") {
		cfg.Stability.BaseURL = get("STABILITY_API_ENDPOINT")
	}

	// HuggingFace
	if has("HUGGINGFACE_API_KEY") {
		cfg.HuggingFace.APIKey = get("HUGGINGFACE_API_KEY")
		cfg.HuggingFace.Enabled = true
	}
	if has("HUGGINGFACE_ENDPOINT") {
		cfg.HuggingFace.BaseURL = get("HUGGINGFACE_ENDPOINT")
	} else if has("HUGGINGFACE_API_ENDPOINT") {
		cfg.HuggingFace.BaseURL = get("HUGGINGFACE_API_ENDPOINT")
	}

	// Ollama (local — always enabled if URL is set)
	if has("NEXUS_OLLAMA_URL") {
		cfg.Ollama.BaseURL = get("NEXUS_OLLAMA_URL")
		cfg.Ollama.Enabled = true
	}

	// Ollama model
	if has("NEXUS_OLLAMA_MODEL") {
		cfg.Ollama.DefaultLLMModel = LLMModel(get("NEXUS_OLLAMA_MODEL"))
	}

	// Default providers — only apply if user has not already chosen one in Settings.
	// This prevents .env from overriding a saved preference on every login.
	if has("DEFAULT_AI_SERVICE") && cfg.SelectedLLMProvider == "" {
		cfg.SelectedLLMProvider = Provider(get("DEFAULT_AI_SERVICE"))
	}
	if has("DEFAULT_IMAGE_SERVICE") && cfg.SelectedImageProvider == "" {
		cfg.SelectedImageProvider = Provider(get("DEFAULT_IMAGE_SERVICE"))
	}

	// Per-provider model defaults — set regardless of which provider is active.
	// This means every provider has a working model the moment you switch to it.
	if has("GEMINI_MODEL") {
		cfg.Gemini.DefaultLLMModel = LLMModel(get("GEMINI_MODEL"))
	}
	if has("MISTRAL_MODEL") {
		cfg.Mistral.DefaultLLMModel = LLMModel(get("MISTRAL_MODEL"))
	}
	if has("GROQ_MODEL") {
		cfg.Groq.DefaultLLMModel = LLMModel(get("GROQ_MODEL"))
	}
	if has("OPENAI_MODEL") {
		cfg.OpenAI.DefaultLLMModel = LLMModel(get("OPENAI_MODEL"))
	}
	if has("CLAUDE_MODEL") {
		cfg.Claude.DefaultLLMModel = LLMModel(get("CLAUDE_MODEL"))
	}
	if has("OLLAMA_MODEL") {
		cfg.Ollama.DefaultLLMModel = LLMModel(get("OLLAMA_MODEL"))
	}
	if has("OPENROUTER_MODEL") {
		cfg.OpenRouter.DefaultLLMModel = LLMModel(get("OPENROUTER_MODEL"))
	}
	if has("HUGGINGFACE_MODEL") {
		cfg.HuggingFace.DefaultLLMModel = LLMModel(get("HUGGINGFACE_MODEL"))
	}

	// NEXUS_LLM_MODEL overrides the model for whichever provider is currently selected.
	if has("NEXUS_LLM_MODEL") {
		model := LLMModel(get("NEXUS_LLM_MODEL"))
		switch cfg.SelectedLLMProvider {
		case ProviderMistral:
			cfg.Mistral.DefaultLLMModel = model
		case ProviderGroq:
			cfg.Groq.DefaultLLMModel = model
		case ProviderOpenAI:
			cfg.OpenAI.DefaultLLMModel = model
		case ProviderClaude:
			cfg.Claude.DefaultLLMModel = model
		case ProviderHuggingFace:
			cfg.HuggingFace.DefaultLLMModel = model
		case ProviderOllama:
			cfg.Ollama.DefaultLLMModel = model
		case ProviderOpenRouter:
			cfg.OpenRouter.DefaultLLMModel = model
		case ProviderGemini:
			cfg.Gemini.DefaultLLMModel = model
		}
	}

	// Default image model
	if has("NEXUS_IMAGE_MODEL") {
		model := ImageModel(get("NEXUS_IMAGE_MODEL"))
		switch cfg.SelectedImageProvider {
		case ProviderAIHorde:
			cfg.AIHorde.DefaultImageModel = model
		case ProviderPollinations:
			cfg.Pollinations.DefaultImageModel = model
		case ProviderOpenAI:
			cfg.OpenAI.DefaultImageModel = model
		case ProviderHuggingFace:
			cfg.HuggingFace.DefaultImageModel = model
		case ProviderStability:
			cfg.Stability.DefaultImageModel = model
		}
	}

	if err := SaveAPIConfig(user, cfg); err != nil {
		NexusLog("ERROR", "env", "failed to save config after .env apply: "+err.Error())
	} else {
		NexusLog("INFO", "env", ".env applied to config")
	}

	// Apply non-config env settings immediately
	if has("LOG_LEVEL") {
		SetLogLevel(get("LOG_LEVEL"))
		NexusLog("INFO", "env", "log level set to "+get("LOG_LEVEL"))
	}
	if has("DEVELOPMENT") && get("DEVELOPMENT") == "true" {
		SetLogLevel("debug")
		NexusLog("DEBUG", "env", "development mode enabled — log level forced to debug")
	}

	// Feature flags
	for _, key := range []string{
		"ENABLE_TERMINAL_ACCESS",
		"ENABLE_GOOGLE_SEARCH",
		"ENABLE_PLUGIN_SYSTEM",
		"ENABLE_ANIMATIONS",
	} {
		if has(key) {
			SetFeature(key, get(key))
		}
	}

	// Theme
	if has("DEFAULT_THEME") {
		SetTheme(get("DEFAULT_THEME"))
	}
}
