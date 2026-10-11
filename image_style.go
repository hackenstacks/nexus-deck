package main

import (
	"fmt"
)

// buildStyledImagePrompt combines a base prompt with style presets and negative prompts.
func buildStyledImagePrompt(base string, style ImageStyleConfig) string {
	prompt := base
	if style.StylePreset != "" {
		prompt = fmt.Sprintf("%s, %s", style.StylePreset, prompt)
	}
	if style.NegativePrompt != "" {
		prompt = fmt.Sprintf("%s | negative: %s", prompt, style.NegativePrompt)
	}
	return prompt
}
