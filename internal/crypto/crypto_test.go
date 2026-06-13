package crypto

import (
	"encoding/base64"
	"testing"
)

func TestNewRandomKey(t *testing.T) {
	k1, err := NewRandomKey()
	if err != nil {
		t.Fatalf("NewRandomKey: %v", err)
	}

	raw, err := base64.RawURLEncoding.DecodeString(k1)
	if err != nil {
		t.Fatalf("key is not valid raw-url-base64: %v", err)
	}
	if len(raw) != keyLen {
		t.Fatalf("key entropy = %d bytes, want %d", len(raw), keyLen)
	}

	k2, err := NewRandomKey()
	if err != nil {
		t.Fatalf("NewRandomKey (2nd): %v", err)
	}
	if k1 == k2 {
		t.Fatal("two generated keys are identical; not random")
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key, salt, err := DeriveKey("master-passphrase")
	if err != nil {
		t.Fatalf("DeriveKey: %v", err)
	}

	plaintext := []byte("super-secret-credential")
	ct, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	got, err := Decrypt(key, ct)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if string(got) != string(plaintext) {
		t.Fatalf("round-trip = %q, want %q", got, plaintext)
	}

	// A key derived from the wrong passphrase must fail authentication.
	wrong := DeriveKeyWithSalt("wrong-passphrase", salt)
	if _, err := Decrypt(wrong, ct); err == nil {
		t.Fatal("Decrypt with wrong key succeeded; expected authentication failure")
	}
}
