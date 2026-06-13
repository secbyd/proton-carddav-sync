// Package crypto provides AES-256-GCM encryption helpers and PBKDF2 key
// derivation. All random material is sourced from crypto/rand — never
// math/rand (go-defensive: crypto/rand for keys).
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/pbkdf2"
)

// Sentinel errors.
var (
	// ErrCiphertextTooShort is returned when a ciphertext is shorter than the
	// GCM nonce size.
	ErrCiphertextTooShort = errors.New("ciphertext too short")
)

const (
	keyLen     = 32 // AES-256
	saltLen    = 32
	pbkdf2Iter = 200_000
)

// DeriveKey derives a 256-bit AES key from password using PBKDF2-SHA256.
// It generates a fresh random salt and returns (key, salt, error).
func DeriveKey(password string) (key []byte, salt []byte, err error) {
	salt = make([]byte, saltLen)
	if _, err = io.ReadFull(rand.Reader, salt); err != nil {
		return nil, nil, fmt.Errorf("generate salt: %w", err)
	}
	key = pbkdf2.Key([]byte(password), salt, pbkdf2Iter, keyLen, sha256.New)
	return key, salt, nil
}

// DeriveKeyWithSalt re-derives a key from password and an existing salt.
func DeriveKeyWithSalt(password string, salt []byte) []byte {
	return pbkdf2.Key([]byte(password), salt, pbkdf2Iter, keyLen, sha256.New)
}

// Encrypt encrypts plaintext with AES-256-GCM.
// The returned ciphertext has the 12-byte nonce prepended.
func Encrypt(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt decrypts a nonce-prefixed AES-256-GCM ciphertext.
func Decrypt(key, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}

	if len(ciphertext) < gcm.NonceSize() {
		return nil, ErrCiphertextTooShort
	}

	nonce, ct := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return plaintext, nil
}
