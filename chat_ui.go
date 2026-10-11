package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func (m chatModel) View() string {
	if m.approvalModel != nil {
		return m.approvalModel.View()
	}
	if m.state == selectingCharacter {
		return m.renderCharacterSelection()
	}
	if m.state == selectingLLMProvider || m.state == selectingImageProvider || m.state == selectingLLMModel || m.state == selectingImageModel {
		return m.renderProviderSelection()
	}

	// Dynamic Styles from ActiveTheme
	statusStyle := lipgloss.NewStyle().Foreground(ActiveTheme.Secondary).Italic(true)
	titleStyle := TitleStyle()

	// Calculate status bar content
	charName := m.selectedCharacterID
	if charName == "" {
		charName = "None"
	}
	llmInfo := string(m.selectedLLMProvider)
	if m.selectedLLMModel != "" {
		llmInfo += " (" + m.selectedLLMModel + ")"
	}

	// Build status bar
	status := statusStyle.Render(fmt.Sprintf(" 👤 %s | 🎭 %s | 🧠 %s", m.user.Username, charName, llmInfo))
	help := MutedStyle().Render(" [ESC] Reset • [/help] Commands")

	// Error display
	errView := ""
	if m.err != nil {
		errView = ErrorStyle().Render(fmt.Sprintf(" ⚠️ ERROR: %v", m.err)) + "\n"
	}

	return fmt.Sprintf("%s\n%s\n%s%s\n%s",
		titleStyle.Render(" 🤖 THE SANCTUM — SECURE AGENT CHAT "),
		m.viewport.View(),
		errView,
		status,
		m.textInput.View()+" "+help)
}

func (m chatModel) renderCharacterSelection() string {
	var s strings.Builder
	s.WriteString(chatTitleStyle.Render("🛡️ NeXuS SECURE CHAT 🛡️") + "\n\n")

	if m.err != nil {
		s.WriteString(chatErrorStyle.Render(fmt.Sprintf("⚠️ %v", m.err)) + "\n\n")
	}

	if len(m.characterChoices) == 0 {
		s.WriteString(chatItemStyle.Render("No characters found. Create one in Character Generator first.") + "\n\n")
		s.WriteString(chatHelpStyle.Render("[ESC] Back to Menu"))
		return chatBoxStyle.Render(s.String())
	}

	s.WriteString("Select a character to chat with:\n\n")
	for i, ch := range m.characterChoices {
		line := fmt.Sprintf("%d. 🎭 %s", i+1, ch.Name)
		if m.providerCursor == i {
			s.WriteString(chatSelectedItemStyle.Render("👉 "+line) + "\n")
		} else {
			s.WriteString(chatItemStyle.Render("   "+line) + "\n")
		}
	}
	s.WriteString(chatHelpStyle.Render("\n[↑/↓] Navigate • [1-9] Quick pick • [Enter] Select • [ESC] Back"))
	return chatBoxStyle.Render(s.String())
}

func (m *chatModel) renderMessages() string {
	var sb strings.Builder

	// Display messages
	for _, msg := range m.messages {
		// DM visibility logic
		if msg.Recipient != "" && msg.Sender != m.user.Username && msg.Recipient != m.user.Username {
			continue // Don't show DMs not involving the user
		}

		var senderStyle lipgloss.Style
		var contentStyle lipgloss.Style
		var privateIndicator string

		if msg.Recipient != "" {
			privateIndicator = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(" 🔒 (private) ")
		}

		timestamp := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(time.Unix(msg.Timestamp, 0).Format("15:04")) + " "

		// Helper: render content with word-wrap based on available width after the prefix.
		renderWrapped := func(prefix string, cs lipgloss.Style, content string) string {
			avail := m.chatWidth - lipgloss.Width(prefix) - 2
			if avail < 20 {
				avail = 20
			}
			return prefix + cs.Width(avail).Render(content) + "\n"
		}

		switch msg.Type {
		case MessageTypeUser:
			senderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#87D7FF")).Bold(true)
			contentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#C5C5C5"))
			prefix := timestamp + "👤 " + senderStyle.Render(msg.Sender+":") + privateIndicator + " "
			sb.WriteString(renderWrapped(prefix, contentStyle, msg.Content))
		case MessageTypeCharacter:
			senderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#87FF87")).Bold(true)
			contentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#C5C5C5"))
			prefix := timestamp + "🎭 " + senderStyle.Render(msg.Sender+":") + privateIndicator + " "
			sb.WriteString(renderWrapped(prefix, contentStyle, msg.Content))
		case MessageTypeSystem:
			senderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD700")).Bold(true)
			contentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFF87"))
			prefix := timestamp + "⚙️  " + senderStyle.Render("SYSTEM:") + " "
			sb.WriteString(renderWrapped(prefix, contentStyle, msg.Content))
		case MessageTypeCommand:
			senderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#87AFFF")).Bold(true)
			contentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#AFAFAF"))
			prefix := timestamp + "⌨️  " + senderStyle.Render("CMD:") + " "
			sb.WriteString(renderWrapped(prefix, contentStyle, msg.Content))
		case MessageTypeImage:
			senderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF87D7")).Bold(true)
			contentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFC0CB"))
			imgPath := msg.Metadata["image_path"]
			if imgPath == "" {
				imgPath = "Unknown"
			}
			prefix := timestamp + "🖼️  " + senderStyle.Render("IMAGE:") + privateIndicator + " "
			sb.WriteString(renderWrapped(prefix, contentStyle, fmt.Sprintf("Generated Image (Path: %s)", imgPath)))
			if imgID, ok := msg.Metadata["image_id"]; ok && imgID != "" {
				imgBytes, err := DBGet(m.user.DB, m.user.EncryptionKey, "images", imgID)
				if err == nil {
					sb.WriteString("\n" + DisplayInlineImage(imgBytes, m.chatWidth-10) + "\n")
				}
			}
		case MessageTypeSummary:
			senderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#AFFF87")).Bold(true)
			contentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#E0FFE0"))
			prefix := timestamp + "📜 " + senderStyle.Render("SUMMARY:") + " "
			sb.WriteString(renderWrapped(prefix, contentStyle, msg.Content))
		case MessageTypeNarrator:
			senderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#D787FF")).Bold(true).Italic(true)
			contentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#EFEFEF")).Italic(true)
			prefix := timestamp + "📖 " + senderStyle.Render("NARRATOR:") + " "
			sb.WriteString(renderWrapped(prefix, contentStyle, msg.Content))
		default:
			senderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF")).Bold(true)
			contentStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#EFEFEF"))
			prefix := timestamp + "💬 " + senderStyle.Render(msg.Sender+":") + privateIndicator + " "
			sb.WriteString(renderWrapped(prefix, contentStyle, msg.Content))
		}
	}
	return sb.String()
}

func (m *chatModel) renderProviderSelection() string {
	var s strings.Builder
	if m.state == selectingLLMProvider {
		s.WriteString("Select an LLM Provider 🧠:\n\n")
	} else if m.state == selectingImageProvider {
		s.WriteString("Select an Image Provider 🖼️:\n\n")
	} else if m.state == selectingLLMModel {
		s.WriteString(fmt.Sprintf("Select a Model for %s 🧠:\n\n", m.selectedLLMProvider))
	} else if m.state == selectingImageModel {
		s.WriteString(fmt.Sprintf("Select a Model for %s 🖼️:\n\n", m.selectedImageProvider))
	} else if m.state == selectingSessionToImport {
		s.WriteString("Select a Session to Import Memories From 📁:\n\n")
	} else {
		s.WriteString("Select a Provider:\n\n")
	}

	for i, choice := range m.providerChoices {
		prefix := fmt.Sprintf("%d. ", i)
		if m.providerCursor == i {
			s.WriteString(chatSelectedItemStyle.Render(prefix+"👉 "+choice) + "\n")
		} else {
			s.WriteString(chatItemStyle.Render(prefix+"  "+choice) + "\n")
		}
	}
	s.WriteString(chatHelpStyle.Render(fmt.Sprintf("\nPress 'enter' to select, 'esc' to cancel.")))
	return chatBoxStyle.Render(s.String())
}

// providersToChoices converts a slice of Providers or Models to a slice of strings for TUI display.
func providersToChoices[T ~string](items []T) []string {
	choices := make([]string, len(items))
	for i, item := range items {
		choices[i] = string(item)
	}
	return choices
}
