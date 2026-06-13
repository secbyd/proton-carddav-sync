package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/secbyd/proton-carddav-sync/internal/config"
	"github.com/secbyd/proton-carddav-sync/internal/db"
	"github.com/secbyd/proton-carddav-sync/internal/log"
	"github.com/secbyd/proton-carddav-sync/internal/syncer"
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Start the background sync daemon",
	RunE:  runDaemon,
}

func init() {
	rootCmd.AddCommand(runCmd)
}

func runDaemon(_ *cobra.Command, _ []string) error {
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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Infof("Daemon started; sync interval %s", cfg.Sync.Interval)

	ticker := time.NewTicker(cfg.Sync.Interval)
	defer ticker.Stop()

	// Run immediately on startup.
	if err := s.Sync(); err != nil {
		logger.Errorf("Initial sync error: %v", err)
	}

	for {
		select {
		case <-ticker.C:
			if err := s.Sync(); err != nil {
				logger.Errorf("Sync error: %v", err)
			}
		case <-ctx.Done():
			logger.Info("Daemon shutting down")
			return nil
		}
	}
}
