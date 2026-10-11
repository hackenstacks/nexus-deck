package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"path/filepath"
	"time"
)


// LibraryFile represents a file stored in the user's library.
type LibraryFile struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`          // Display name of the file
	Type         string    `json:"type"`          // e.g., "image", "chat_log", "character_card", "json", "text"
	OriginalPath string    `json:"originalPath"`  // Original path if uploaded, empty if generated
	StoredPath   string    `json:"storedPath"`    // Path to the encrypted content in data_store
	Timestamp    time.Time `json:"timestamp"`
}

// --- Helper Functions (Assuming these exist elsewhere) ---

// GenerateRandomID generates a unique ID for a file.
func GenerateRandomID() string {
	// Implementation omitted for brevity
	return "random-id-" + fmt.Sprint(time.Now().UnixNano())
}



// --- Core Functions ---

// SharedFile represents a file encrypted for sharing, including its metadata, encrypted content, salt, and nonce.
type SharedFile struct {
	Name         string `json:"name"`
	Type         string `json:"type"`
	EncryptedData []byte `json:"encryptedData"`
	Salt         []byte `json:"salt"`
	Nonce        []byte `json:"nonce"`
}

// AddFileToLibrary adds a file to the user's encrypted library.
// It reads the file content, encrypts it, saves it, and then saves metadata.
func AddFileToLibrary(user *User, originalFilePath, desiredFileName, fileType string) (string, error) {
	if user == nil || user.Username == "" {
		return "", fmt.Errorf("invalid user for adding file to library")
	}

	fileContent, err := ioutil.ReadFile(originalFilePath)
	if err != nil {
		return "", fmt.Errorf("failed to read original file '%s': %w", originalFilePath, err)
	}

	fileID := GenerateRandomID()
	// Save the encrypted content
	err = SaveEncryptedData(user, user.Username, "library_content", fileID, fileContent)
	if err != nil {
		return "", fmt.Errorf("failed to save encrypted file content: %w", err)
	}

	// Create and save metadata
	libraryFile := LibraryFile{
		ID:           fileID,
		Name:         desiredFileName,
		Type:         fileType,
		OriginalPath: originalFilePath,
		StoredPath:   filepath.Join(GetUserDataPath(user.Username), "library_content", fileID+".bin"), // Path to the actual encrypted content
		Timestamp:    time.Now(),
	}

	metadataBytes, err := json.Marshal(libraryFile)
	if err != nil {
		return "", fmt.Errorf("failed to marshal library file metadata: %w", err)
	}

	err = SaveEncryptedData(user, user.Username, "library_metadata", fileID, metadataBytes)
	if err != nil {
		return "", fmt.Errorf("failed to save encrypted library file metadata: %w", err)
	}

	return fileID, nil
}

// EncryptAndMarshalSharedFile encrypts file content with a sharing password and marshals it into a sharable JSON.
func EncryptAndMarshalSharedFile(outputPath string, file *LibraryFile, content []byte, sharingPassword string) error {
	if file == nil || content == nil || sharingPassword == "" {
		return fmt.Errorf("invalid parameters for sharing file")
	}

	// Generate salt for sharing password
	sharingSalt, err := GenerateSalt(saltLen)
	if err != nil {
		return fmt.Errorf("failed to generate salt for sharing: %w", err)
	}

	// Derive key from sharing password and salt
	sharingEncryptionKey, err := DeriveKey(sharingPassword, sharingSalt)
	if err != nil {
		return fmt.Errorf("failed to derive encryption key for sharing: %w", err)
	}

	// Encrypt the content with the sharing key
	block, err := aes.NewCipher(sharingEncryptionKey)
	if err != nil {
		return fmt.Errorf("failed to create cipher for sharing: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("failed to create GCM for sharing: %w", err)
	}

	sharingNonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, sharingNonce); err != nil {
		return fmt.Errorf("failed to generate nonce for sharing: %w", err)
	}

	encryptedContent := gcm.Seal(sharingNonce, sharingNonce, content, nil)

	// Create shared file structure
	sharedFile := SharedFile{
		Name:         file.Name,
		Type:         file.Type,
		EncryptedData: encryptedContent,
		Salt:         sharingSalt,
		Nonce:        sharingNonce,
	}

	// Marshal into JSON
	sharedFileJSON, err := json.MarshalIndent(sharedFile, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal shared file JSON: %w", err)
	}

	// Write to output path
	if err := ioutil.WriteFile(outputPath, sharedFileJSON, 0644); err != nil {
		return fmt.Errorf("failed to write shared file to output path '%s': %w", outputPath, err)
	}

	return nil
}

// LoadFileFromLibrary loads a file's metadata and content from the user's encrypted library.
func LoadFileFromLibrary(user *User, fileID string) (*LibraryFile, []byte, error) {
	if user == nil || user.Username == "" {
		return nil, nil, fmt.Errorf("invalid user for loading file from library")
	}
	if fileID == "" {
		return nil, nil, fmt.Errorf("file ID cannot be empty")
	}

	// Load metadata
	metadataBytes, err := LoadEncryptedData(user, user.Username, "library_metadata", fileID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load encrypted library file metadata for ID '%s': %w", fileID, err)
	}

	var libraryFile LibraryFile
	if err := json.Unmarshal(metadataBytes, &libraryFile); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal library file metadata for ID '%s': %w", fileID, err)
	}

	// Load content
	contentBytes, err := LoadEncryptedData(user, user.Username, "library_content", fileID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load encrypted file content for ID '%s': %w", fileID, err)
	}

	return &libraryFile, contentBytes, nil
}

// ListLibraryFiles lists all files stored in the user's library from the database.
func ListLibraryFiles(user *User) ([]LibraryFile, error) {
	if user == nil || user.Username == "" {
		return nil, fmt.Errorf("invalid user for listing library files")
	}

	keys, err := ListEncryptedFiles(user, "library_metadata")
	if err != nil {
		return nil, err
	}

	var libraryFiles []LibraryFile
	for _, fileID := range keys {
		metadataBytes, err := LoadEncryptedData(user, user.Username, "library_metadata", fileID)
		if err != nil {
			fmt.Printf("Warning: Could not load metadata for library file ID %s: %v\n", fileID, err)
			continue
		}

		var libraryFile LibraryFile
		if err := json.Unmarshal(metadataBytes, &libraryFile); err != nil {
			fmt.Printf("Warning: Could not unmarshal metadata for library file ID %s: %v\n", fileID, err)
			continue
		}
		libraryFiles = append(libraryFiles, libraryFile)
	}
	return libraryFiles, nil
}

// DeleteLibraryFile removes a file and its metadata from the user's library in the database.
func DeleteLibraryFile(user *User, fileID string) error {
	if user == nil || user.Username == "" {
		return fmt.Errorf("invalid user for deleting file from library")
	}
	if fileID == "" {
		return fmt.Errorf("file ID cannot be empty")
	}

	// Delete content
	if err := DeleteEncryptedData(user, "library_content", fileID); err != nil {
		fmt.Printf("Warning: failed to delete library content for %s: %v\n", fileID, err)
	}

	// Delete metadata
	return DeleteEncryptedData(user, "library_metadata", fileID)
}

// DeleteFileFromLibrary deletes a file and its metadata from the user's library.
func DeleteFileFromLibrary(user *User, fileID string) error {
	return DeleteLibraryFile(user, fileID)
}
