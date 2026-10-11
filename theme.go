package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// NexusTheme holds the color palette for the TUI.
type NexusTheme struct {
	Name string

	// Core palette
	Primary    lipgloss.Color // main accent — titles, selected items
	Secondary  lipgloss.Color // secondary accent — borders, highlights
	Accent     lipgloss.Color // pop color — warnings, badges
	Muted      lipgloss.Color // dimmed text — help lines, timestamps
	Text       lipgloss.Color // normal body text
	Background lipgloss.Color // panel background (empty string = terminal default)

	// Semantic
	Success lipgloss.Color
	Warning lipgloss.Color
	Error   lipgloss.Color
	Info    lipgloss.Color

	// Specific roles
	UserMsg      lipgloss.Color // user chat bubble
	CharMsg      lipgloss.Color // character chat bubble
	SystemMsg    lipgloss.Color // system/narrator line
	Border       lipgloss.Color // standard border
	BorderFocus  lipgloss.Color // focused/active border
	InputCursor  lipgloss.Color // text input cursor

	// Visual Effects
	Glow bool // Toggle neon "glow" (bold/bright effects)
}

// Themes ─────────────────────────────────────────────────────────────────────

var ThemeDark = NexusTheme{
	Name:       "dark",
	Primary:    lipgloss.Color("#BD93F9"), // dracula purple
	Secondary:  lipgloss.Color("#8BE9FD"), // cyan
	Accent:     lipgloss.Color("#FFB86C"), // orange
	Muted:      lipgloss.Color("#6272A4"), // comment grey
	Text:       lipgloss.Color("#F8F8F2"), // foreground
	Background: lipgloss.Color(""),       // terminal default

	Success: lipgloss.Color("#50FA7B"),
	Warning: lipgloss.Color("#FFD700"),
	Error:   lipgloss.Color("#FF5555"),
	Info:    lipgloss.Color("#8BE9FD"),

	UserMsg:     lipgloss.Color("#50FA7B"),
	CharMsg:     lipgloss.Color("#BD93F9"),
	SystemMsg:   lipgloss.Color("#6272A4"),
	Border:      lipgloss.Color("#44475A"),
	BorderFocus: lipgloss.Color("#BD93F9"),
	InputCursor: lipgloss.Color("#FFB86C"),
	Glow:        false,
}

var ThemeFire = NexusTheme{
	Name:       "fire",
	Primary:    lipgloss.Color("#FF4500"), // OrangeRed
	Secondary:  lipgloss.Color("#FFD700"), // Gold
	Accent:     lipgloss.Color("#FF0000"), // Red
	Muted:      lipgloss.Color("#800000"), // Maroon
	Text:       lipgloss.Color("#FFF5EE"), // Seashell
	Background: lipgloss.Color(""),

	Success: lipgloss.Color("#ADFF2F"), // GreenYellow
	Warning: lipgloss.Color("#FFA500"), // Orange
	Error:   lipgloss.Color("#FF4500"),
	Info:    lipgloss.Color("#FF8C00"), // DarkOrange

	UserMsg:     lipgloss.Color("#ADFF2F"),
	CharMsg:     lipgloss.Color("#FF4500"),
	SystemMsg:   lipgloss.Color("#FFD700"),
	Border:      lipgloss.Color("#800000"),
	BorderFocus: lipgloss.Color("#FF4500"),
	InputCursor: lipgloss.Color("#FFD700"),
	Glow:        true,
}

var ThemeMatrix = NexusTheme{
	Name:       "matrix",
	Primary:    lipgloss.Color("#00FF41"), // Matrix Green
	Secondary:  lipgloss.Color("#008F11"), // Dark Matrix Green
	Accent:     lipgloss.Color("#D1FFBD"), // Pale Green
	Muted:      lipgloss.Color("#003B00"), // Deep Forest
	Text:       lipgloss.Color("#00FF41"),
	Background: lipgloss.Color("#000000"), // True Black

	Success: lipgloss.Color("#00FF41"),
	Warning: lipgloss.Color("#008F11"),
	Error:   lipgloss.Color("#FF0000"),
	Info:    lipgloss.Color("#00FF41"),

	UserMsg:     lipgloss.Color("#00FF41"),
	CharMsg:     lipgloss.Color("#00FF41"),
	SystemMsg:   lipgloss.Color("#008F11"),
	Border:      lipgloss.Color("#003B00"),
	BorderFocus: lipgloss.Color("#00FF41"),
	InputCursor: lipgloss.Color("#00FF41"),
	Glow:        true,
}

var ThemeTron = NexusTheme{
	Name:       "tron",
	Primary:    lipgloss.Color("#6FC3DF"), // Tron Blue
	Secondary:  lipgloss.Color("#00E5FF"), // Neon Cyan
	Accent:     lipgloss.Color("#FFFFFF"), // Pure White
	Muted:      lipgloss.Color("#1A2F38"), // Slate Grey
	Text:       lipgloss.Color("#DFEBF2"),
	Background: lipgloss.Color("#0D1921"), // Deep Grid Blue

	Success: lipgloss.Color("#00E5FF"),
	Warning: lipgloss.Color("#FFD700"),
	Error:   lipgloss.Color("#FF0060"), // Neon Pink
	Info:    lipgloss.Color("#6FC3DF"),

	UserMsg:     lipgloss.Color("#00E5FF"),
	CharMsg:     lipgloss.Color("#6FC3DF"),
	SystemMsg:   lipgloss.Color("#1A2F38"),
	Border:      lipgloss.Color("#1A2F38"),
	BorderFocus: lipgloss.Color("#00E5FF"),
	InputCursor: lipgloss.Color("#00E5FF"),
	Glow:        true,
}

// ActiveTheme is the currently loaded theme. Default: fire (NeXuS Standard).
var ActiveTheme = ThemeFire

// SetTheme switches the active theme by name.
func SetTheme(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "dark":
		ActiveTheme = ThemeDark
	case "fire":
		ActiveTheme = ThemeFire
	case "matrix":
		ActiveTheme = ThemeMatrix
	case "tron":
		ActiveTheme = ThemeTron
	default:
		return false
	}
	return true
}

// SetGlow toggles the neon glow effect on/off for the active theme.
func SetGlow(on bool) {
	ActiveTheme.Glow = on
}

// ─── Convenience style constructors ──────────────────────────────────────────

func T() NexusTheme { return ActiveTheme }

// TitleStyle renders a section title with optional glow.
func TitleStyle() lipgloss.Style {
	s := lipgloss.NewStyle().Foreground(ActiveTheme.Primary).Bold(true)
	if ActiveTheme.Glow {
		s = s.Italic(true).Underline(true) // Neon effect simulation
	}
	return s
}

func MutedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(ActiveTheme.Muted)
}

func ErrorStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(ActiveTheme.Error).Bold(true)
}

func SuccessStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(ActiveTheme.Success)
}

func WarningStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(ActiveTheme.Warning)
}

// BorderedBox wraps content in a rounded border with theme color.
func BorderedBox(focused bool) lipgloss.Style {
	color := ActiveTheme.Border
	if focused {
		color = ActiveTheme.BorderFocus
	}
	
	s := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(color).
		Padding(0, 1)
		
	if ActiveTheme.Glow && focused {
		s = s.Bold(true)
	}
	
	return s
}

// MenuStyle renders a numerical selection menu item.
func MenuStyle(selected bool) lipgloss.Style {
	if selected {
		s := lipgloss.NewStyle().
			Foreground(ActiveTheme.Background).
			Background(ActiveTheme.Primary).
			Bold(true).
			PaddingLeft(1)
		if ActiveTheme.Glow {
			s = s.Italic(true)
		}
		return s
	}
	return lipgloss.NewStyle().Foreground(ActiveTheme.Text).PaddingLeft(1)
}
