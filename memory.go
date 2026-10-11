package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"time"
)

// MemoryType defines the type of a memory entry.
type MemoryType string

const (
	MemoryTypeEpisodic  MemoryType = "episodic"  // Specific past experiences, session histories
	MemoryTypeSemantic  MemoryType = "semantic"  // Generalized, factual knowledge, user-specific profiles
	MemoryTypeProcedural MemoryType = "procedural" // Rules, skills, functional protocols (often in system prompts)
	MemoryTypeSummary   MemoryType = "summary"   // Summarized segments of conversation
	MemoryTypeSystem    MemoryType = "system"    // System-level notes or instructions
)

// MemoryEntry represents a single piece of stored memory.
type MemoryEntry struct {
	ID        string       `json:"id"`
	CharacterID string       `json:"characterId"` // Character this memory is associated with
	Content   string       `json:"content"`     // The actual text content of the memory
	Type      MemoryType   `json:"type"`        // Type of memory (episodic, semantic, etc.)
	Source    string       `json:"source"`      // Where this memory came from (e.g., "chat", "lorebook", "user_note")
	Timestamp int64        `json:"timestamp"`   // Unix timestamp when the memory was created
	Keywords  []string     `json:"keywords,omitempty"` // Relevant keywords for retrieval
	Embedding []float32    `json:"-"`           // Vector embedding of the content, not stored directly in JSON
	EmbeddingString string `json:"embeddingString,omitempty"` // Base64 encoded embedding for storage
	Score     float64      `json:"-"`           // Score for retrieval, not stored persistently
}

// MemoryManager defines the interface for managing various types of memories.
type MemoryManager interface {
	AddMemory(user *User, characterID string, entry *MemoryEntry, apiConfig *APIConfig) error
	RetrieveMemories(user *User, characterID string, query string, limit int, apiConfig *APIConfig) ([]MemoryEntry, error)
	// UpdateMemory(user *User, entry *MemoryEntry, apiConfig *APIConfig) error
	// DeleteMemory(user *User, memoryID string) error
	GenerateRollingSummary(user *User, characterID string, messages []Message, systemPrompt string, apiConfig *APIConfig) ([]Message, error)
}

// LocalMemoryManager implements MemoryManager for local, file-based storage.
type LocalMemoryManager struct {
	embeddingService EmbeddingService
	summarizer       Summarizer
}

// NewLocalMemoryManager creates a new LocalMemoryManager.
func NewLocalMemoryManager(es EmbeddingService, s Summarizer) *LocalMemoryManager {
	return &LocalMemoryManager{
		embeddingService: es,
		summarizer:       s,
	}
}

// cosineSimilarity calculates the cosine similarity between two vectors.
func cosineSimilarity(vec1, vec2 []float32) float64 {
	if len(vec1) != len(vec2) {
		return 0.0
	}

	var dotProduct, magnitude1, magnitude2 float64
	for i := 0; i < len(vec1); i++ {
		dotProduct += float64(vec1[i] * vec2[i])
		magnitude1 += float64(vec1[i] * vec1[i])
		magnitude2 += float64(vec2[i] * vec2[i])
	}

	if magnitude1 == 0 || magnitude2 == 0 {
		return 0.0
	}

	return dotProduct / (math.Sqrt(magnitude1) * math.Sqrt(magnitude2))
}

// AddMemory adds a new memory entry, generates its embedding, and saves it to the character's memory array in the database.
func (mm *LocalMemoryManager) AddMemory(user *User, characterID string, entry *MemoryEntry, apiConfig *APIConfig) error {
	if user == nil || characterID == "" || entry == nil {
		return fmt.Errorf("invalid parameters for AddMemory")
	}
	if entry.ID == "" {
		entry.ID = GenerateRandomID()
	}
	entry.CharacterID = characterID
	entry.Timestamp = time.Now().Unix()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Generate embedding
	embedding, err := mm.embeddingService.CreateEmbedding(ctx, entry.Content)
	if err != nil {
		return fmt.Errorf("failed to create embedding for memory entry: %w", err)
	}
	// Encode embedding to base64 string for storage
	embeddingBytes, err := json.Marshal(embedding)
	if err != nil {
		return fmt.Errorf("failed to marshal embedding: %w", err)
	}
	entry.EmbeddingString = fmt.Sprintf("%x", embeddingBytes) // Store as hex string, not base64 directly

	// Load existing memories for this character
	memories, err := mm.loadAllCharacterMemories(user, characterID)
	if err != nil {
		return fmt.Errorf("failed to load existing memories: %w", err)
	}

	// Append new memory
	memories = append(memories, *entry)

	// Marshal memory array
	memoryBytes, err := json.Marshal(memories)
	if err != nil {
		return fmt.Errorf("failed to marshal memory array: %w", err)
	}

	return SaveEncryptedData(user, user.Username, "memories", characterID, memoryBytes)
}

// RetrieveMemories retrieves relevant memories based on a query.
func (mm *LocalMemoryManager) RetrieveMemories(user *User, characterID string, query string, limit int, apiConfig *APIConfig) ([]MemoryEntry, error) {
	if user == nil || characterID == "" || query == "" {
		return nil, fmt.Errorf("invalid parameters for RetrieveMemories")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Generate embedding for the query
	queryEmbedding, err := mm.embeddingService.CreateEmbedding(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to create embedding for query: %w", err)
	}

	// Load all memories for the character
	allMemories, err := mm.loadAllCharacterMemories(user, characterID)
	if err != nil {
		return nil, fmt.Errorf("failed to load all character memories: %w", err)
	}

	// Calculate similarity scores
	for i := range allMemories {
		var storedEmbedding []float32
		if allMemories[i].EmbeddingString != "" {
			decoded, decodeErr := hex.DecodeString(allMemories[i].EmbeddingString)
			if decodeErr != nil {
				fmt.Printf("Warning: Failed to decode embedding string for memory %s: %v\n", allMemories[i].ID, decodeErr)
				continue
			}
			if err := json.Unmarshal(decoded, &storedEmbedding); err != nil {
				fmt.Printf("Warning: Failed to unmarshal embedding for memory %s: %v\n", allMemories[i].ID, err)
				continue
			}
			allMemories[i].Embedding = storedEmbedding
		}
		
		if len(allMemories[i].Embedding) > 0 {
			allMemories[i].Score = cosineSimilarity(queryEmbedding, allMemories[i].Embedding)
		} else {
			allMemories[i].Score = 0.0 // No embedding, no score
		}
	}

	// Sort by score in descending order
	sort.Slice(allMemories, func(i, j int) bool {
		return allMemories[i].Score > allMemories[j].Score
	})

	// Return top 'limit' memories
	if len(allMemories) < limit {
		return allMemories, nil
	}
	return allMemories[:limit], nil
}

// ListFilesInDirectory returns filenames with the given extension in a directory.
func ListFilesInDirectory(dirPath, ext string) ([]string, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ext) {
			files = append(files, e.Name())
		}
	}
	return files, nil
}

// loadAllCharacterMemories is a helper to load all memory entries for a character from the database.
func (mm *LocalMemoryManager) loadAllCharacterMemories(user *User, characterID string) ([]MemoryEntry, error) {
	memoryBytes, err := LoadEncryptedData(user, user.Username, "memories", characterID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return []MemoryEntry{}, nil
		}
		return nil, fmt.Errorf("failed to load encrypted memory data: %w", err)
	}

	var memories []MemoryEntry
	if err := json.Unmarshal(memoryBytes, &memories); err != nil {
		return nil, fmt.Errorf("failed to unmarshal memory entries: %w", err)
	}
	return memories, nil
}

// CognitiveWorkspace proactively curates and auto-links memories (A-Mem).
func (mm *LocalMemoryManager) CognitiveWorkspace(user *User, characterID string, apiConfig *APIConfig) error {
	allMemories, err := mm.loadAllCharacterMemories(user, characterID)
	if err != nil {
		return err
	}

	fmt.Printf("🧠 A-Mem: Evolving memory for character %s...\n", characterID)

	// 1. Auto-linking: Find memories with shared keywords or similar content
	for i := 0; i < len(allMemories); i++ {
		for j := i + 1; j < len(allMemories); j++ {
			m1 := &allMemories[i]
			m2 := &allMemories[j]
			
			// Simple shared keyword logic for auto-linking
			shared := 0
			for _, k1 := range m1.Keywords {
				for _, k2 := range m2.Keywords {
					if k1 == k2 {
						shared++
					}
				}
			}
			
			if shared > 0 {
				// Create an atomic link entry if not already present
				linkID := fmt.Sprintf("link_%s_%s", m1.ID, m2.ID)
				linkEntry := &MemoryEntry{
					ID:          linkID,
					CharacterID: characterID,
					Content:     fmt.Sprintf("🔗 Connection: Memory '%s' and '%s' share %d context points.", m1.ID, m2.ID, shared),
					Type:        "link",
					Source:      "a-mem_autolink",
					Timestamp:   time.Now().Unix(),
					Keywords:    []string{m1.ID, m2.ID, "auto-link"},
				}
				mm.AddMemory(user, characterID, linkEntry, apiConfig)
			}
		}
	}

	// 2. Pruning: Logic to prune redundant summaries or very old, low-relevance episodic memories
	if len(allMemories) > 1000 {
		sort.Slice(allMemories, func(i, j int) bool {
			return allMemories[i].Timestamp > allMemories[j].Timestamp
		})
		// Keep only the most recent 1000 for efficiency
		// In a production system, we'd archive the rest to a separate "deep storage"
	}
	return nil
}

// VirtualContextPage retrieves a "page" of memories that fit within a token budget.
func (mm *LocalMemoryManager) VirtualContextPage(user *User, characterID string, query string, tokenBudget int, apiConfig *APIConfig) ([]MemoryEntry, error) {
	// Retrieve high-relevance memories
	memories, err := mm.RetrieveMemories(user, characterID, query, 20, apiConfig)
	if err != nil {
		return nil, err
	}

	var pagedMemories []MemoryEntry
	currentTokens := 0
	
	// Rough token estimate (4 chars per token)
	for _, mem := range memories {
		tokens := len(mem.Content) / 4
		if currentTokens+tokens < tokenBudget {
			pagedMemories = append(pagedMemories, mem)
			currentTokens += tokens
		} else {
			break // Budget reached
		}
	}
	
	return pagedMemories, nil
}

// ImportMemoryFromSession imports messages from another session that were sent by characterID.
// Stores each message as MemoryTypeEpisodic, then generates a rolling summary.
func (mm *LocalMemoryManager) ImportMemoryFromSession(
	user *User, characterID string,
	sessionMessages []Message, apiConfig *APIConfig,
) (int, error) {
	count := 0
	for _, msg := range sessionMessages {
		// Only import messages that belong to this character
		if msg.Sender != characterID && msg.SignedBy != characterID {
			continue
		}
		if msg.Content == "" {
			continue
		}
		err := mm.AddMemory(user, characterID, &MemoryEntry{
			ID:        GenerateRandomID(),
			Content:   msg.Content,
			Type:      MemoryTypeEpisodic,
			Timestamp: msg.Timestamp,
			Source:    "imported_session",
		}, apiConfig)
		if err != nil {
			continue
		}
		count++
	}
	return count, nil
}

// ImportLore chunks lore text and stores each chunk as MemoryTypeSemantic (RAG-ready).
func (mm *LocalMemoryManager) ImportLore(
	user *User, characterID string,
	loreName, loreContent string, apiConfig *APIConfig,
) (int, error) {
	chunks := chunkText(loreContent, 1000, 200)
	for _, chunk := range chunks {
		err := mm.AddMemory(user, characterID, &MemoryEntry{
			ID:        GenerateRandomID(),
			Content:   chunk,
			Type:      MemoryTypeSemantic,
			Timestamp: time.Now().Unix(),
			Source:    "lore:" + loreName,
		}, apiConfig)
		if err != nil {
			return 0, err
		}
	}
	return len(chunks), nil
}

// chunkText splits text into overlapping chunks.
func chunkText(text string, chunkSize, overlap int) []string {
	var chunks []string
	runes := []rune(text)
	i := 0
	for i < len(runes) {
		end := i + chunkSize
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[i:end]))
		i += chunkSize - overlap
		if i+overlap >= len(runes) {
			break
		}
	}
	return chunks
}

func (mm *LocalMemoryManager) GenerateRollingSummary(user *User, characterID string, messages []Message, systemPrompt string, apiConfig *APIConfig) ([]Message, error) {
	if user == nil || characterID == "" || len(messages) == 0 {
		return nil, fmt.Errorf("invalid parameters for GenerateRollingSummary")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second) // Longer timeout for summarization
	defer cancel()

	// Construct a prompt for summarization
	var chatHistoryBuilder strings.Builder
	chatHistoryBuilder.WriteString("Please summarize the following conversation. Focus on key events, decisions, and character developments. Make it concise.\n\n")
	if systemPrompt != "" {
		chatHistoryBuilder.WriteString(fmt.Sprintf("System Instruction: %s\n\n", systemPrompt))
	}
	chatHistoryBuilder.WriteString("---\n")
	for _, msg := range messages {
		chatHistoryBuilder.WriteString(fmt.Sprintf("%s: %s\n", msg.Sender, msg.Content))
	}
	
	summarizationPrompt := chatHistoryBuilder.String()

	// Use LLM to summarize
	summarizedContent, err := mm.summarizer.Summarize(ctx, summarizationPrompt)
	if err != nil {
		return nil, fmt.Errorf("failed to get summarization from LLM: %w", err)
	}

	// Add the summary as a new memory entry
	summaryMemory := &MemoryEntry{
		ID:          GenerateRandomID(),
		CharacterID: characterID,
		Content:     summarizedContent,
		Type:        MemoryTypeSummary,
		Source:      "rolling_summary",
		Timestamp:   time.Now().Unix(),
		Keywords:    []string{"summary", characterID, "conversation_overview"},
	}
	if err := mm.AddMemory(user, characterID, summaryMemory, apiConfig); err != nil {
		return nil, fmt.Errorf("failed to save summary as memory: %w", err)
	}

	// Add summary as a system message to chat history
	summaryMessage := Message{
		ID: GenerateRandomID(), Sender: "System",
		Content:   fmt.Sprintf("✨ Conversation Summary: %s", summarizedContent),
		Timestamp: time.Now().Unix(),
		Type:      MessageTypeSummary,
	}

	// Trim old messages, keeping the summary and recent messages (e.g., last 5-10 messages)
	trimmedMessages := []Message{summaryMessage}
	recentMessagesCount := 10
	if len(messages) > recentMessagesCount {
		trimmedMessages = append(trimmedMessages, messages[len(messages)-recentMessagesCount:]...)
	} else {
		trimmedMessages = append(trimmedMessages, messages...)
	}

	return trimmedMessages, nil
}

// HierarchicalSummarize compresses multiple rolling summaries into a single master summary.
func (mm *LocalMemoryManager) HierarchicalSummarize(user *User, characterID string, summaries []MemoryEntry, apiConfig *APIConfig) {
	if len(summaries) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var sb strings.Builder
	sb.WriteString("Create a concise master summary from these conversation summaries:\n\n")
	for i, s := range summaries {
		sb.WriteString(fmt.Sprintf("[Summary %d]: %s\n", i+1, s.Content))
	}

	masterContent, err := mm.summarizer.Summarize(ctx, sb.String())
	if err != nil {
		fmt.Printf("HierarchicalSummarize error: %v\n", err)
		return
	}

	masterEntry := &MemoryEntry{
		ID:          GenerateRandomID(),
		CharacterID: characterID,
		Content:     masterContent,
		Type:        MemoryTypeSummary,
		Source:      "hierarchical_summary",
		Timestamp:   time.Now().Unix(),
		Keywords:    []string{"master_summary", characterID},
	}
	if err := mm.AddMemory(user, characterID, masterEntry, apiConfig); err != nil {
		fmt.Printf("HierarchicalSummarize save error: %v\n", err)
	}
}
