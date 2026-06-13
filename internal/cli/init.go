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
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Store encrypted credentials in the local database",
	Long: `Prompts for Proton Mail and CardDAV passwords, derives an encryption
key via PBKDF2, and stores the encrypted credentials in the SQLite database.

This command must be run once before starting the daemon.`,
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

	// Derive key from Proton password; encrypt the CardDAV password.
	key, salt, err := crypto.DeriveKey(string(protonPass))
	if err != nil {
		return fmt.Errorf("derive key: %w", err)
	}

	encCardDAV, err := crypto.Encrypt(key, carddavPass)
	if err != nil {
		return fmt.Errorf("encrypt carddav password: %w", err)
	}

	creds := db.Credentials{
		Salt:               salt,
		CardDAVPasswordEnc: encCardDAV,
	}
	if err := db.SaveCredentials(ctx, sqlDB, creds); err != nil {
		return fmt.Errorf("save credentials: %w", err)
	}

	fmt.Println("Credentials stored successfully.")
	return nil
}
