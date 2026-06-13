package cli

import (
	"fmt"

	"github.com/secbyd/proton-carddav-sync/internal/config"
	"github.com/secbyd/proton-carddav-sync/internal/db"
	"github.com/secbyd/proton-carddav-sync/internal/log"
	"github.com/secbyd/proton-carddav-sync/internal/syncer"
	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Run a one-shot sync between ProtonMail and CardDAV",
	RunE:  runSync,
}

func init() {
	rootCmd.AddCommand(syncCmd)
}

func runSync(_ *cobra.Command, _ []string) error {
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

	s, err := syncer.New(cfg, database, logger)
	if err != nil {
		return fmt.Errorf("creating syncer: %w", err)
	}

	if err := s.Sync(); err != nil {
		return fmt.Errorf("sync failed: %w", err)
	}
	logger.Info("Sync complete")
	return nil
}
