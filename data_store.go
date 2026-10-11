package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
)

const (
	userDataDirPerm = 0700 // Owner read/write/execute
)

// GetUserDataPath returns the legacy flat-file base path for a user's data.
// Used for migration from flat-files to bbolt and by callers not yet migrated.
func GetUserDataPath(username string) string {
	usernameHash := sha256.Sum256([]byte(username))
	return filepath.Join(usersDir, hex.EncodeToString(usernameHash[:]), "data")
}

// getBucketName maps old dataType string to bbolt bucket names.
func getBucketName(dataType string) string {
	switch dataType {
	case "chat_sessions":
		return "sessions"
	case "library_content":
		return "library"
	case "vault_content":
		return "vault"
	case "global/lorebooks":
		return "lorebooks"
	default:
		return dataType // characters, memories, images, knowledge, vault
	}
}

// SaveEncryptedData encrypts and saves generic data associated with the user in the database.
func SaveEncryptedData(user *User, username string, dataType string, dataName string, data []byte) error {
	if user == nil || user.EncryptionKey == nil || user.DB == nil {
		return fmt.Errorf("database session not active or encryption key not available")
	}

	bucket := getBucketName(dataType)
	return DBPut(user.DB, user.EncryptionKey, bucket, dataName, data)
}

// LoadEncryptedData loads and decrypts generic data associated with the user from the database.
func LoadEncryptedData(user *User, username string, dataType string, dataName string) ([]byte, error) {
	if user == nil || user.EncryptionKey == nil || user.DB == nil {
		return nil, fmt.Errorf("database session not active or encryption key not available")
	}

	bucket := getBucketName(dataType)
	data, err := DBGet(user.DB, user.EncryptionKey, bucket, dataName)
	if err != nil {
		return nil, fmt.Errorf("data '%s/%s' not found: %w", dataType, dataName, err)
	}
	return data, nil
}

// ListEncryptedFiles returns all keys in a given bucket.
func ListEncryptedFiles(user *User, dataType string) ([]string, error) {
	if user == nil || user.DB == nil {
		return nil, fmt.Errorf("database session not active")
	}

	bucket := getBucketName(dataType)
	return DBList(user.DB, bucket)
}

// DeleteEncryptedData removes a record from the database.
func DeleteEncryptedData(user *User, dataType string, dataName string) error {
	if user == nil || user.DB == nil {
		return fmt.Errorf("database session not active")
	}

	bucket := getBucketName(dataType)
	return DBDelete(user.DB, bucket, dataName)
}

// ReEncryptAllData re-encrypts every record in every bucket with a new key.
// Call this after ChangePassword.
func ReEncryptAllData(user *User, oldKey, newKey []byte) error {
	if user == nil || user.DB == nil {
		return fmt.Errorf("database session not active")
	}

	buckets := []string{"characters", "sessions", "memories", "images", "library", "vault", "lorebooks", "knowledge"}

	for _, bucket := range buckets {
		keys, err := DBList(user.DB, bucket)
		if err != nil {
			continue
		}

		for _, key := range keys {
			// Get with old key
			plain, err := DBGet(user.DB, oldKey, bucket, key)
			if err != nil {
				fmt.Printf("Warning: Failed to decrypt %s/%s during re-encryption: %v\n", bucket, key, err)
				continue
			}

			// Put with new key
			err = DBPut(user.DB, newKey, bucket, key, plain)
			if err != nil {
				return fmt.Errorf("failed to re-encrypt %s/%s: %w", bucket, key, err)
			}
		}
	}
	return nil
}

// ReEncryptCharacterKeys walks all saved characters and re-encrypts their EC private keys.
func ReEncryptCharacterKeys(user *User, username string, oldKey, newKey []byte) error {
	if user == nil || user.DB == nil {
		return fmt.Errorf("database session not active")
	}

	keys, err := DBList(user.DB, "characters")
	if err != nil {
		return nil
	}

	for _, key := range keys {
		data, err := DBGet(user.DB, oldKey, "characters", key)
		if err != nil {
			continue
		}

		var c Character
		if err := json.Unmarshal(data, &c); err != nil {
			continue
		}
		if c.PrivateKeyEncA == "" {
			continue
		}

		// Decrypt with old key, re-encrypt with new
		privPEM, err := DecryptPEM(c.PrivateKeyEncA, oldKey)
		if err != nil {
			continue
		}
		c.PrivateKeyEncA, err = EncryptPEM(privPEM, newKey)
		if err != nil {
			continue
		}

		updated, _ := json.Marshal(c)
		// Save with new key
		err = DBPut(user.DB, newKey, "characters", c.ID, updated)
		if err != nil {
			return fmt.Errorf("failed to save re-encrypted character %s: %w", c.ID, err)
		}
	}
	return nil
}
