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
	Short: "Start the continuous sync daemon",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDaemon()
	},
}

func runDaemon() error {
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s, err := syncer.New(ctx, cfg, sqlDB, logger)
	if err != nil {
		return fmt.Errorf("create syncer: %w", err)
	}
	defer s.Close()

	interval, err := time.ParseDuration(cfg.Sync.Interval)
	if err != nil {
		return fmt.Errorf("parse sync interval %q: %w", cfg.Sync.Interval, err)
	}

	logger.Infof("Daemon started; sync every %s", interval)

	// Run immediately on start.
	if err := s.Sync(ctx); err != nil {
		logger.Warnf("Initial sync error: %v", err)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case <-ticker.C:
			logger.Info("Starting scheduled sync")
			if err := s.Sync(ctx); err != nil {
				logger.Warnf("Sync error: %v", err)
			} else {
				logger.Info("Sync completed")
			}
		case sig := <-sigCh:
			logger.Infof("Received signal %s, shutting down", sig)
			cancel()
			return nil
		case <-ctx.Done():
			return nil
		}
	}
}
