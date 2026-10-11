package main

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// Character represents a character in the generator.
type Character struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	Personality  string            `json:"personality"`
	Scenario     string            `json:"scenario"`
	FirstMessage string            `json:"firstMessage,omitempty"`
	AlternateGreetings []string    `json:"alternateGreetings,omitempty"`
	Lorebook     map[string]string `json:"lorebook,omitempty"`
	PostHistoryInstructions string `json:"postHistoryInstructions,omitempty"`
	CreatorNotes string            `json:"creatorNotes,omitempty"`
	Tags         []string          `json:"tags,omitempty"`
	AvatarBase64 string            `json:"avatarBase64,omitempty"`
	// Security fields (Phase B — populated by NewCharacter via GenerateECKeyPair)
	PublicKeyPEM    string `json:"publicKeyPem,omitempty"`    // EC P-256 public key — plaintext
	PrivateKeyEncA  string `json:"privateKeyEncA,omitempty"`  // AES-encrypted private key (Option A)
	UserSignature   string `json:"signature,omitempty"`       // Signed by user's master EC key
	// Lorebook linking
	LorebookIDs     []string `json:"lorebookIds,omitempty"`   // Attached lorebook IDs
	// Plugin
	PluginEnabled bool   `json:"pluginEnabled,omitempty"`
	PluginCode    string `json:"pluginCode,omitempty"`
}

// CharacterInfo is a lighter version of Character for listing purposes.
type CharacterInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// NewCharacter creates a new character with a generated ID.
func NewCharacter() *Character {
	return &Character{
		ID:       uuid.New().String(),
		Lorebook: make(map[string]string),
	}
}

// GenerateAndAssignCharacterKeys generates EC keys for a character and stores them encrypted.
// Call this when saving a new character (PublicKeyPEM == "").
func GenerateAndAssignCharacterKeys(c *Character, user *User) error {
	pubPEM, privPEM, err := GenerateECKeyPair()
	if err != nil {
		return err
	}
	encPriv, err := EncryptPEM(privPEM, user.EncryptionKey)
	if err != nil {
		return err
	}
	c.PublicKeyPEM = pubPEM
	c.PrivateKeyEncA = encPriv
	return nil
}

// SillyTavernV2 represents the Silly Tavern V2 character format.
type SillyTavernV2 struct {
	Name               string            `json:"name"`
	Description        string            `json:"description"`
	Personality        string            `json:"personality"`
	Scenario           string            `json:"scenario"`
	FirstMes           string            `json:"first_mes"`
	MesExample         string            `json:"mes_example"`
	CreatorNotes       string            `json:"creator_notes"`
	SystemPrompt       string            `json:"system_prompt"`
	PostHistoryInstructions string       `json:"post_history_instructions"`
	AlternateGreetings []string          `json:"alternate_greetings"`
	CharacterBook      interface{}       `json:"character_book,omitempty"`
	Tags               []string          `json:"tags"`
	Creator            string            `json:"creator"`
}

// ExportCharacterToSillyTavern marshals a character into Silly Tavern V2 JSON format.
func ExportCharacterToSillyTavern(char *Character) ([]byte, error) {
	st := SillyTavernV2{
		Name:                    char.Name,
		Description:             char.Description,
		Personality:             char.Personality,
		Scenario:                char.Scenario,
		FirstMes:                char.FirstMessage,
		CreatorNotes:            char.CreatorNotes,
		PostHistoryInstructions: char.PostHistoryInstructions,
		AlternateGreetings:      char.AlternateGreetings,
		Tags:                    char.Tags,
	}
	return json.MarshalIndent(st, "", "  ")
}

// ImportCharacterFromSillyTavern unmarshals a Silly Tavern V2 JSON into a Character.
func ImportCharacterFromSillyTavern(data []byte) (*Character, error) {
	var st SillyTavernV2
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, err
	}
	char := NewCharacter()
	char.Name = st.Name
	char.Description = st.Description
	char.Personality = st.Personality
	char.Scenario = st.Scenario
	char.FirstMessage = st.FirstMes
	char.CreatorNotes = st.CreatorNotes
	char.PostHistoryInstructions = st.PostHistoryInstructions
	char.AlternateGreetings = st.AlternateGreetings
	char.Tags = st.Tags
	return char, nil
}

// SaveCharacter saves a character to the user's encrypted data store.
func SaveCharacter(user *User, char *Character) error {
	if user == nil || user.Username == "" {
		return fmt.Errorf("invalid user for saving character")
	}
	if char == nil || char.ID == "" || char.Name == "" {
		return fmt.Errorf("invalid character data for saving")
	}

	// PHASE B: Handle key generation and signing
	if char.PublicKeyPEM == "" {
		if err := GenerateAndAssignCharacterKeys(char, user); err != nil {
			return fmt.Errorf("failed to generate character keys: %w", err)
		}
	}

	// Re-sign character card with user's identity key
	if user.KeyPair.PrivateKeyEnc != "" && user.EncryptionKey != nil {
		userPrivPEM, err := DecryptPEM(user.KeyPair.PrivateKeyEnc, user.EncryptionKey)
		if err == nil {
			sig, err := SignCharacterCard(char, userPrivPEM)
			if err == nil {
				char.UserSignature = sig
			}
		}
	}

	charBytes, err := json.Marshal(char)
	if err != nil {
		return fmt.Errorf("failed to marshal character data: %w", err)
	}

	// Use character ID as dataName for unique storage
	return SaveEncryptedData(user, user.Username, "characters", char.ID, charBytes)
}

// LoadCharacter loads a character from the user's encrypted data store by ID.
func LoadCharacter(user *User, characterID string) (*Character, error) {
	if user == nil || user.Username == "" {
		return nil, fmt.Errorf("invalid user for loading character")
	}
	if characterID == "" {
		return nil, fmt.Errorf("character ID cannot be empty")
	}

	charBytes, err := LoadEncryptedData(user, user.Username, "characters", characterID)
	if err != nil {
		return nil, fmt.Errorf("failed to load encrypted character data for ID '%s': %w", characterID, err)
	}

	var char Character
	if err := json.Unmarshal(charBytes, &char); err != nil {
		return nil, fmt.Errorf("failed to unmarshal character data for ID '%s': %w", characterID, err)
	}
	return &char, nil
}

// ListCharacters lists all characters for a given user.
func ListCharacters(user *User) ([]CharacterInfo, error) {
	if user == nil || user.Username == "" {
		return nil, fmt.Errorf("invalid user for listing characters")
	}

	keys, err := ListEncryptedFiles(user, "characters")
	if err != nil {
		return nil, err
	}

	var charInfos []CharacterInfo
	for _, characterID := range keys {
		char, err := LoadCharacter(user, characterID)
		if err != nil {
			NexusLog("WARN", "character", fmt.Sprintf("removing unreadable character ID=%s: %v", characterID, err))
			// Auto-purge — it can't be decrypted so there's nothing to recover
			if delErr := DeleteEncryptedData(user, "characters", characterID); delErr != nil {
				NexusLog("ERROR", "character", fmt.Sprintf("failed to purge bad character ID=%s: %v", characterID, delErr))
			} else {
				NexusLog("INFO", "character", fmt.Sprintf("purged bad character ID=%s", characterID))
			}
			continue
		}
		if char.Name == "" || char.ID == "" {
			NexusLog("WARN", "character", fmt.Sprintf("skipping character with missing name/ID, ID=%s", characterID))
			continue
		}
		charInfos = append(charInfos, CharacterInfo{
			ID:   char.ID,
			Name: char.Name,
		})
	}
	return charInfos, nil
}

// DeleteCharacter deletes a character from the user's encrypted data store.
func DeleteCharacter(user *User, characterID string) error {
	if user == nil || user.Username == "" {
		return fmt.Errorf("invalid user for deleting character")
	}
	if characterID == "" {
		return fmt.Errorf("character ID cannot be empty")
	}

	return DeleteEncryptedData(user, "characters", characterID)
}