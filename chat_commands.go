package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// handleCommand processes in-chat commands (starting with /)
func (m *chatModel) handleCommand(command, args string) tea.Cmd {
	switch command {
	case "help":
		helpMessage := "🛡️ NeXuS Command Protocol 🛡️\n\n" +
			"💬 /character {name} - Select active character\n" +
			"🖼️ /image {prompt} - Generate AI visual artifact\n" +
			"📖 /summarize - Trigger hierarchical memory compression\n" +
			"🤖 /models - List available models | /models set <n> to switch\n" +
			"🔍 /search {query} - Perform live web grounding\n" +
			"🌐 /fetch {url} - Fetch and strip text from a webpage\n" +
			"📤 /upload {path} [--vault] - Securely upload and analyze pixels\n" +
			"🔗 /share {name} - Link existing vault item to character brain\n" +
			"📌 /topic {topic} - Update conversation focal point\n" +
			"📚 /lore {key}={val} - Update character lorebook\n" +
			"📝 /special note {text} - Add atomic memory entry\n" +
			"🛑 /end - Clear session context\n" +
			"🚪 /quit - Save and exit cockpit\n"

		return m.systemMsg(helpMessage)

	case "upload":
		filePath := args
		useVault := false
		if strings.Contains(args, " --vault") {
			filePath = strings.Replace(args, " --vault", "", 1)
			useVault = true
		}
		filePath = strings.TrimSpace(filePath)
		
		if filePath == "" {
			return m.systemMsg("Usage: /upload <path> [--vault]")
		}

		fileInfo, err := os.Stat(filePath)
		if err != nil {
			return m.systemMsg(fmt.Sprintf("❌ File error: %v", err))
		}

		data, err := os.ReadFile(filePath)
		if err != nil {
			return m.systemMsg(fmt.Sprintf("❌ Read error: %v", err))
		}

		fileType := "file"
		ext := strings.ToLower(filepath.Ext(fileInfo.Name()))
		if ext == ".png" || ext == ".jpg" || ext == ".jpeg" {
			fileType = "image"
		} else if ext == ".txt" || ext == ".md" {
			fileType = "text"
		}

		var storageLoc string
		if useVault {
			vItem := VaultItem{
				ID: GenerateRandomID(), ParentID: VaultRootID, Name: fileInfo.Name(),
				Type: "file", MimeType: fileType, Size: fileInfo.Size(), CreatedAt: time.Now().Unix(),
			}
			SaveVaultFile(m.user, vItem, data)
			items, _ := LoadVaultIndex(m.user)
			SaveVaultIndex(m.user, append(items, vItem))
			storageLoc = "Vault 🛡️"
		} else {
			AddFileToLibrary(m.user, filePath, fileInfo.Name(), fileType)
			storageLoc = "Library 📚"
		}

		// Index and Vision Analysis
		if m.activeCharacterID != "" {
			if fileType == "text" {
				m.memoryManager.(*LocalMemoryManager).ImportLore(m.user, m.activeCharacterID, fileInfo.Name(), string(data), m.apiConfig)
			} else if fileType == "image" {
				// Cache image for cockpit display + inline message
				imageID := GenerateRandomID()
				DBPut(m.user.DB, m.user.EncryptionKey, "images", imageID, data)

				// Create inline image message so character sees it immediately
				inlineMsgID := GenerateRandomID()
				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
					defer cancel()
					visionSvc, visionModel := GetVisionService(m.llmService, m.apiConfig)
					if visionModel == "" { visionModel = LLMModel(m.apiConfig.GetVisionProviderModel()) }

					NexusLog("INFO", "vision", fmt.Sprintf("AnalyzeImage — provider:%T model:%s bytes:%d", visionSvc, visionModel, len(data)))

					const visionPrompt = "Look at this image carefully. State EXACTLY what is depicted: the main subject (person, place, object, scene), specific details, colors, mood, and setting. Name things precisely — say 'Eiffel Tower at sunset' not 'a tall structure'. Be specific and concrete in 3-4 sentences."
					description, err := visionSvc.AnalyzeImage(ctx, data, visionPrompt, visionModel, *m.apiConfig)
					// Auto-fallback: if configured provider fails, try Pollinations
					if err != nil && m.apiConfig.SelectedVisionLLMProvider != ProviderPollinations {
						NexusLog("WARN", "vision", fmt.Sprintf("primary vision failed (%v), falling back to Pollinations", err))
						if fbSvc, err2 := LLMFactory(ProviderPollinations, *m.apiConfig); err2 == nil {
							fbModel := m.apiConfig.Pollinations.DefaultLLMModel
							if fbModel == "" { fbModel = "openai" }
							description, err = fbSvc.AnalyzeImage(ctx, data, visionPrompt, fbModel, *m.apiConfig)
						}
					}
					if err == nil {
						NexusLog("INFO", "vision", fmt.Sprintf("description(%d chars): %s", len(description), description))
						// Save to RAG memory for long-term recall
						m.memoryManager.AddMemory(m.user, m.activeCharacterID, &MemoryEntry{
							Content: fmt.Sprintf("[VISUAL MEMORY: %s]\n%s", fileInfo.Name(), description),
							Type: MemoryTypeSemantic, Source: "vision:" + fileInfo.Name(),
						}, m.apiConfig)
						// Update the inline message so character sees it every turn
						globalProgram.Send(UpdateImageDescMsg{MessageID: inlineMsgID, Description: description})
						globalProgram.Send(ChatMsg{Message: Message{
							Sender: "System", Content: fmt.Sprintf("👁️ Vision locked on '%s'. Brain synced. 🧠", fileInfo.Name()),
							Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
						}})
					} else {
						NexusLog("ERROR", "vision", fmt.Sprintf("AnalyzeImage failed — provider:%T model:%s err:%v", visionSvc, visionModel, err))
						globalProgram.Send(ChatMsg{Message: Message{
							Sender: "System", Content: fmt.Sprintf("⚠️ Vision analysis failed: %v", err),
							Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
						}})
					}
				}()
				// Return the inline image message — displayed immediately while analysis runs
				return func() tea.Msg {
					return ChatMsg{Message: Message{
						ID: inlineMsgID, Sender: "You", Content: fileInfo.Name(),
						Type: MessageTypeImage, Timestamp: time.Now().Unix(),
						Metadata: map[string]string{"image_id": imageID, "image_path": fileInfo.Name()},
					}}
				}
			}
		}

		return m.systemMsg(fmt.Sprintf("✅ File '%s' stored in %s.", fileInfo.Name(), storageLoc))

	case "share":
		if m.activeCharacterID == "" {
			return m.systemMsg("Select a character first.")
		}
		fileName := strings.TrimSpace(args)
		if fileName == "" {
			return m.systemMsg("Usage: /share <filename>")
		}

		return func() tea.Msg {
			var content []byte
			var foundName string
			var isImage bool

			// Search Logic
			libFiles, _ := ListLibraryFiles(m.user)
			for _, f := range libFiles {
				if strings.EqualFold(f.Name, fileName) {
					_, data, err := LoadFileFromLibrary(m.user, f.ID)
					if err == nil { content = data; foundName = f.Name; isImage = f.Type == "image" }
					break
				}
			}
			if content == nil {
				vItems, _ := LoadVaultIndex(m.user)
				for _, it := range vItems {
					if strings.EqualFold(it.Name, fileName) {
						data, err := LoadVaultFile(m.user, it.ID)
						if err == nil { content = data; foundName = it.Name; isImage = it.MimeType == "image" }
						break
					}
				}
			}

			if content == nil { return ChatMsg{Message: Message{Sender: "System", Content: "❌ File not found."}} }

			if isImage {
				imageID := GenerateRandomID()
				DBPut(m.user.DB, m.user.EncryptionKey, "images", imageID, content)
				inlineMsgID := GenerateRandomID()
				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
					defer cancel()
					visionSvc, visionModel := GetVisionService(m.llmService, m.apiConfig)
					if visionModel == "" { visionModel = LLMModel(m.apiConfig.GetVisionProviderModel()) }

					NexusLog("INFO", "vision", fmt.Sprintf("/share AnalyzeImage — provider:%T model:%s bytes:%d", visionSvc, visionModel, len(content)))

					const visionPrompt = "Look at this image carefully. State EXACTLY what is depicted: the main subject (person, place, object, scene), specific details, colors, mood, and setting. Name things precisely — say 'Eiffel Tower at sunset' not 'a tall structure'. Be specific and concrete in 3-4 sentences."
					desc, err := visionSvc.AnalyzeImage(ctx, content, visionPrompt, visionModel, *m.apiConfig)
					// Auto-fallback: if configured provider fails, try Pollinations
					if err != nil && m.apiConfig.SelectedVisionLLMProvider != ProviderPollinations {
						NexusLog("WARN", "vision", fmt.Sprintf("/share primary vision failed (%v), falling back to Pollinations", err))
						if fbSvc, err2 := LLMFactory(ProviderPollinations, *m.apiConfig); err2 == nil {
							fbModel := m.apiConfig.Pollinations.DefaultLLMModel
							if fbModel == "" { fbModel = "openai" }
							desc, err = fbSvc.AnalyzeImage(ctx, content, visionPrompt, fbModel, *m.apiConfig)
						}
					}

					if err == nil {
						NexusLog("INFO", "vision", fmt.Sprintf("/share description(%d chars): %s", len(desc), desc))
						// Save to RAG memory for long-term recall
						m.memoryManager.AddMemory(m.user, m.activeCharacterID, &MemoryEntry{
							Content: fmt.Sprintf("[VISUAL MEMORY: %s]\n%s", foundName, desc),
							Type: MemoryTypeSemantic, Source: "vision:" + foundName,
						}, m.apiConfig)
						// Update the inline message so character sees it every turn
						globalProgram.Send(UpdateImageDescMsg{MessageID: inlineMsgID, Description: desc})
						globalProgram.Send(ChatMsg{Message: Message{
							Sender: "System", Content: fmt.Sprintf("👁️ Vision locked on shared item '%s'. Brain synced. 🧠", foundName),
							Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
						}})
					} else {
						NexusLog("ERROR", "vision", fmt.Sprintf("/share AnalyzeImage failed — provider:%T model:%s err:%v", visionSvc, visionModel, err))
						globalProgram.Send(ChatMsg{Message: Message{
							Sender: "System", Content: fmt.Sprintf("⚠️ Vision analysis failed: %v", err),
							Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
						}})
					}
				}()
				return ChatMsg{Message: Message{
					ID: inlineMsgID, Sender: "You", Content: foundName,
					Type: MessageTypeImage, Timestamp: time.Now().Unix(),
					Metadata: map[string]string{"image_id": imageID, "image_path": foundName},
				}}
			}

			m.memoryManager.(*LocalMemoryManager).ImportLore(m.user, m.activeCharacterID, foundName, string(content), m.apiConfig)
			return ChatMsg{Message: Message{Sender: "System", Content: "🔗 Knowledge link established. 🧠"}}
		}

	case "character":
		if args == "" {
			m.state = selectingCharacter
			return nil
		}
		charInfo, err := findCharacterInfoByName(m.user, args)
		if err != nil { return m.systemMsg(err.Error()) }
		return func() tea.Msg { return SetChatCharacterMsg{CharacterID: charInfo.ID} }

	case "image":
		if args == "" { return m.systemMsg("Usage: /image <prompt>") }
		// Capture needed state for goroutine
		capturedUser := m.user
		capturedCharID := m.activeCharacterID
		capturedMM := m.memoryManager
		capturedCfg := m.apiConfig
		capturedLLM := m.llmService
		capturedPrompt := args

		// Apply visual style presets
		styledPrompt := buildStyledImagePrompt(capturedPrompt, capturedCfg.ImageStyle)

		return func() tea.Msg {
			imgModel := ImageModel(capturedCfg.GetActiveImageModel())
			imgB64, err := m.imageService.GenerateImage(context.Background(), styledPrompt, imgModel, *capturedCfg)
			if err != nil { return ChatMsg{Error: err} }

			// Cache generated image
			imageID := GenerateRandomID()
			imgData, _ := base64.StdEncoding.DecodeString(imgB64)
			DBPut(capturedUser.DB, capturedUser.EncryptionKey, "images", imageID, imgData)

			inlineMsgID := GenerateRandomID()

			// Async: describe image → RAG memory (same dual-front as /upload)
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
				defer cancel()
				visionSvc, visionModel := GetVisionService(capturedLLM, capturedCfg)
				if visionModel == "" { visionModel = LLMModel(capturedCfg.GetVisionProviderModel()) }
				NexusLog("INFO", "vision", fmt.Sprintf("/image AnalyzeImage — provider:%T model:%s bytes:%d", visionSvc, visionModel, len(imgData)))
				const imgVisionPrompt = "Look at this image carefully. State EXACTLY what is depicted: the main subject (person, place, object, scene), specific details, colors, mood, and setting. Name things precisely — say 'Eiffel Tower at sunset' not 'a tall structure'. Be specific and concrete in 3-4 sentences."
				desc, err := visionSvc.AnalyzeImage(ctx, imgData, imgVisionPrompt, visionModel, *capturedCfg)
				// Auto-fallback to Pollinations if primary vision provider fails
				if err != nil && capturedCfg.SelectedVisionLLMProvider != ProviderPollinations {
					NexusLog("WARN", "vision", fmt.Sprintf("/image primary vision failed (%v), falling back to Pollinations", err))
					if fbSvc, err2 := LLMFactory(ProviderPollinations, *capturedCfg); err2 == nil {
						fbModel := capturedCfg.Pollinations.DefaultLLMModel
						if fbModel == "" { fbModel = "openai" }
						desc, err = fbSvc.AnalyzeImage(ctx, imgData, imgVisionPrompt, fbModel, *capturedCfg)
					}
				}
				if err == nil {
					NexusLog("INFO", "vision", fmt.Sprintf("/image description(%d chars): %s", len(desc), desc))
					capturedMM.AddMemory(capturedUser, capturedCharID, &MemoryEntry{
						Content: fmt.Sprintf("[VISUAL MEMORY: generated image]\nPrompt: %s\n%s", capturedPrompt, desc),
						Type:    MemoryTypeSemantic, Source: "vision:generated",
					}, capturedCfg)
					globalProgram.Send(UpdateImageDescMsg{MessageID: inlineMsgID, Description: desc})
				} else {
					NexusLog("ERROR", "vision", fmt.Sprintf("/image AnalyzeImage failed — provider:%T model:%s err:%v", visionSvc, visionModel, err))
				}
			}()

			return ChatMsg{Message: Message{
				ID: inlineMsgID, Sender: "System", Content: capturedPrompt,
				Type: MessageTypeImage, Timestamp: time.Now().Unix(),
				Metadata: map[string]string{"image_id": imageID},
			}}
		}

	case "models":
		// /models — list available models for current provider, pick by number
		// /models set <number> — switch to that model immediately
		capturedSvc := m.llmService
		capturedProvider := m.selectedLLMProvider
		capturedModels := m.llmModels // cached from last fetch

		if args != "" && strings.HasPrefix(args, "set ") {
			// /models set <n> — apply selection from previously listed models
			numStr := strings.TrimPrefix(args, "set ")
			n, err := strconv.Atoi(strings.TrimSpace(numStr))
			if err != nil || n < 1 || n > len(capturedModels) {
				return m.systemMsg(fmt.Sprintf("⚠️ Invalid selection. Use /models to list, then /models set <number>"))
			}
			chosen := string(capturedModels[n-1])
			m.selectedLLMModel = chosen
			// Persist to config
			switch capturedProvider {
			case ProviderPollinations: m.apiConfig.Pollinations.DefaultLLMModel = LLMModel(chosen)
			case ProviderGemini:       m.apiConfig.Gemini.DefaultLLMModel = LLMModel(chosen)
			case ProviderMistral:      m.apiConfig.Mistral.DefaultLLMModel = LLMModel(chosen)
			case ProviderGroq:         m.apiConfig.Groq.DefaultLLMModel = LLMModel(chosen)
			case ProviderOpenAI:       m.apiConfig.OpenAI.DefaultLLMModel = LLMModel(chosen)
			case ProviderClaude:       m.apiConfig.Claude.DefaultLLMModel = LLMModel(chosen)
			case ProviderOllama:       m.apiConfig.Ollama.DefaultLLMModel = LLMModel(chosen)
			case ProviderOpenRouter:   m.apiConfig.OpenRouter.DefaultLLMModel = LLMModel(chosen)
			}
			SaveAPIConfig(m.user, *m.apiConfig)
			return m.systemMsg(fmt.Sprintf("✅ Model set to: %s", chosen))
		}

		return func() tea.Msg {
			models, err := capturedSvc.GetAvailableModels(context.Background())
			if err != nil || len(models) == 0 {
				return ChatMsg{Message: Message{Sender: "System",
					Content: fmt.Sprintf("⚠️ Could not fetch models for %s: %v", capturedProvider, err),
					Type: MessageTypeSystem, Timestamp: time.Now().Unix()}}
			}
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("📋 Models for %s — type /models set <n> to switch:\n\n", capturedProvider))
			for i, model := range models {
				sb.WriteString(fmt.Sprintf("  %2d. %s\n", i+1, model))
			}
			return ChatMsg{Message: Message{Sender: "System", Content: sb.String(),
				Type: MessageTypeSystem, Timestamp: time.Now().Unix()}}
		}

	case "summarize":
		return func() tea.Msg {
			m.memoryManager.GenerateRollingSummary(m.user, m.activeCharacterID, m.messages, "Summarize context.", m.apiConfig)
			return ChatMsg{Message: Message{Sender: "System", Content: "🌀 Hierarchical compression engaged."}}
		}

	case "save":
		return func() tea.Msg {
			SaveChatSession(m.user, m.activeCharacterID, m.messages)
			return ChatMsg{Message: Message{Sender: "System", Content: "💾 Session sealed in Vault."}}
		}

	case "end":
		m.messages = []Message{}
		return m.systemMsg("🛑 Session cleared.")

	case "quit":
		return func() tea.Msg { return tea.Quit() }

	default:
		return m.systemMsg(fmt.Sprintf("❌ Unknown command: /%s", command))
	}
}

func (m *chatModel) handleTopicChange(topic string) tea.Cmd {
	m.systemPrompt = "Focus on: " + topic
	return m.systemMsg("📌 Topic set: " + topic)
}

func (m *chatModel) handleDM(args string) tea.Cmd {
	return m.systemMsg("DM Protocol Engaged.")
}

func (m *chatModel) handleInvite(name string) tea.Cmd {
	return m.systemMsg("Inviting " + name + "...")
}

func (m *chatModel) handleNarrator(prompt string) tea.Cmd {
	return m.systemMsg("Narrator awakened.")
}

// findCharacterInfoByName is a helper to find a CharacterInfo by name.
func findCharacterInfoByName(user *User, name string) (*CharacterInfo, error) {
	chars, err := ListCharacters(user)
	if err != nil {
		return nil, err
	}
	for _, charInfo := range chars {
		if strings.EqualFold(charInfo.Name, name) {
			return &charInfo, nil
		}
	}
	return nil, fmt.Errorf("character '%s' not found", name)
}

// getOrLoadCharacter retrieves a character from the cache or loads it from disk.
func (m *chatModel) getOrLoadCharacter(characterID string) (*Character, error) {
	if m.loadedCharacters == nil {
		m.loadedCharacters = make(map[string]*Character)
	}
	if char, ok := m.loadedCharacters[characterID]; ok {
		return char, nil
	}
	char, err := LoadCharacter(m.user, characterID)
	if err != nil {
		return nil, err
	}
	m.loadedCharacters[characterID] = char
	return char, nil
}

func (m *chatModel) loadLorebooksForCharacter(char *Character) {
	m.activeLorebooks = []Lorebook{}
	if len(char.LorebookIDs) == 0 {
		return
	}

	allLorebooks, err := LoadLorebooks(m.user)
	if err != nil {
		return
	}

	for _, id := range char.LorebookIDs {
		for _, lb := range allLorebooks {
			if lb.ID == id {
				m.activeLorebooks = append(m.activeLorebooks, lb)
				break
			}
		}
	}
}

// SignChatMessage signs a character message using the character's private key.
func SignChatMessage(msg *Message, character *Character, user *User) error {
	if character.PrivateKeyEncA == "" {
		return nil // unsigned if no key yet (legacy char)
	}
	privPEM, err := DecryptPEM(character.PrivateKeyEncA, user.EncryptionKey)
	if err != nil {
		return err
	}
	payload := []byte(msg.Content + "|" + strconv.FormatInt(msg.Timestamp, 10))
	sig, err := SignData(payload, privPEM)
	if err != nil {
		return err
	}
	msg.Signature = sig
	msg.SignedBy = character.ID
	return nil
}

// parseCommand splits a string into a command and its arguments.
func parseCommand(input string) (string, string, bool) {
	if !strings.HasPrefix(input, "/") {
		return "", "", false
	}
	parts := strings.SplitN(input[1:], " ", 2)
	command := strings.ToLower(parts[0])
	args := ""
	if len(parts) > 1 {
		args = parts[1]
	}
	return command, args, true
}

func GetLLMProviders() []Provider {
	return []Provider{
		ProviderNexusRouter,                                        // Local fleet manager (Ollama→cloud failover)
		ProviderGemini, ProviderPollinations, ProviderMistral,
		ProviderGroq, ProviderOpenAI, ProviderClaude,
		ProviderOllama, ProviderOpenRouter, ProviderHuggingFace,
	}
}

func GetImageProviders() []Provider {
	return []Provider{
		ProviderPollinations, ProviderAIHorde, ProviderOpenAI, ProviderImageRouter,
	}
}

func parseAgentActions(content string) []string {
	// Simple regex to find /command tags
	re := regexp.MustCompile(`/[a-zA-Z0-9_-]+(\s+[^\n]+)?`)
	return re.FindAllString(content, -1)
}

// m.systemMsg is a helper to return a system message bubble
func (m *chatModel) systemMsg(content string) tea.Cmd {
	return func() tea.Msg {
		return ChatMsg{Message: Message{
			ID: GenerateRandomID(), Sender: "System", Content: content,
			Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
		}}
	}
}

// openLastImageCmd finds the most recent MessageTypeImage in history, writes it to
// a temp file, and tries to open it in Kitty → imv → xdg-open, in that order.
func openLastImageCmd(messages []Message, user *User) tea.Cmd {
	return func() tea.Msg {
		// Walk backwards to find latest image message
		var imageID string
		var imageName string
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Type == MessageTypeImage {
				imageID = messages[i].Metadata["image_id"]
				imageName = messages[i].Metadata["image_path"]
				break
			}
		}
		if imageID == "" {
			return ChatMsg{Message: Message{
				ID: GenerateRandomID(), Sender: "System",
				Content: "❌ No image in this session yet.", Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
			}}
		}

		imgData, err := DBGet(user.DB, user.EncryptionKey, "images", imageID)
		if err != nil || len(imgData) == 0 {
			return ChatMsg{Message: Message{
				ID: GenerateRandomID(), Sender: "System",
				Content: fmt.Sprintf("❌ Could not load image from vault: %v", err),
				Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
			}}
		}

		// Write to temp file with correct extension
		ext := filepath.Ext(imageName)
		if ext == "" { ext = ".png" }
		tmpFile, err := os.CreateTemp("", "nexus-image-*"+ext)
		if err != nil {
			return ChatMsg{Message: Message{
				ID: GenerateRandomID(), Sender: "System",
				Content: fmt.Sprintf("❌ Temp file error: %v", err),
				Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
			}}
		}
		tmpFile.Write(imgData)
		tmpFile.Close()

		tmpPath := tmpFile.Name()

		// Try viewers in priority order: imv (Wayland) → timg → feh → xdg-open
		// Note: Kitty skipped — crashes before fully loading on this system
		viewers := [][]string{
			{"imv", tmpPath},
			{"timg", "--loops=1", tmpPath},
			{"feh", tmpPath},
			{"xdg-open", tmpPath},
		}
		opened := ""
		for _, args := range viewers {
			if path, lookErr := exec.LookPath(args[0]); lookErr == nil {
				cmd := exec.Command(path, args[1:]...)
				if startErr := cmd.Start(); startErr == nil {
					opened = args[0]
					break
				}
			}
		}

		if opened == "" {
			return ChatMsg{Message: Message{
				ID: GenerateRandomID(), Sender: "System",
				Content: fmt.Sprintf("❌ No image viewer found. Image saved to: %s", tmpPath),
				Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
			}}
		}
		return ChatMsg{Message: Message{
			ID: GenerateRandomID(), Sender: "System",
			Content: fmt.Sprintf("🖼️ Opened in %s → %s", opened, tmpPath),
			Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
		}}
	}
}
