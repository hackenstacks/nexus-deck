package main

import "strings"

// Feature flags — controlled by .env or runtime toggles.
// Defaults are conservative: terminal off, search on, plugins on.
var (
	TerminalAccessEnabled bool = false
	SearchEnabled         bool = true
	PluginSystemEnabled   bool = true
	AnimationsEnabled     bool = true
)

// SetFeature toggles a named feature flag from a string value ("true"/"false"/"1"/"0").
func SetFeature(name, value string) {
	on := strings.ToLower(strings.TrimSpace(value))
	enabled := on == "true" || on == "1" || on == "yes"
	switch strings.ToUpper(name) {
	case "ENABLE_TERMINAL_ACCESS":
		TerminalAccessEnabled = enabled
		NexusLog("INFO", "features", "terminal_access="+value)
	case "ENABLE_GOOGLE_SEARCH":
		SearchEnabled = enabled
		NexusLog("INFO", "features", "search="+value)
	case "ENABLE_PLUGIN_SYSTEM":
		PluginSystemEnabled = enabled
		NexusLog("INFO", "features", "plugin_system="+value)
	case "ENABLE_ANIMATIONS":
		AnimationsEnabled = enabled
	}
}
