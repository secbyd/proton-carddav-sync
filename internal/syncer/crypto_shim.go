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
)

// EncryptionKeyEnv is the environment variable that holds the master key used
// to encrypt every credential stored in the local database (the Proton and
// CardDAV passwords). It is independent of the Proton account password.
const EncryptionKeyEnv = "PCS_ENCRYPTION_KEY"

// LoadDecryptedCredentials retrieves the encrypted credentials from the DB and
// decrypts them with a key derived from PCS_ENCRYPTION_KEY and the stored salt.
//
// go-logging: the PCS_ENCRYPTION_KEY value is never logged (no secrets in logs).
// go-context: ctx is the first parameter, not stored.
func LoadDecryptedCredentials(ctx context.Context, sqlDB *sql.DB) (protonPass, cardDAVPass string, err error) {
	creds, err := db.LoadCredentials(ctx, sqlDB)
	if err != nil {
		return "", "", fmt.Errorf("load credentials from db: %w", err)
	}

	encKey := os.Getenv(EncryptionKeyEnv)
	if encKey == "" {
		return "", "", fmt.Errorf(
			"%s environment variable not set; "+
				"set it in a systemd EnvironmentFile or equivalent", EncryptionKeyEnv)
	}

	key := crypto.DeriveKeyWithSalt(encKey, creds.Salt)

	protonPassBytes, err := crypto.Decrypt(key, creds.ProtonPasswordEnc)
	if err != nil {
		return "", "", fmt.Errorf("decrypt proton password (wrong %s?): %w", EncryptionKeyEnv, err)
	}

	cdPassBytes, err := crypto.Decrypt(key, creds.CardDAVPasswordEnc)
	if err != nil {
		return "", "", fmt.Errorf("decrypt carddav password (wrong %s?): %w", EncryptionKeyEnv, err)
	}

	return string(protonPassBytes), string(cdPassBytes), nil
}

// hashString returns the hex-encoded SHA-256 digest of s.
// Used to detect whether a vCard has changed since last sync.
func hashString(s string) string {
	sum := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", sum)
}
