package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/secbyd/proton-carddav-sync/internal/config"
	"github.com/secbyd/proton-carddav-sync/internal/crypto"
	"github.com/secbyd/proton-carddav-sync/internal/db"
	"github.com/secbyd/proton-carddav-sync/internal/syncer"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Store encrypted credentials in the local database",
	Long: `Prompts for Proton Mail and CardDAV passwords, derives an encryption
key from PCS_ENCRYPTION_KEY via PBKDF2, and stores the encrypted credentials in
the SQLite database.

PCS_ENCRYPTION_KEY must be set in the environment: it is the master key that
protects every stored credential, and the daemon needs the same value to
decrypt them. This command must be run once before starting the daemon.`,
	RunE: runInit,
}

func runInit(cmd *cobra.Command, _ []string) error {
	// go-context: derive from command context so Ctrl-C cancels the prompt.
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// The master key comes from the environment so init and the daemon agree
	// on the same value. go-logging: the key value is never logged.
	encKey := os.Getenv(syncer.EncryptionKeyEnv)
	if encKey == "" {
		return fmt.Errorf("%s environment variable not set; "+
			"export it (and reuse the same value for the daemon) before running init",
			syncer.EncryptionKeyEnv)
	}

	sqlDB, err := db.Open(cfg.Database.Path)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer sqlDB.Close() // go-defensive: defer cleanup immediately

	fmt.Printf("Proton Mail password for %s: ", cfg.Proton.Username)
	protonPass, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return fmt.Errorf("read proton password: %w", err)
	}

	fmt.Printf("CardDAV password for %s: ", cfg.CardDAV.Username)
	carddavPass, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return fmt.Errorf("read carddav password: %w", err)
	}

	// Derive the master key from PCS_ENCRYPTION_KEY; encrypt both passwords.
	key, salt, err := crypto.DeriveKey(encKey)
	if err != nil {
		return fmt.Errorf("derive key: %w", err)
	}

	encProton, err := crypto.Encrypt(key, protonPass)
	if err != nil {
		return fmt.Errorf("encrypt proton password: %w", err)
	}

	encCardDAV, err := crypto.Encrypt(key, carddavPass)
	if err != nil {
		return fmt.Errorf("encrypt carddav password: %w", err)
	}

	creds := db.Credentials{
		Salt:               salt,
		ProtonPasswordEnc:  encProton,
		CardDAVPasswordEnc: encCardDAV,
	}
	if err := db.SaveCredentials(ctx, sqlDB, creds); err != nil {
		return fmt.Errorf("save credentials: %w", err)
	}

	fmt.Println("Credentials stored successfully.")
	return nil
}
