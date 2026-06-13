// Package syncer — credential bootstrap from environment.
package syncer

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"os"

	"github.com/secbyd/proton-carddav-sync/internal/crypto"
	"github.com/secbyd/proton-carddav-sync/internal/db"
	"github.com/secbyd/proton-carddav-sync/internal/protonmail"
)

// EncryptionKeyEnv is the environment variable that holds the master key used
// to encrypt every credential stored in the local database (the Proton session
// and the CardDAV password). It is independent of the Proton account password.
const EncryptionKeyEnv = "PCS_ENCRYPTION_KEY"

// Credentials holds the decrypted material needed for a sync run. The derived
// AES key is retained (in memory only) so rotated Proton refresh tokens can be
// re-encrypted with EncryptRefreshToken and written back to the database.
type Credentials struct {
	Session     protonmail.Session
	CardDAVPass string
	key         []byte
}

// LoadDecryptedCredentials retrieves the encrypted credentials from the DB and
// decrypts them with a key derived from PCS_ENCRYPTION_KEY and the stored salt.
//
// go-logging: the PCS_ENCRYPTION_KEY value is never logged (no secrets in logs).
// go-context: ctx is the first parameter, not stored.
func LoadDecryptedCredentials(ctx context.Context, sqlDB *sql.DB) (*Credentials, error) {
	creds, err := db.LoadCredentials(ctx, sqlDB)
	if err != nil {
		return nil, fmt.Errorf("load credentials from db: %w", err)
	}

	envKey := os.Getenv(EncryptionKeyEnv)
	if envKey == "" {
		return nil, fmt.Errorf(
			"%s environment variable not set; "+
				"set it in a systemd EnvironmentFile or equivalent", EncryptionKeyEnv)
	}

	key := crypto.DeriveKeyWithSalt(envKey, creds.Salt)

	uid, err := decryptField(key, creds.ProtonUIDEnc, "proton uid")
	if err != nil {
		return nil, err
	}
	refresh, err := decryptField(key, creds.ProtonRefreshEnc, "proton refresh token")
	if err != nil {
		return nil, err
	}
	keyPass, err := crypto.Decrypt(key, creds.ProtonKeyPassEnc)
	if err != nil {
		return nil, fmt.Errorf("decrypt proton key password (wrong %s?): %w", EncryptionKeyEnv, err)
	}
	cdPass, err := decryptField(key, creds.CardDAVPasswordEnc, "carddav password")
	if err != nil {
		return nil, err
	}

	return &Credentials{
		Session: protonmail.Session{
			UID:          string(uid),
			RefreshToken: string(refresh),
			KeyPass:      keyPass,
		},
		CardDAVPass: string(cdPass),
		key:         key,
	}, nil
}

// EncryptRefreshToken encrypts a rotated Proton refresh token with the same key
// used to decrypt the credentials, for persistence via db.UpdateProtonRefresh.
func (c *Credentials) EncryptRefreshToken(token string) ([]byte, error) {
	return crypto.Encrypt(c.key, []byte(token))
}

func decryptField(key, ciphertext []byte, label string) ([]byte, error) {
	plain, err := crypto.Decrypt(key, ciphertext)
	if err != nil {
		return nil, fmt.Errorf("decrypt %s (wrong %s?): %w", label, EncryptionKeyEnv, err)
	}
	return plain, nil
}

// hashString returns the hex-encoded SHA-256 digest of s.
// Used to detect whether a vCard has changed since last sync.
func hashString(s string) string {
	sum := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", sum)
}
