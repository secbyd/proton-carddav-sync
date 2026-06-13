package syncer

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	"github.com/secbyd/proton-carddav-sync/internal/crypto"
	"github.com/secbyd/proton-carddav-sync/internal/db"
)

// loadDecryptedCredentials retrieves the encrypted credentials from the DB,
// decrypts them using the stored salt, and returns plaintext passwords.
//
// The decryption key is re-derived from the Proton password itself.
// At init time the user provided the password interactively; for daemon
// operation the password must be supplied via an environment variable or a
// secrets manager (e.g. a systemd EnvironmentFile).
func loadDecryptedCredentials(ctx context.Context, sqlDB *sql.DB) (protonPass, cardDAVPass string, err error) {
	creds, err := db.LoadCredentials(ctx, sqlDB)
	if err != nil {
		return "", "", fmt.Errorf("load credentials from db: %w", err)
	}

	// Bootstrap from the PROTON_PASSWORD environment variable
	// (systemd EnvironmentFile pattern).
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
