package main

import (
	"encoding/json"
	"fmt"
	"strings"
)

type LorebookEntry struct {
	ID      string   `json:"id"`
	Keys    []string `json:"keys"`    // trigger keywords
	Content string   `json:"content"` // injected lore text
}

type Lorebook struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Entries     []LorebookEntry `json:"entries"`
}

// FindRelevantLore scans last 5 messages for keyword matches from active lorebooks.
// Returns matched lore content joined by "\n---\n", capped at 1000 chars. Returns "" if no match.
func FindRelevantLore(messages []Message, lorebooks []Lorebook) string {
	if len(messages) == 0 || len(lorebooks) == 0 {
		return ""
	}

	// Combine last min(5, len) messages into recentText (lowercase)
	numMessages := 5
	if len(messages) < numMessages {
		numMessages = len(messages)
	}

	var sb strings.Builder
	for i := len(messages) - numMessages; i < len(messages); i++ {
		sb.WriteString(messages[i].Content)
		sb.WriteString(" ")
	}

	recentText := strings.ToLower(sb.String())
	// Cap recentText at 2000 chars
	if len(recentText) > 2000 {
		recentText = recentText[len(recentText)-2000:]
	}

	var triggered []string
	triggeredMap := make(map[string]bool)

	for _, lb := range lorebooks {
		for _, entry := range lb.Entries {
			for _, key := range entry.Keys {
				if key == "" {
					continue
				}
				if strings.Contains(recentText, strings.ToLower(key)) {
					if !triggeredMap[entry.ID] {
						triggered = append(triggered, entry.Content)
						triggeredMap[entry.ID] = true
					}
					break // Entry triggered, move to next entry
				}
			}
		}
	}

	if len(triggered) == 0 {
		return ""
	}

	result := strings.Join(triggered, "\n---\n")
	if len(result) > 1000 {
		result = result[:1000]
	}

	return result
}

// SaveLorebook encrypts and saves a lorebook to disk.
func SaveLorebook(user *User, lb *Lorebook) error {
	if user == nil || lb == nil || lb.ID == "" {
		return fmt.Errorf("invalid parameters for SaveLorebook")
	}

	jsonBytes, err := json.Marshal(lb)
	if err != nil {
		return fmt.Errorf("failed to marshal lorebook: %w", err)
	}

	return SaveEncryptedData(user, user.Username, "global/lorebooks", lb.ID, jsonBytes)
}

// LoadLorebooks loads all lorebooks for a user from the database.
func LoadLorebooks(user *User) ([]Lorebook, error) {
	if user == nil {
		return nil, fmt.Errorf("invalid user for LoadLorebooks")
	}

	keys, err := ListEncryptedFiles(user, "global/lorebooks")
	if err != nil {
		return nil, err
	}

	var lorebooks []Lorebook
	for _, lorebookID := range keys {
		jsonBytes, err := LoadEncryptedData(user, user.Username, "global/lorebooks", lorebookID)
		if err != nil {
			fmt.Printf("Warning: failed to load lorebook %s: %v\n", lorebookID, err)
			continue
		}

		var lb Lorebook
		if err := json.Unmarshal(jsonBytes, &lb); err != nil {
			fmt.Printf("Warning: failed to unmarshal lorebook %s: %v\n", lorebookID, err)
			continue
		}
		lorebooks = append(lorebooks, lb)
	}

	return lorebooks, nil
}

// DeleteLorebook removes a lorebook by ID from the database.
func DeleteLorebook(user *User, lorebookID string) error {
	if user == nil || lorebookID == "" {
		return fmt.Errorf("invalid parameters for DeleteLorebook")
	}

	return DeleteEncryptedData(user, "global/lorebooks", lorebookID)
}
