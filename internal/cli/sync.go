package cli

import (
	"context"
	"fmt"

	"github.com/secbyd/proton-carddav-sync/internal/config"
	"github.com/secbyd/proton-carddav-sync/internal/db"
	"github.com/secbyd/proton-carddav-sync/internal/log"
	"github.com/secbyd/proton-carddav-sync/internal/syncer"
	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Run a single synchronisation pass and exit",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSync()
	},
}

func runSync() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger, err := log.New(cfg.Log.Level, cfg.Log.Format)
	if err != nil {
		return fmt.Errorf("init logger: %w", err)
	}
	defer logger.Sync() //nolint:errcheck

	sqlDB, err := db.Open(cfg.DB.Path)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer sqlDB.Close()

	s, err := syncer.New(context.Background(), cfg, sqlDB, logger)
	if err != nil {
		return fmt.Errorf("create syncer: %w", err)
	}
	defer s.Close()

	if err := s.Sync(context.Background()); err != nil {
		return fmt.Errorf("sync: %w", err)
	}

	logger.Info("Sync completed successfully.")
	return nil
}
