package syncer

import (
	"fmt"

	"github.com/secbyd/proton-carddav-sync/internal/crypto"
	"github.com/secbyd/proton-carddav-sync/internal/db"
)

// LoadDecryptedCredentials loads and decrypts stored credentials from the DB.
// The masterPassword is derived from the config before the syncer starts.
func LoadDecryptedCredentials(database *db.DB, masterPassword string) (protonPw, cardDAVPw string, err error) {
	key, err := crypto.DeriveKey(masterPassword)
	if err != nil {
		return "", "", fmt.Errorf("deriving key: %w", err)
	}

	encProton, encCardDAV, err := database.LoadCredentials()
	if err != nil {
		return "", "", fmt.Errorf("loading credentials from db: %w", err)
	}

	plainProton, err := crypto.Decrypt(key, encProton)
	if err != nil {
		return "", "", fmt.Errorf("decrypting proton password: %w", err)
	}
	plainCardDAV, err := crypto.Decrypt(key, encCardDAV)
	if err != nil {
		return "", "", fmt.Errorf("decrypting carddav password: %w", err)
	}

	return string(plainProton), string(plainCardDAV), nil
}
