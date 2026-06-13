package cli

import (
	"fmt"

	"github.com/secbyd/proton-carddav-sync/internal/config"
	"github.com/secbyd/proton-carddav-sync/internal/crypto"
	"github.com/secbyd/proton-carddav-sync/internal/db"
	"github.com/secbyd/proton-carddav-sync/internal/log"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialise the database and store credentials",
	RunE:  runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
}

func runInit(_ *cobra.Command, _ []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	logger, err := log.New(cfg.Log.Level, cfg.Log.Format)
	if err != nil {
		return err
	}

	database, err := db.Open(cfg.Sync.DBPath)
	if err != nil {
		return fmt.Errorf("opening db: %w", err)
	}
	defer database.Close()

	key, err := crypto.DeriveKey(cfg.Proton.Password)
	if err != nil {
		return fmt.Errorf("deriving encryption key: %w", err)
	}

	encProton, err := crypto.Encrypt(key, []byte(cfg.Proton.Password))
	if err != nil {
		return fmt.Errorf("encrypting proton password: %w", err)
	}
	encCardDAV, err := crypto.Encrypt(key, []byte(cfg.CardDAV.Password))
	if err != nil {
		return fmt.Errorf("encrypting carddav password: %w", err)
	}

	if err := database.StoreCredentials(encProton, encCardDAV); err != nil {
		return fmt.Errorf("storing credentials: %w", err)
	}

	logger.Info("Initialisation complete")
	return nil
}
