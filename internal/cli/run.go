package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/secbyd/proton-carddav-sync/internal/config"
	internallog "github.com/secbyd/proton-carddav-sync/internal/log"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Start the background daemon",
	Long: `Starts a long-running process that syncs contacts on a configurable
interval. Gracefully shuts down on SIGINT or SIGTERM.

Set PROTON_PASSWORD in the environment (or a systemd EnvironmentFile) before
starting.`,
	RunE: runDaemon,
}

func runDaemon(cmd *cobra.Command, _ []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	log := internallog.New(cfg.Log.Format, cfg.Log.Level)

	// go-concurrency: clear stop mechanism — context cancelled on signal.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// go-defensive: never start a goroutine without knowing how it will stop.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case sig := <-sigCh:
			log.Info("received signal, shutting down", "signal", sig)
			cancel()
		case <-ctx.Done():
		}
	}()

	interval := time.Duration(cfg.Sync.IntervalSeconds) * time.Second
	log.Info("daemon started",
		"interval_seconds", cfg.Sync.IntervalSeconds,
		"direction", cfg.Sync.Direction)

	// Sync immediately on start, then on each tick.
	if err := runSyncWithConfig(ctx, cfg, log); err != nil && ctx.Err() == nil {
		log.Error("sync failed", "err", err)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := runSyncWithConfig(ctx, cfg, log); err != nil {
				if ctx.Err() != nil {
					// Context cancelled — shutdown in progress.
					break
				}
				// go-logging: log OR return — log error and continue daemon.
				log.Error("sync failed", "err", err)
			}
		case <-ctx.Done():
			log.Info("daemon stopped")
			return nil
		}
	}
}
