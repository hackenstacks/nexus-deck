package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"

	bolt "go.etcd.io/bbolt"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/scrypt"
)

// UserKeyPair holds the user's stable EC P-256 identity key pair.
type UserKeyPair struct {
	PublicKeyPEM  string `json:"publicKeyPem"`  // stored plaintext
	PrivateKeyEnc string `json:"privateKeyEnc"` // AES-256-GCM encrypted hex
}

// User represents a user in the system
type User struct {
	Username       string      `json:"username"`
	HashedPassword string      `json:"hashedPassword"`
	EncryptionKey  []byte      `json:"-"` // This will hold the derived key for session, not marshaled
	KeyPair        UserKeyPair `json:"keyPair"`
	DB             *bolt.DB    `json:"-"` // open DB handle for session
}

const (
	usersDir      = "users"
	userFilePerm  = 0600 // Owner read/write only
	keyLen        = 32   // AES-256 key length
	saltLen       = 16   // 16 bytes for salt
	scryptN       = 32768
	scryptR       = 8
	scryptP       = 1
)

// GenerateUserKeyPair generates the user's stable EC identity key pair and stores it encrypted.
// Call this once when creating a new account.
func GenerateUserKeyPair(user *User) error {
	pubPEM, privPEM, err := GenerateECKeyPair()
	if err != nil {
		return fmt.Errorf("failed to generate user key pair: %w", err)
	}
	encPriv, err := EncryptPEM(privPEM, user.EncryptionKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt user private key: %w", err)
	}
	user.KeyPair = UserKeyPair{
		PublicKeyPEM:  pubPEM,
		PrivateKeyEnc: encPriv,
	}
	return nil
}

// ChangePassword re-derives the user's encryption key with a new password,
// re-encrypts the user's EC private key, and saves the updated user file.
// Caller must then call ReEncryptAllCharacters and ReEncryptAllSessions.
func ChangePassword(user *User, oldPassword, newPassword string) error {
	// 1. Verify old password
	if !CheckPasswordHash(oldPassword, user.HashedPassword) {
		return fmt.Errorf("incorrect current password")
	}
	// 2. Derive new key
	newSalt, err := GenerateSalt(saltLen)
	if err != nil {
		return err
	}
	newKey, err := DeriveKey(newPassword, newSalt)
	if err != nil {
		return err
	}
	// 3. Re-encrypt user EC private key
	oldPrivPEM, err := DecryptPEM(user.KeyPair.PrivateKeyEnc, user.EncryptionKey)
	if err != nil {
		return fmt.Errorf("failed to decrypt user private key: %w", err)
	}
	newEncPriv, err := EncryptPEM(oldPrivPEM, newKey)
	if err != nil {
		return fmt.Errorf("failed to re-encrypt user private key: %w", err)
	}
	user.KeyPair.PrivateKeyEnc = newEncPriv
	// 4. Update user
	user.EncryptionKey = newKey
	return SaveUser(user, newPassword)
}

// HashPassword hashes a password using bcrypt
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// CheckPasswordHash checks if a hashed password matches a plaintext password
func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// GenerateSalt generates a random salt for key derivation
func GenerateSalt(n int) ([]byte, error) {
	bytes := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, bytes); err != nil {
		return nil, err
	}
	return bytes, nil
}

// DeriveKey derives an encryption key from a password and salt using scrypt
func DeriveKey(password string, salt []byte) ([]byte, error) {
	key, err := scrypt.Key([]byte(password), salt, scryptN, scryptR, scryptP, keyLen)
	if err != nil {
		return nil, err
	}
	return key, nil
}

// Encrypt encrypts data using AES-256 GCM
func Encrypt(plaintext []byte, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt decrypts data using AES-256 GCM
func Decrypt(ciphertext []byte, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}
	return plaintext, nil
}

// SaveUser saves user data to the database
func SaveUser(user *User, password string) error {
	// Generate a random salt
	salt, err := GenerateSalt(saltLen)
	if err != nil {
		return fmt.Errorf("failed to generate salt: %w", err)
	}

	// Derive encryption key from password and salt
	encryptionKey, err := DeriveKey(password, salt)
	if err != nil {
		return fmt.Errorf("failed to derive key: %w", err)
	}

	// Hash the password for storage
	hashedPassword, err := HashPassword(password)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}
	user.HashedPassword = hashedPassword

	// Ensure the user has an EC identity key pair
	user.EncryptionKey = encryptionKey // Temporarily set for generation
	if user.KeyPair.PublicKeyPEM == "" {
		if err := GenerateUserKeyPair(user); err != nil {
			return fmt.Errorf("failed to generate user keys: %w", err)
		}
	}

	// Marshal user data (excluding EncryptionKey which is for session)
	userData, err := json.Marshal(user)
	if err != nil {
		return fmt.Errorf("failed to marshal user data: %w", err)
	}

	// Encrypt the user data using the derived key
	encryptedUserData, err := Encrypt(userData, encryptionKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt user data: %w", err)
	}

	// Save to DB
	db := user.DB
	shouldClose := false
	if db == nil {
		db, err = OpenNexusDB()
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		shouldClose = true
	}
	if shouldClose {
		defer db.Close()
	}

	err = DBSaveAuth(db, salt, encryptedUserData)
	if err != nil {
		return fmt.Errorf("failed to save auth to DB: %w", err)
	}

	return nil
}

// LoadUser loads user data from the database and initializes the session
func LoadUser(username, password string) (*User, error) {
	db, err := OpenNexusDB()
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	salt, encData, err := DBLoadAuth(db)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("user not found or auth corrupt: %w", err)
	}

	// Derive the encryption key using the provided password and extracted salt
	encryptionKey, err := DeriveKey(password, salt)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to derive key: %w", err)
	}

	// Decrypt the user data
	decryptedUserData, err := Decrypt(encData, encryptionKey)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to decrypt user data. Incorrect password or corrupted data: %w", err)
	}

	var loadedUser User
	if err := json.Unmarshal(decryptedUserData, &loadedUser); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to unmarshal user data: %w", err)
	}

	// Check if the provided password matches the stored hashed password
	if !CheckPasswordHash(password, loadedUser.HashedPassword) {
		db.Close()
		return nil, fmt.Errorf("invalid username or password")
	}

	// Set the derived encryption key and DB handle for session use
	loadedUser.EncryptionKey = encryptionKey
	loadedUser.DB = db

	// Trigger migration if legacy data exists
	_ = MigrateUserToDBIfNeeded(&loadedUser)

	return &loadedUser, nil
}

// UserExists checks if an account exists in the database
func UserExists(username string) bool {
	db, err := OpenNexusDB()
	if err != nil {
		return false
	}
	defer db.Close()

	_, _, err = DBLoadAuth(db)
	return err == nil
}