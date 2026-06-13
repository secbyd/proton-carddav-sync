package cli

import (
	"context"
	"fmt"
	"os"
	"syscall"

	"github.com/secbyd/proton-carddav-sync/internal/config"
	"github.com/secbyd/proton-carddav-sync/internal/crypto"
	"github.com/secbyd/proton-carddav-sync/internal/db"
	"github.com/secbyd/proton-carddav-sync/internal/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	golang_crypto "golang.org/x/crypto/ssh/terminal"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Store encrypted credentials in the local database",
	Long: `Reads Proton Mail and CardDAV passwords from stdin (never stored
in the config file) and persists them AES-256-GCM encrypted in the
SQLite database. Run this once before starting the daemon.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		logger, err := log.New(cfg.Log.Level, cfg.Log.Format)
		if err != nil {
			return fmt.Errorf("init logger: %w", err)
		}
		defer logger.Sync() //nolint:errcheck

		// --- Proton password --------------------------------------------------
		fmt.Print("Enter Proton Mail password: ")
		protonPassBytes, err := golang_crypto.ReadPassword(int(syscall.Stdin))
		if err != nil {
			return fmt.Errorf("read proton password: %w", err)
		}
		fmt.Println()
		protonPass := string(protonPassBytes)

		// --- CardDAV password -------------------------------------------------
		// CardDAV password may come from config.yaml or from stdin.
		cardDAVPass := viper.GetString("carddav.password")
		if cardDAVPass == "" {
			fmt.Print("Enter CardDAV password: ")
			cdPassBytes, err := golang_crypto.ReadPassword(int(syscall.Stdin))
			if err != nil {
				return fmt.Errorf("read carddav password: %w", err)
			}
			fmt.Println()
			cardDAVPass = string(cdPassBytes)
		}

		// --- Derive encryption key from Proton password -----------------------
		key, salt, err := crypto.DeriveKey(protonPass)
		if err != nil {
			return fmt.Errorf("derive key: %w", err)
		}

		encProton, err := crypto.Encrypt(key, []byte(protonPass))
		if err != nil {
			return fmt.Errorf("encrypt proton password: %w", err)
		}

		encCardDAV, err := crypto.Encrypt(key, []byte(cardDAVPass))
		if err != nil {
			return fmt.Errorf("encrypt carddav password: %w", err)
		}

		// --- Open DB and store ------------------------------------------------
		sqlDB, err := db.Open(cfg.DB.Path)
		if err != nil {
			return fmt.Errorf("open db: %w", err)
		}
		defer sqlDB.Close()

		if err := db.StoreCredentials(context.Background(), sqlDB, &db.Credentials{
			ProtonPasswordEnc: encProton,
			CardDAVPasswordEnc: encCardDAV,
			Salt: salt,
		}); err != nil {
			return fmt.Errorf("store credentials: %w", err)
		}

		logger.Info("Credentials stored successfully. You can now start the daemon.")
		_ = os.Stdout
		return nil
	},
}
