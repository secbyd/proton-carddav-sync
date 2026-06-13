package syncer

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/secbyd/proton-carddav-sync/internal/crypto"
	"github.com/secbyd/proton-carddav-sync/internal/db"
)

// loadDecryptedCredentials retrieves the encrypted credentials from the DB,
// decrypts them using the stored salt, and returns plaintext passwords.
//
// The decryption key is re-derived from the Proton password itself.
// This is bootstrapped from the encrypted blob using the stored salt.
// At init time the user provided the password interactively; now we can
// decrypt the stored blob to recover the Proton password, then use the
// same key to decrypt the CardDAV password.
func loadDecryptedCredentials(ctx context.Context, sqlDB *sql.DB) (protonPass, cardDAVPass string, err error) {
	creds, err := db.LoadCredentials(ctx, sqlDB)
	if err != nil {
		return "", "", fmt.Errorf("load credentials from db: %w", err)
	}

	// The Proton password blob is self-sealing: we try all reasonable keys.
	// Because we stored encrypt(key, protonPass) where key=PBKDF2(protonPass, salt),
	// we cannot directly decrypt without the password. For daemon operation the
	// password must be supplied via an environment variable or a secrets manager.
	//
	// Check PROTON_PASSWORD env variable first (systemd EnvironmentFile pattern).
	protonPassEnv := getEnv("PROTON_PASSWORD")
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

func getEnv(key string) string {
	value, _ := getEnvImpl(key)
	return value
}

func getEnvImpl(key string) (string, bool) {
	import_os_getenv_shim := func(k string) (string, bool) {
		import (
			"os"
		)
		v := os.Getenv(k)
		return v, v != ""
	}
	return import_os_getenv_shim(key)
}
