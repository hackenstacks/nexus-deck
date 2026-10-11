package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// CharactersLoadedMsg carries the character list on startup.
type CharactersLoadedMsg struct {
	Chars []CharacterInfo
	Err   error
}

func (m chatModel) Init() tea.Cmd {
	u := m.user
	return func() tea.Msg {
		chars, err := ListCharacters(u)
		return CharactersLoadedMsg{Chars: chars, Err: err}
	}
}

func (m chatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	if m.approvalModel != nil {
		newModel, cmd := m.approvalModel.Update(msg)
		am := newModel.(approvalModel)
		m.approvalModel = &am
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.chatHeight = msg.Height - 5
		m.chatWidth = msg.Width
		m.viewport.Width = msg.Width
		m.viewport.Height = m.chatHeight
		m.viewport.SetContent(m.renderMessages())

	case CharactersLoadedMsg:
		if msg.Err != nil {
			NexusLog("ERROR", "chat", fmt.Sprintf("failed to list characters: %v", msg.Err))
			m.err = fmt.Errorf("failed to load character list: %w", msg.Err)
		} else {
			m.characterChoices = msg.Chars
		}
		return m, nil

	case SetChatCharacterMsg:
		char, err := LoadCharacter(m.user, msg.CharacterID)
		if err != nil {
			NexusLog("ERROR", "chat", fmt.Sprintf("LoadCharacter(%s): %v", msg.CharacterID, err))
			m.err = fmt.Errorf("failed to load character: %w", err)
			m.state = selectingCharacter // bounce back to picker
			return m, nil
		}
		m.activeCharacterID = char.ID
		m.selectedCharacterID = char.Name
		m.chatSessionID = GenerateRandomID()

		// Add the character to participants so the turn system fires the LLM
		alreadyIn := false
		for _, p := range m.participants {
			if p == char.Name {
				alreadyIn = true
				break
			}
		}
		if !alreadyIn {
			m.participants = append(m.participants, char.Name)
		}
		m.turn = 0 // user goes first

		m.loadLorebooksForCharacter(char)

		history, err := LoadChatSession(m.user, m.activeCharacterID)
		if err != nil {
			m.err = fmt.Errorf("failed to load chat history: %w", err)
			m.messages = []Message{}
		} else {
			m.messages = history
		}

		if len(m.messages) == 0 && char.FirstMessage != "" {
			firstMsg := Message{
				ID:        GenerateRandomID(),
				Sender:    char.Name,
				Content:   char.FirstMessage,
				Timestamp: time.Now().Unix(),
				Type:      MessageTypeCharacter,
			}
			m.messages = append(m.messages, firstMsg)
		}
		systemMsg := Message{
			ID:        GenerateRandomID(),
			Sender:    "System",
			Content:   fmt.Sprintf("Started chat with %s.", char.Name),
			Timestamp: time.Now().Unix(),
			Type:      MessageTypeSystem,
		}
		m.messages = append(m.messages, systemMsg)
		m.viewport.SetContent(m.renderMessages())
		m.viewport.GotoBottom()
		return m, nil

	case tea.KeyMsg:
		// --- Character selection screen ---
		if m.state == selectingCharacter {
			switch msg.String() {
			case "ctrl+c", "ctrl+q":
				return m, tea.Quit
			case "esc":
				if m.state == selectingCharacter || m.state == selectingLLMProvider || m.state == selectingImageProvider || m.state == selectingLLMModel || m.state == selectingImageModel {
					m.state = chatting
					return m, nil
				}
				// In standalone mode this might exit, but in Deck we just reset local state
				m.err = nil
				return m, nil
			case "up", "k":
				if m.providerCursor > 0 {
					m.providerCursor--
				}
			case "down", "j":
				if m.providerCursor < len(m.characterChoices)-1 {
					m.providerCursor++
				}
			case "enter":
				if len(m.characterChoices) == 0 {
					return m, func() tea.Msg { return BackToMainAppMsg{} }
				}
				chosen := m.characterChoices[m.providerCursor]
				NexusLog("INFO", "chat", fmt.Sprintf("loading character: %s (%s)", chosen.Name, chosen.ID))
				m.state = chatting
				m.providerCursor = 0
				return m, func() tea.Msg { return SetChatCharacterMsg{CharacterID: chosen.ID} }
			default:
				// Number shortcut — use ctrl+number to avoid conflict
				if strings.HasPrefix(msg.String(), "ctrl+") && len(msg.String()) == 6 && msg.String()[5] >= '1' && msg.String()[5] <= '9' {
					idx := int(msg.String()[5] - '1')
					if idx < len(m.characterChoices) {
						chosen := m.characterChoices[idx]
						NexusLog("INFO", "chat", fmt.Sprintf("loading character: %s (%s)", chosen.Name, chosen.ID))
						m.state = chatting
						m.providerCursor = 0
						return m, func() tea.Msg { return SetChatCharacterMsg{CharacterID: chosen.ID} }
					}
				}
			}
			return m, nil
		}

		if m.state == selectingLLMProvider || m.state == selectingImageProvider || m.state == selectingLLMModel || m.state == selectingImageModel {
			switch msg.String() {
			case "up", "k":
				if m.providerCursor > 0 {
					m.providerCursor--
				}
			case "down", "j":
				if m.providerCursor < len(m.providerChoices)-1 {
					m.providerCursor++
				}
			case "enter":
				if m.state == selectingLLMProvider {
					m.selectedLLMProvider = Provider(m.providerChoices[m.providerCursor])
					m.state = selectingLLMModel
					llmSvc, err := LLMFactory(m.selectedLLMProvider, *m.apiConfig)
					if err != nil {
						m.err = fmt.Errorf("failed to initialize LLM service for model listing: %w", err)
						m.providerChoices = []string{}
					} else {
						models, err := llmSvc.GetAvailableModels(context.Background())
						if err != nil {
							m.err = fmt.Errorf("failed to get LLM models from %s: %w", m.selectedLLMProvider, err)
							m.providerChoices = []string{}
						} else {
							llmModels := make([]string, len(models))
							for i, v := range models {
								llmModels[i] = string(v)
							}
							m.llmModels = llmModels
							m.providerChoices = providersToChoices(m.llmModels)
						}
					}
					m.providerCursor = 0
				} else if m.state == selectingImageProvider {
					m.selectedImageProvider = Provider(m.providerChoices[m.providerCursor])
					m.state = selectingImageModel
					imgSvc, err := ImageFactory(m.selectedImageProvider, *m.apiConfig)
					if err != nil {
						m.err = fmt.Errorf("failed to initialize Image service for model listing: %w", err)
						m.providerChoices = []string{}
					} else {
						models, err := imgSvc.GetAvailableModels(context.Background())
						if err != nil {
							m.err = fmt.Errorf("failed to get Image models from %s: %w", m.selectedImageProvider, err)
							m.providerChoices = []string{}
						} else {
							imageModels := make([]string, len(models))
							for i, v := range models {
								imageModels[i] = string(v)
							}
							m.imageModels = imageModels
							m.providerChoices = providersToChoices(m.imageModels)
						}
					}
					m.providerCursor = 0
				} else if m.state == selectingLLMModel {
					m.selectedLLMModel = m.providerChoices[m.providerCursor]
					m.state = selectingImageProvider
					m.providerChoices = providersToChoices(GetImageProviders())
					m.providerCursor = 0
				} else if m.state == selectingImageModel {
					m.selectedImageModel = m.providerChoices[m.providerCursor]
					llmService, err := LLMFactory(m.selectedLLMProvider, *m.apiConfig)
					if err != nil {
						m.err = fmt.Errorf("failed to create LLM service: %w", err)
					} else {
						m.llmService = llmService
					}

					imageService, err := ImageFactory(m.selectedImageProvider, *m.apiConfig)
					if err != nil {
						m.err = fmt.Errorf("failed to create Image service: %w", err)
					} else {
						m.imageService = imageService
					}
					m.state = chatting
				} else if m.state == selectingSessionToImport {
					selectedSessionID := m.providerChoices[m.providerCursor]
					m.state = chatting

					characterID := m.activeCharacterID
					if characterID == "" {
						characterID = m.selectedCharacterID
					}

					if characterID != "" {
						go func() {
							msgs, err := LoadChatSession(m.user, selectedSessionID)
							if err != nil {
								globalProgram.Send(ChatMsg{Error: fmt.Errorf("failed to load session for import: %w", err)})
								return
							}
							count, err := m.memoryManager.(*LocalMemoryManager).ImportMemoryFromSession(m.user, characterID, msgs, m.apiConfig)
							if err != nil {
								globalProgram.Send(ChatMsg{Error: fmt.Errorf("memory import failed: %w", err)})
								return
							}
							sysMsg := Message{
								ID: GenerateRandomID(), Sender: "System",
								Content:   fmt.Sprintf("✨ Imported %d relevant memories from session '%s'!", count, selectedSessionID),
								Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
							}
							globalProgram.Send(ChatMsg{Message: sysMsg})
						}()
					}
				}
				return m, nil
			default:
				if strings.HasPrefix(msg.String(), "ctrl+") && len(msg.String()) == 6 && msg.String()[5] >= '0' && msg.String()[5] <= '9' {
					selectedNum := int(msg.String()[5] - '0')
					if selectedNum >= 0 && selectedNum < len(m.providerChoices) {
						m.providerCursor = selectedNum
						return m.Update(tea.KeyMsg{Type: tea.KeyEnter})
					}
				}
			}
			return m, nil
		}

		switch msg.String() {
		case "ctrl+c", "ctrl+q":
			if m.activeCharacterID != "" && len(m.messages) > 0 {
				if err := SaveChatSession(m.user, m.activeCharacterID, m.messages); err != nil {
					fmt.Fprintf(os.Stderr, "Error saving chat session: %v\n", err)
				}
			}
			return m, tea.Quit
		case "esc":
			switch m.state {
			case chatting:
				if m.activeCharacterID != "" && len(m.messages) > 0 {
					if err := SaveChatSession(m.user, m.activeCharacterID, m.messages); err != nil {
						fmt.Fprintf(os.Stderr, "Error saving chat session: %v\n", err)
					}
				}
				// Blur the input so deck tab shortcuts (1-5, S) work again
				m.textInput.Blur()
				return m, nil
			case selectingLLMProvider:
				m.state = chatting
				return m, nil
			case selectingLLMModel:
				m.state = selectingLLMProvider
				m.providerChoices = providersToChoices(GetLLMProviders())
				m.providerCursor = 0
				return m, nil
			case selectingImageProvider:
				m.state = selectingLLMModel
				if models, err := m.llmService.GetAvailableModels(context.Background()); err != nil {
					m.err = fmt.Errorf("failed to get LLM models from %s: %w", m.selectedLLMProvider, err)
					m.providerChoices = []string{}
				} else {
					llmModels := make([]string, len(models))
					for i, v := range models {
						llmModels[i] = string(v)
					}
					m.providerChoices = llmModels
				}
				m.providerCursor = 0
				return m, nil
			case selectingImageModel:
				m.state = selectingImageProvider
				m.providerChoices = providersToChoices(GetImageProviders())
				m.providerCursor = 0
				return m, nil
			case selectingSessionToImport:
				m.state = chatting
				return m, nil
			}
			return m, nil

		case "enter":
			input := m.textInput.Value()
			m.textInput.SetValue("")

			if strings.TrimSpace(input) == "" {
				return m, nil
			}

			command, args, isCommand := parseCommand(input)
			if isCommand {
				commandMessage := Message{
					ID:        GenerateRandomID(),
					Sender:    m.user.Username,
					Content:   input,
					Timestamp: time.Now().Unix(),
					Type:      MessageTypeCommand,
				}
				m.messages = append(m.messages, commandMessage)
				return m, m.handleCommand(command, args)
			}

			userMessage := Message{
				ID:        GenerateRandomID(),
				Sender:    m.user.Username,
				Content:   input,
				Timestamp: time.Now().Unix(),
				Type:      MessageTypeUser,
			}
			m.messages = append(m.messages, userMessage)

			memEntry := &MemoryEntry{
				Content:  userMessage.Content,
				Type:     "episodic",
				Source:   fmt.Sprintf("Chat with %s", m.selectedCharacterID),
				Keywords: []string{m.user.Username, m.selectedCharacterID},
			}
			if m.selectedCharacterID != "" {
				if err := m.memoryManager.AddMemory(m.user, m.selectedCharacterID, memEntry, m.apiConfig); err != nil {
					m.err = fmt.Errorf("failed to save user message to memory: %w", err)
				}
			}

			if m.selectedCharacterID == "" {
				systemMessage := Message{
					ID:        GenerateRandomID(),
					Sender:    "System",
					Content:   "Please select a character to chat with using `/character <name>` or invite one with `/invite <name>`.",
					Timestamp: time.Now().Unix(),
					Type:      MessageTypeSystem,
				}
				m.messages = append(m.messages, systemMessage)
				m.viewport.SetContent(m.renderMessages())
				m.viewport.GotoBottom()
				return m, nil
			}

			m.turn = (m.turn + 1) % len(m.participants)
			if m.participants[m.turn] == m.user.Username {
				if len(m.participants) > 1 {
					m.turn = 1
				} else {
					m.turn = 0
				}
			}

			m.viewport.SetContent(m.renderMessages())
			m.viewport.GotoBottom()

			if m.turn > 0 && len(m.participants) > 1 {
				nextCharName := m.participants[m.turn]
				charInfo, err := findCharacterInfoByName(m.user, nextCharName)
				if err != nil {
					m.err = fmt.Errorf("failed to find next character '%s' in participant list", nextCharName)
					return m, nil
				}
				m.activeCharacterID = charInfo.ID

				llmCmd := func(characterID string, currentInput string) tea.Cmd {
					return func() tea.Msg {
					char, err := LoadCharacter(m.user, characterID)
					if err != nil {
						return ChatMsg{Error: fmt.Errorf("failed to load active character '%s': %w", characterID, err)}
					}

					// Plugin: beforeResponseGenerate
					if char.PluginEnabled && char.PluginCode != "" {
						result := RunBeforeResponseGenerate(char.PluginCode, currentInput)
						if result.Error != nil {
							fmt.Printf("Plugin warning (before): %v\n", result.Error)
						} else if result.Modified {
							currentInput = result.Content
						}
					}

					var promptBuilder strings.Builder
					
					// PHASE I: Scene-Aware Memory Retrieval
					searchQuery := currentInput
					if strings.Contains(strings.ToLower(currentInput), "see") || strings.Contains(strings.ToLower(currentInput), "look") || strings.Contains(strings.ToLower(currentInput), "describe") {
						searchQuery += " visual description scene environment"
					}
					
					retrievedMemories, _ := m.memoryManager.RetrieveMemories(m.user, char.ID, searchQuery, 8, m.apiConfig)

					// Specifically pull the MOST RECENT vision memory to ensure they don't ignore it
					allMemories, _ := m.memoryManager.(*LocalMemoryManager).loadAllCharacterMemories(m.user, char.ID)
					var latestVision string
					for i := len(allMemories)-1; i >= 0; i-- {
						if strings.HasPrefix(allMemories[i].Source, "vision:") {
							latestVision = allMemories[i].Content
							break
						}
					}

					kg, _ := LoadKnowledgeGraph(m.user, char.ID)
					kgContext := kg.QueryKnowledge(currentInput)

					usedLLMModel := LLMModel(m.selectedLLMModel)
					if usedLLMModel == "" {
						return ChatMsg{Error: fmt.Errorf("no LLM model selected for provider '%s'", m.selectedLLMProvider)}
					}

					promptBuilder.WriteString(fmt.Sprintf("### IDENTITY: You are %s. Roleplay based on these details. Respond ONLY in character.\n", char.Name))
					promptBuilder.WriteString(fmt.Sprintf("DESCRIPTION: %s\n", char.Description))
					promptBuilder.WriteString(fmt.Sprintf("PERSONALITY: %s\n", char.Personality))
					promptBuilder.WriteString(fmt.Sprintf("SCENARIO: %s\n", char.Scenario))

					if latestVision != "" {
						promptBuilder.WriteString("\n--- 👁️ CURRENT VISUAL CONTEXT (MANDATORY) ---\n")
						promptBuilder.WriteString(latestVision + "\n")
					}

					if char.PostHistoryInstructions != "" {
						promptBuilder.WriteString(fmt.Sprintf("POST-HISTORY INSTRUCTIONS: %s\n", char.PostHistoryInstructions))
					}

					if len(char.Lorebook) > 0 {
						promptBuilder.WriteString("\n--- 📚 LOREBOOK ENTRIES ---\n")
						for key, value := range char.Lorebook {
							promptBuilder.WriteString(fmt.Sprintf("%s: %s\n", key, value))
						}
					}

					if kgContext != "" {
						promptBuilder.WriteString(kgContext)
					}

					if len(m.activeLorebooks) > 0 {
						lore := FindRelevantLore(m.messages, m.activeLorebooks)
						if lore != "" {
							promptBuilder.WriteString("\n\n[WORLD LORE — use this context]\n" + lore + "\n")
						}
					}

					promptBuilder.WriteString("\n--- Relevant Memories ---\n")
					if len(retrievedMemories) > 0 {
						for _, mem := range retrievedMemories {
							promptBuilder.WriteString(fmt.Sprintf("[%s]: %s\n", mem.Type, mem.Content))
						}
					} else {
						promptBuilder.WriteString("No specific memories retrieved for this interaction.\n")
					}
					promptBuilder.WriteString("--- End Memories ---\n")

					if m.systemPrompt != "" {
						promptBuilder.WriteString(fmt.Sprintf("\n--- ⚙️  ACTIVE SYSTEM PROMPT ---\\n%s\\n", m.systemPrompt))
					}

					// PHASE H: Context Budgeter
					budgetedMessages := TrimMessagesToBudget(m.messages, 4000) // Keep ~16k chars context

					// Check if any images exist in history AND the provider supports multimodal
					mmSvc, providerSupportsMultimodal := m.llmService.(MultiModalLLMService)
					hasImages := false
					for _, msg := range budgetedMessages {
						if msg.Type == MessageTypeImage && msg.Metadata["image_id"] != "" {
							hasImages = true
							break
						}
					}

					var llmResponseContent string
					if providerSupportsMultimodal && hasImages {
						// Build structured messages: system + chat history with real image bytes
						systemText := promptBuilder.String()
						var mmMessages []MultiModalMessage
						mmMessages = append(mmMessages, TextMsg("system", systemText))

						for _, msg := range budgetedMessages {
							if msg.Recipient != "" && msg.Recipient != char.Name && msg.Sender != char.Name {
								continue
							}
							role := "user"
							if msg.Sender == char.Name {
								role = "assistant"
							}
							if msg.Type == MessageTypeSummary {
								mmMessages = append(mmMessages, TextMsg(role, "[SUMMARY]: "+msg.Content))
							} else if msg.Type == MessageTypeImage {
								imageID := msg.Metadata["image_id"]
								imgData, dbErr := DBGet(m.user.DB, m.user.EncryptionKey, "images", imageID)
								if dbErr == nil && len(imgData) > 0 {
									// Detect mime type from first bytes
									mimeType := "image/jpeg"
									if len(imgData) > 4 && imgData[0] == 0x89 && imgData[1] == 'P' {
										mimeType = "image/png"
									}
									mmMessages = append(mmMessages, MultiModalMessage{
										Role: "user",
										Parts: []MultiModalPart{
											{Type: "text", Text: msg.Sender + " shared an image:"},
											{Type: "image", ImageData: imgData, MimeType: mimeType},
										},
									})
								} else if desc, ok := msg.Metadata["vision_description"]; ok && desc != "" {
									// Image no longer in cache — fall back to description
									mmMessages = append(mmMessages, TextMsg("user", fmt.Sprintf("%s shared an image — [IMAGE: %s]", msg.Sender, desc)))
								}
							} else {
								mmMessages = append(mmMessages, TextMsg(role, fmt.Sprintf("%s: %s", msg.Sender, msg.Content)))
							}
						}
						// Final user message (the current input)
						mmMessages = append(mmMessages, TextMsg("user", fmt.Sprintf("%s: %s\nRespond ONLY as %s:", currentInput, char.Name, char.Name)))

						ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
						defer cancel()

						llmResponseContent, err = mmSvc.GenerateResponseMultimodal(ctx, mmMessages, usedLLMModel, *m.apiConfig)
						// If multimodal fails, fall back to text-only path
						if err != nil {
							NexusLog("WARN", "llm", fmt.Sprintf("multimodal failed (%v), falling back to text path", err))
							promptBuilder.WriteString("\n--- 💬 CONVERSATION HISTORY ---\n")
							for _, msg := range budgetedMessages {
								if msg.Recipient != "" && msg.Recipient != char.Name && msg.Sender != char.Name {
									continue
								}
								if msg.Type == MessageTypeImage {
									if desc, ok := msg.Metadata["vision_description"]; ok && desc != "" {
										promptBuilder.WriteString(fmt.Sprintf("%s shared an image — [IMAGE: %s]\n", msg.Sender, desc))
									}
								} else if msg.Type != MessageTypeSummary {
									promptBuilder.WriteString(fmt.Sprintf("%s: %s\n", msg.Sender, msg.Content))
								}
							}
							promptBuilder.WriteString(fmt.Sprintf("\n### RESPONSE: %s: ", char.Name))
							llmResponseContent, err = m.llmService.GenerateResponse(ctx, promptBuilder.String(), usedLLMModel, *m.apiConfig)
						}
					} else {
						// Text-only path (no images or provider doesn't support multimodal)
						promptBuilder.WriteString("\n--- 💬 CONVERSATION HISTORY ---\n")
						for _, msg := range budgetedMessages {
							if msg.Recipient != "" && msg.Recipient != char.Name && msg.Sender != char.Name {
								continue
							}
							if msg.Type == MessageTypeSummary {
								promptBuilder.WriteString(fmt.Sprintf("[SUMMARY]: %s\n", msg.Content))
							} else if msg.Type == MessageTypeImage {
								if desc, ok := msg.Metadata["vision_description"]; ok && desc != "" {
									promptBuilder.WriteString(fmt.Sprintf("%s shared an image — [IMAGE: %s]\n", msg.Sender, desc))
								} else {
									promptBuilder.WriteString(fmt.Sprintf("%s shared an image: %s\n", msg.Sender, msg.Metadata["image_path"]))
								}
							} else {
								promptBuilder.WriteString(fmt.Sprintf("%s: %s\n", msg.Sender, msg.Content))
							}
						}
						promptBuilder.WriteString(fmt.Sprintf("\n### RESPONSE: %s: ", char.Name))

						ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
						defer cancel()

						llmResponseContent, err = m.llmService.GenerateResponse(ctx, promptBuilder.String(), usedLLMModel, *m.apiConfig)
					}
					if err != nil {
						NexusLog("ERROR", "llm", fmt.Sprintf("GenerateResponse provider=%s model=%s char=%s: %v", m.selectedLLMProvider, usedLLMModel, char.Name, err))
						return ChatMsg{Error: fmt.Errorf("failed to get LLM response from '%s': %w", char.Name, err)}
					}

					// Plugin: afterResponseGenerate
					if char.PluginEnabled && char.PluginCode != "" {
						result := RunAfterResponseGenerate(char.PluginCode, llmResponseContent)
						if result.Error != nil {
							fmt.Printf("Plugin warning (after): %v\n", result.Error)
						} else if result.Modified {
							llmResponseContent = result.Content
						}
					}

					responseMsg := Message{
						ID:        GenerateRandomID(),
						Sender:    char.Name,
						Content:   llmResponseContent,
						Timestamp: time.Now().Unix(),
						Type:      MessageTypeCharacter,
					}
					SignChatMessage(&responseMsg, char, m.user)

					memEntry := &MemoryEntry{Content: responseMsg.Content, Type: "episodic", Source: "Chat response"}
					m.memoryManager.AddMemory(m.user, char.ID, memEntry, m.apiConfig)
					return ChatMsg{Message: responseMsg}
				}
			}(m.activeCharacterID, input)
				return m, llmCmd
			}

			// Rolling summary check
			if len(m.messages) >= 20 {
				go func() tea.Msg {
					summaryPrompt := "Summarize the conversation so far, focusing on key themes and character interactions."
					rimmedMsgs, err := m.memoryManager.GenerateRollingSummary(m.user, m.selectedCharacterID, m.messages, summaryPrompt, m.apiConfig)
					if err != nil {
						return ChatMsg{Error: fmt.Errorf("failed to generate rolling summary: %w", err)}
					}

					allMemories, _ := m.memoryManager.RetrieveMemories(m.user, m.selectedCharacterID, "summary", 100, m.apiConfig)
					var summaries []MemoryEntry
					for _, mem := range allMemories {
						if mem.Type == MemoryTypeSummary && mem.Source == "rolling_summary" {
							summaries = append(summaries, mem)
						}
					}

					if len(summaries) >= 5 {
						m.memoryManager.(*LocalMemoryManager).HierarchicalSummarize(m.user, m.selectedCharacterID, summaries, m.apiConfig)
					}

					m.messages = rimmedMsgs
					return nil
				}()
			}

			// Auto-image generation
			if m.apiConfig.AutoImageGeneration && len(m.messages)%m.apiConfig.AutoImageFrequency == 0 {
				go func() tea.Msg {
					visualPrompt := "Describe the current visual scene of this roleplay in detail for an image generator: "
					history := ""
					for i := len(m.messages) - 5; i < len(m.messages); i++ {
						if i >= 0 {
							history += m.messages[i].Sender + ": " + m.messages[i].Content + "\n"
						}
					}

					ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
					defer cancel()

					refinedPrompt, err := m.llmService.GenerateResponse(ctx, visualPrompt+history, LLMModel(m.selectedLLMModel), *m.apiConfig)
					if err != nil {
						return ChatMsg{Error: fmt.Errorf("failed to refine image prompt: %w", err)}
					}

					imageService, err := ImageFactory(m.apiConfig.SelectedImageProvider, *m.apiConfig)
					if err != nil {
						return ChatMsg{Error: fmt.Errorf("failed to create image service: %w", err)}
					}

					// Apply visual style presets
					styledPrompt := buildStyledImagePrompt(refinedPrompt, m.apiConfig.ImageStyle)

					imgModel := ImageModel(m.apiConfig.GetActiveImageModel())
					imgBase64, err := imageService.GenerateImage(ctx, styledPrompt, imgModel, *m.apiConfig)
					if err != nil {
						return ChatMsg{Error: fmt.Errorf("auto-image generation failed: %w", err)}
					}

					imgBytes, err := base64.StdEncoding.DecodeString(imgBase64)
					if err != nil {
						return ChatMsg{Error: fmt.Errorf("failed to decode auto-image: %w", err)}
					}

					imageID := GenerateRandomID()
					if err := DBPut(m.user.DB, m.user.EncryptionKey, "images", imageID, imgBytes); err != nil {
						return ChatMsg{Error: fmt.Errorf("failed to store auto-image in DB: %w", err)}
					}

					inlineMsgID := GenerateRandomID()

					// Async: describe → RAG (dual-front, same as /upload and /image)
					capturedUser := m.user
					capturedCharID := m.activeCharacterID
					capturedMM := m.memoryManager
					capturedCfg := m.apiConfig
					capturedLLM := m.llmService
					capturedPrompt := refinedPrompt
					capturedImgBytes := imgBytes
					go func() {
						vctx, vcancel := context.WithTimeout(context.Background(), 180*time.Second)
						defer vcancel()
						visionSvc, visionModel := GetVisionService(capturedLLM, capturedCfg)
						if visionModel == "" { visionModel = LLMModel(capturedCfg.GetVisionProviderModel()) }
						desc, err := visionSvc.AnalyzeImage(vctx, capturedImgBytes,
							"Look at this image carefully. State EXACTLY what is depicted: the main subject (person, place, object, scene), specific details, colors, mood, and setting. Name things precisely — say 'Eiffel Tower at sunset' not 'a tall structure'. Be specific and concrete in 3-4 sentences.", visionModel, *capturedCfg)
						if err == nil {
							capturedMM.AddMemory(capturedUser, capturedCharID, &MemoryEntry{
								Content: fmt.Sprintf("[VISUAL MEMORY: auto scene]\nScene: %s\n%s", capturedPrompt, desc),
								Type:    MemoryTypeSemantic, Source: "vision:auto-scene",
							}, capturedCfg)
							globalProgram.Send(UpdateImageDescMsg{MessageID: inlineMsgID, Description: desc})
						}
					}()

					imageMsg := Message{
						ID:        inlineMsgID,
						Sender:    "System",
						Content:   "Auto-generated scene visualization",
						Type:      MessageTypeImage,
						Timestamp: time.Now().Unix(),
						Metadata:  map[string]string{"image_id": imageID},
					}
					return ChatMsg{Message: imageMsg}
				}()
			}
		}

	case UpdateImageDescMsg:
		for i := range m.messages {
			if m.messages[i].ID == msg.MessageID {
				if m.messages[i].Metadata == nil {
					m.messages[i].Metadata = map[string]string{}
				}
				m.messages[i].Metadata["vision_description"] = msg.Description
				m.viewport.SetContent(m.renderMessages())
				break
			}
		}
		return m, nil

	case ChatMsg:
		if msg.Error != nil {
			m.err = msg.Error
		} else {
			m.messages = append(m.messages, msg.Message)
			m.viewport.SetContent(m.renderMessages())
			m.viewport.GotoBottom()

			// PHASE H: Agentic Tag Parsing
			if msg.Message.Type == MessageTypeCharacter {
				agentCmds := parseAgentActions(msg.Message.Content)
				if len(agentCmds) > 0 {
					var agentActionsCmds []tea.Cmd
					for _, cmdStr := range agentCmds {
						command, args, isCommand := parseCommand(cmdStr)
						if isCommand {
							agentActionsCmds = append(agentActionsCmds, m.handleCommand(command, args))
						}
					}
					if len(agentActionsCmds) > 0 {
						// Show system note about triggered actions
						sysMsg := Message{
							ID: GenerateRandomID(), Sender: "System",
							Content:   fmt.Sprintf("🤖 Agent triggered %d automated actions.", len(agentActionsCmds)),
							Timestamp: time.Now().Unix(), Type: MessageTypeSystem,
						}
						m.messages = append(m.messages, sysMsg)
						return m, tea.Batch(agentActionsCmds...)
					}
				}
			}
		}
		return m, nil

	case RequestExecApprovalMsg:
		am := newApprovalModel(msg.CharID, msg.Command)
		m.approvalModel = &am
		return m, m.approvalModel.Init()

	case TermApprovalMsg:
		m.approvalModel = nil
		if !msg.Approved {
			sys := Message{ID: GenerateRandomID(), Type: MessageTypeSystem,
				Sender: "SYSTEM", Content: "⛔ Command denied by user.",
				Timestamp: time.Now().Unix()}
			m.messages = append(m.messages, sys)
			m.viewport.SetContent(m.renderMessages())
			return m, nil
		}
		// Execute approved command
		result := RunInWorkspace(msg.CharID, msg.Command)
		content := fmt.Sprintf("💻 $ %s\n%s\n[exit %d, %.2fs]",
			result.Command, result.Output, result.ExitCode, result.Duration.Seconds())
		sys := Message{ID: GenerateRandomID(), Type: MessageTypeSystem,
			Sender: "SYSTEM", Content: content, Timestamp: time.Now().Unix()}
		m.messages = append(m.messages, sys)
		m.viewport.SetContent(m.renderMessages())
		return m, nil
	}

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)
	m.textInput, cmd = m.textInput.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}
