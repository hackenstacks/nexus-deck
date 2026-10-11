package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
)

// GenerateECKeyPair creates an EC P-256 key pair for a character or user.
func GenerateECKeyPair() (publicKeyPEM, privateKeyPEM string, err error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}

	// Marshal private key
	x509Encoded, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return "", "", err
	}
	pemEncoded := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: x509Encoded})

	// Marshal public key
	x509EncodedPub, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return "", "", err
	}
	pemEncodedPub := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: x509EncodedPub})

	return string(pemEncodedPub), string(pemEncoded), nil
}

// SignData signs data using an EC private key (ECDSA + SHA-256).
func SignData(data []byte, privateKeyPEM string) (string, error) {
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return "", fmt.Errorf("failed to parse PEM block containing the private key")
	}

	privateKey, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(data)
	r, s, err := ecdsa.Sign(rand.Reader, privateKey, hash[:])
	if err != nil {
		return "", err
	}

	signature := append(r.Bytes(), s.Bytes()...)
	return base64.StdEncoding.EncodeToString(signature), nil
}

// VerifyData verifies a signature against an EC public key.
func VerifyData(data []byte, signature string, publicKeyPEM string) (bool, error) {
	block, _ := pem.Decode([]byte(publicKeyPEM))
	if block == nil {
		return false, fmt.Errorf("failed to parse PEM block containing the public key")
	}

	pubInterface, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return false, err
	}

	publicKey, ok := pubInterface.(*ecdsa.PublicKey)
	if !ok {
		return false, fmt.Errorf("not an ECDSA public key")
	}

	sigBytes, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return false, err
	}

	if len(sigBytes) < 64 {
		return false, fmt.Errorf("signature too short")
	}

	rBytes := sigBytes[:len(sigBytes)/2]
	sBytes := sigBytes[len(sigBytes)/2:]

	r := new(big.Int).SetBytes(rBytes)
	s := new(big.Int).SetBytes(sBytes)

	hash := sha256.Sum256(data)
	return ecdsa.Verify(publicKey, hash[:], r, s), nil
}

// EncryptPEM encrypts a PEM string with an AES-256-GCM key.
func EncryptPEM(pemStr string, aesKey []byte) (string, error) {
	ciphertext, err := Encrypt([]byte(pemStr), aesKey)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(ciphertext), nil
}

// DecryptPEM decrypts an encrypted PEM string.
func DecryptPEM(encryptedHex string, aesKey []byte) (string, error) {
	ciphertext, err := hex.DecodeString(encryptedHex)
	if err != nil {
		return "", err
	}
	plaintext, err := Decrypt(ciphertext, aesKey)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

// CanonicalCharacterJSON returns a canonical JSON representation of a character for signing.
func CanonicalCharacterJSON(c *Character) ([]byte, error) {
	// Create a copy without UserSignature field
	type CharacterForSigning struct {
		ID           string            `json:"id"`
		Name         string            `json:"name"`
		Description  string            `json:"description"`
		Personality  string            `json:"personality"`
		Scenario     string            `json:"scenario"`
		PublicKeyPEM string            `json:"publicKeyPem,omitempty"`
	}

	s := CharacterForSigning{
		ID:           c.ID,
		Name:         c.Name,
		Description:  c.Description,
		Personality:  c.Personality,
		Scenario:     c.Scenario,
		PublicKeyPEM: c.PublicKeyPEM,
	}

	return json.Marshal(s)
}

// SignCharacterCard signs a character card using the user's master private key.
func SignCharacterCard(c *Character, userPrivKeyPEM string) (string, error) {
	data, err := CanonicalCharacterJSON(c)
	if err != nil {
		return "", err
	}
	return SignData(data, userPrivKeyPEM)
}

// VerifyCharacterCard verifies a character card signature.
func VerifyCharacterCard(c *Character, userPubKeyPEM string) (bool, error) {
	if c.UserSignature == "" {
		return false, fmt.Errorf("character has no signature")
	}
	data, err := CanonicalCharacterJSON(c)
	if err != nil {
		return false, err
	}
	return VerifyData(data, c.UserSignature, userPubKeyPEM)
}

// EncryptPackage encrypts data with a password for secure sharing (.nexusvault file).
func EncryptPackage(data []byte, password string) ([]byte, error) {
	salt, err := GenerateSalt(16)
	if err != nil {
		return nil, err
	}

	key, err := DeriveKey(password, salt)
	if err != nil {
		return nil, err
	}

	ciphertext, err := Encrypt(data, key)
	if err != nil {
		return nil, err
	}

	// Format: [16B salt][ciphertext (which includes nonce)]
	return append(salt, ciphertext...), nil
}

// DecryptPackage decrypts data from a .nexusvault package.
func DecryptPackage(pkg []byte, password string) ([]byte, error) {
	if len(pkg) < 16 {
		return nil, fmt.Errorf("package too short")
	}

	salt := pkg[:16]
	ciphertext := pkg[16:]

	key, err := DeriveKey(password, salt)
	if err != nil {
		return nil, err
	}

	return Decrypt(ciphertext, key)
}
