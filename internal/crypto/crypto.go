// Package crypto provides AES-256-GCM encryption for credentials stored at rest.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"

	"golang.org/x/crypto/pbkdf2"
)

const (
	pbkdf2Iter  = 100_000
	pbkdf2Salt  = "proton-carddav-sync-salt-v1" // static; key is per-password
	keyLen      = 32
)

// DeriveKey derives a 256-bit AES key from the given password using PBKDF2-SHA256.
func DeriveKey(password string) ([]byte, error) {
	if password == "" {
		return nil, fmt.Errorf("password must not be empty")
	}
	key := pbkdf2.Key([]byte(password), []byte(pbkdf2Salt), pbkdf2Iter, keyLen, sha256.New)
	return key, nil
}

// Encrypt encrypts plaintext with AES-256-GCM using key.
// The returned byte slice is: nonce (12 bytes) || ciphertext+tag.
func Encrypt(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt decrypts data produced by Encrypt.
func Decrypt(key, data []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	if len(data) < ns {
		return nil, fmt.Errorf("ciphertext too short")
	}
	return gcm.Open(nil, data[:ns], data[ns:], nil)
}
