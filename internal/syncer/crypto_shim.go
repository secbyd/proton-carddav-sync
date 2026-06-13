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

// LoadDecryptedCredentials retrieves encrypted credentials from the DB and
// decrypts them using the Proton password from the PROTON_PASSWORD environment
// variable.
//
// go-logging: PROTON_PASSWORD value is never logged (no secrets in logs).
// go-context: ctx is the first parameter, not stored.
func LoadDecryptedCredentials(ctx context.Context, sqlDB *sql.DB) (protonPass, cardDAVPass string, err error) {
	creds, err := db.LoadCredentials(ctx, sqlDB)
	if err != nil {
		return "", "", fmt.Errorf("load credentials from db: %w", err)
	}

	protonPassEnv := os.Getenv("PROTON_PASSWORD")
	if protonPassEnv == "" {
		return "", "", fmt.Errorf(
			"PROTON_PASSWORD environment variable not set; " +
				"set it in a systemd EnvironmentFile or equivalent")
	}

	key := crypto.DeriveKeyWithSalt(protonPassEnv, creds.Salt)

	cdPassBytes, err := crypto.Decrypt(key, creds.CardDAVPasswordEnc)
	if err != nil {
		return "", "", fmt.Errorf("decrypt carddav password: %w", err)
	}

	return protonPassEnv, string(cdPassBytes), nil
}

// hashString returns the hex-encoded SHA-256 digest of s.
// Used to detect whether a vCard has changed since last sync.
func hashString(s string) string {
	sum := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", sum)
}
