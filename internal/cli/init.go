package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/secbyd/proton-carddav-sync/internal/config"
	"github.com/secbyd/proton-carddav-sync/internal/crypto"
	"github.com/secbyd/proton-carddav-sync/internal/db"
	"github.com/secbyd/proton-carddav-sync/internal/protonmail"
	"github.com/secbyd/proton-carddav-sync/internal/syncer"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create the config file (if needed) and store an encrypted session",
	Long: `Sets up proton-carddav-sync:

  1. If no config file exists, prompts for every config.yaml setting and writes
     it (no passwords are ever stored in the config file).
  2. Logs in to Proton (prompting for a TOTP code if 2FA is enabled) and derives
     a long-lasting session — UID, a rotating refresh token, and the mailbox key
     password — so the daemon never needs the account password again.
  3. Encrypts the Proton session and the CardDAV password with a key derived
     from PCS_ENCRYPTION_KEY and stores them in the SQLite database.

PCS_ENCRYPTION_KEY must be set in the environment: it is the master key that
protects every stored credential, and the daemon needs the same value to
decrypt them. Run this once before starting the daemon.`,
	RunE: runInit,
}

func runInit(cmd *cobra.Command, _ []string) error {
	// go-context: derive from command context so Ctrl-C cancels the prompts.
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	// The master key comes from the environment so init and the daemon agree
	// on the same value. go-logging: the key value is never logged.
	encKey := os.Getenv(syncer.EncryptionKeyEnv)
	if encKey == "" {
		return fmt.Errorf("%s environment variable not set; "+
			"export it (and reuse the same value for the daemon) before running init",
			syncer.EncryptionKeyEnv)
	}

	cfg, err := loadOrCreateConfig(cfgFile)
	if err != nil {
		return err
	}

	sqlDB, err := db.Open(cfg.Database.Path)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer sqlDB.Close() // go-defensive: defer cleanup immediately

	protonPass, err := promptSecret(fmt.Sprintf("Proton Mail password for %s", cfg.Proton.Username))
	if err != nil {
		return err
	}
	carddavPass, err := promptSecret(fmt.Sprintf("CardDAV password for %s", cfg.CardDAV.Username))
	if err != nil {
		return err
	}

	// Log in to obtain a durable, password-free session (with TOTP if required).
	protonClient := protonmail.NewClient(cfg.Proton.AppVersion)
	totpPrompt := func() (string, error) {
		return promptLine(bufio.NewReader(os.Stdin), "Proton TOTP code", "")
	}
	session, err := protonClient.LoginWithPassword(ctx, cfg.Proton.Username, protonPass, totpPrompt)
	if err != nil {
		return fmt.Errorf("proton login: %w", err)
	}
	// go-defensive: Close drops local state WITHOUT revoking the session, so the
	// refresh token we are about to store stays valid for the daemon.
	defer protonClient.Close()

	// Derive the master key from PCS_ENCRYPTION_KEY and encrypt everything.
	key, salt, err := crypto.DeriveKey(encKey)
	if err != nil {
		return fmt.Errorf("derive key: %w", err)
	}

	uidEnc, err := crypto.Encrypt(key, []byte(session.UID))
	if err != nil {
		return fmt.Errorf("encrypt proton uid: %w", err)
	}
	refreshEnc, err := crypto.Encrypt(key, []byte(session.RefreshToken))
	if err != nil {
		return fmt.Errorf("encrypt proton refresh token: %w", err)
	}
	keyPassEnc, err := crypto.Encrypt(key, session.KeyPass)
	if err != nil {
		return fmt.Errorf("encrypt proton key password: %w", err)
	}
	cardDAVEnc, err := crypto.Encrypt(key, []byte(carddavPass))
	if err != nil {
		return fmt.Errorf("encrypt carddav password: %w", err)
	}

	creds := db.Credentials{
		Salt:               salt,
		ProtonUIDEnc:       uidEnc,
		ProtonRefreshEnc:   refreshEnc,
		ProtonKeyPassEnc:   keyPassEnc,
		CardDAVPasswordEnc: cardDAVEnc,
	}
	if err := db.SaveCredentials(ctx, sqlDB, creds); err != nil {
		return fmt.Errorf("save credentials: %w", err)
	}

	fmt.Println("Credentials stored successfully.")
	return nil
}

// loadOrCreateConfig loads the config, or — when none exists — prompts for the
// settings interactively, writes the file, and returns the validated result.
func loadOrCreateConfig(cfgFile string) (*config.Config, error) {
	cfg, err := config.Load(cfgFile)
	if err == nil {
		return cfg, nil
	}
	if !errors.Is(err, config.ErrConfigNotFound) {
		return nil, fmt.Errorf("load config: %w", err)
	}

	gathered, err := gatherConfigInteractively()
	if err != nil {
		return nil, err
	}

	path := config.ResolvePath(cfgFile)
	if saveErr := config.Save(gathered, path); saveErr != nil {
		return nil, saveErr
	}
	fmt.Printf("Wrote config to %s\n", path)

	// Reload so defaults are applied, validation runs, and the DB dir is made.
	cfg, err = config.Load(cfgFile)
	if err != nil {
		return nil, fmt.Errorf("reload config: %w", err)
	}
	return cfg, nil
}
