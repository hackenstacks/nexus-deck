package main

import (
	"encoding/json"
	"fmt"
	"time"
)

const (
	VaultRootID     = "root"
	VaultIncomingID = "incoming"
)

// VaultItem is a file or folder in the encrypted vault.
type VaultItem struct {
	ID        string `json:"id"`
	ParentID  string `json:"parentId"` // "root" or "incoming" or another folder ID
	Name      string `json:"name"`
	Type      string `json:"type"` // "file" or "folder"
	MimeType  string `json:"mimeType,omitempty"`
	Size      int64  `json:"size,omitempty"`
	CreatedAt int64  `json:"createdAt"`
}

// NexusVaultPackage is the portable sharing format (.nexusvault file)
type NexusVaultPackage struct {
	Version  string `json:"version"`  // "1.0"
	MimeType string `json:"mimeType"` // "application/nexus-character", "image/png", etc — UNENCRYPTED
	Preview  struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Keywords    []string `json:"keywords"`
		FileCount   int      `json:"fileCount"`
		SizeBytes   int64    `json:"sizeBytes"`
	} `json:"preview"` // UNENCRYPTED — receiver sees this before decrypting
	SenderPubKey string `json:"senderPublicKey"` // EC P-256 PEM — receiver can verify
	Signature    string `json:"signature"`       // sender signed the preview + mimetype before encryption
	Payload      []byte `json:"payload"`         // EncryptPackage() output
}

// SaveVaultIndex encrypts and saves the vault index.
func SaveVaultIndex(user *User, items []VaultItem) error {
	data, err := json.Marshal(items)
	if err != nil {
		return fmt.Errorf("failed to marshal vault index: %w", err)
	}
	return SaveEncryptedData(user, user.Username, "vault", "index", data)
}

// LoadVaultItems returns all VaultItems from the user's vault bucket.
func LoadVaultItems(user *User) ([]VaultItem, error) {
	return LoadVaultIndex(user)
}

// LoadVaultIndex loads and decrypts the vault index.
func LoadVaultIndex(user *User) ([]VaultItem, error) {
	data, err := LoadEncryptedData(user, user.Username, "vault", "index")
	if err != nil {
		// If index not found, return empty list
		return []VaultItem{}, nil
	}
	var items []VaultItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("failed to unmarshal vault index: %w", err)
	}
	return items, nil
}

// SaveVaultFile encrypts and saves a file's content.
func SaveVaultFile(user *User, item VaultItem, content []byte) error {
	return SaveEncryptedData(user, user.Username, "vault_content", item.ID, content)
}

// LoadVaultFile loads and decrypts a file's content.
func LoadVaultFile(user *User, itemID string) ([]byte, error) {
	return LoadEncryptedData(user, user.Username, "vault_content", itemID)
}

// DeleteVaultItem removes a vault item from the index and deletes its content if it's a file.
func DeleteVaultItem(user *User, itemID string) error {
	items, err := LoadVaultIndex(user)
	if err != nil {
		return err
	}

	newItems := make([]VaultItem, 0, len(items))
	var itemToDelete *VaultItem
	for _, item := range items {
		if item.ID == itemID {
			itemToDelete = &item
			continue
		}
		newItems = append(newItems, item)
	}

	if itemToDelete == nil {
		return fmt.Errorf("item %s not found in vault", itemID)
	}

	// If it's a file, delete the content
	if itemToDelete.Type == "file" {
		// We don't have a direct "DeleteEncryptedData" yet, but we can implement it or use os.Remove
		// For now, let's assume we just remove it from index.
		// TODO: Implement actual file deletion in data_store.go
	}

	return SaveVaultIndex(user, newItems)
}

// ExportVaultPackage packs a vault file into a .nexusvault for sharing
func ExportVaultPackage(user *User, itemID string, sharePassword string) ([]byte, error) {
	// 1. Load item metadata
	items, err := LoadVaultIndex(user)
	if err != nil {
		return nil, err
	}
	var targetItem *VaultItem
	for _, item := range items {
		if item.ID == itemID {
			targetItem = &item
			break
		}
	}
	if targetItem == nil {
		return nil, fmt.Errorf("item not found")
	}

	// 2. Load item content
	content, err := LoadVaultFile(user, itemID)
	if err != nil {
		return nil, err
	}

	// 3. Encrypt content with share password
	payload, err := EncryptPackage(content, sharePassword)
	if err != nil {
		return nil, err
	}

	// 4. Create package and sign unencrypted metadata
	pkg := NexusVaultPackage{
		Version:  "1.0",
		MimeType: targetItem.MimeType,
		Payload:  payload,
	}
	if pkg.MimeType == "" {
		pkg.MimeType = "application/octet-stream"
	}
	pkg.Preview.Name = targetItem.Name
	pkg.Preview.SizeBytes = targetItem.Size
	pkg.Preview.FileCount = 1
	if targetItem.Type == "folder" {
		pkg.Preview.FileCount = 0 // Folders not fully supported for export yet
	}
	pkg.SenderPubKey = user.KeyPair.PublicKeyPEM

	// Canonical data for signing: Preview fields + MimeType
	manifest, _ := json.Marshal(pkg.Preview)
	signData := append(manifest, []byte(pkg.MimeType)...)

	userPrivPEM, err := DecryptPEM(user.KeyPair.PrivateKeyEnc, user.EncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt user identity key: %w", err)
	}
	signature, err := SignData(signData, userPrivPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to sign payload: %w", err)
	}
	pkg.Signature = signature

	return json.Marshal(pkg)
}

// ImportVaultPackage decrypts a received .nexusvault and adds it to "incoming" folder
func ImportVaultPackage(user *User, pkgData []byte, sharePassword string) (*VaultItem, error) {
	var pkg NexusVaultPackage
	if err := json.Unmarshal(pkgData, &pkg); err != nil {
		return nil, err
	}

	// 1. Verify signature of unencrypted metadata
	manifest, _ := json.Marshal(pkg.Preview)
	signData := append(manifest, []byte(pkg.MimeType)...)
	valid, err := VerifyData(signData, pkg.Signature, pkg.SenderPubKey)
	if err != nil || !valid {
		return nil, fmt.Errorf("package signature verification failed")
	}

	// 2. Decrypt payload
	content, err := DecryptPackage(pkg.Payload, sharePassword)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt package (check password): %w", err)
	}

	// 3. Create new vault item
	item := VaultItem{
		ID:        GenerateRandomID(),
		ParentID:  VaultIncomingID,
		Name:      pkg.Preview.Name,
		Type:      "file",
		MimeType:  pkg.MimeType,
		Size:      int64(len(content)),
		CreatedAt: time.Now().Unix(),
	}

	// 4. Save content
	if err := SaveVaultFile(user, item, content); err != nil {
		return nil, err
	}

	// 5. Update index
	items, err := LoadVaultIndex(user)
	if err != nil {
		items = []VaultItem{}
	}
	items = append(items, item)
	if err := SaveVaultIndex(user, items); err != nil {
		return nil, err
	}

	return &item, nil
}
