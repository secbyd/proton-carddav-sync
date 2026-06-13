package cli

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/secbyd/proton-carddav-sync/internal/carddav"
	"github.com/secbyd/proton-carddav-sync/internal/config"
	"github.com/secbyd/proton-carddav-sync/internal/db"
	internallog "github.com/secbyd/proton-carddav-sync/internal/log"
	"github.com/secbyd/proton-carddav-sync/internal/protonmail"
	"github.com/secbyd/proton-carddav-sync/internal/syncer"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Run a single synchronisation pass and exit",
	RunE:  runSync,
}

func runSync(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	log := internallog.New(cfg.Log.Format, cfg.Log.Level)

	return runSyncWithConfig(ctx, cfg, log)
}

// runSyncWithConfig is the shared implementation used by both sync and run.
func runSyncWithConfig(ctx context.Context, cfg *config.Config, log *slog.Logger) error {
	sqlDB, err := db.Open(cfg.Database.Path)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer sqlDB.Close()

	protonPass, carddavPass, err := syncer.LoadDecryptedCredentials(ctx, sqlDB)
	if err != nil {
		return fmt.Errorf("load credentials: %w", err)
	}

	protonClient := protonmail.NewClient()

	// loginErr uses a distinct name to avoid shadowing the outer err.
	if loginErr := protonClient.Login(ctx, cfg.Proton.Username, protonPass); loginErr != nil {
		return fmt.Errorf("proton login: %w", loginErr)
	}
	defer func() {
		// Pass ctx so the contextcheck linter is satisfied; Logout uses
		// a background-style context internally when the parent is done.
		if logoutErr := protonClient.Logout(ctx); logoutErr != nil {
			log.Warn("proton logout failed", "err", logoutErr)
		}
	}()

	carddavClient, cdErr := carddav.New(ctx, cfg.CardDAV.URL, cfg.CardDAV.Username, carddavPass)
	if cdErr != nil {
		return fmt.Errorf("create carddav client: %w", cdErr)
	}

	dir := parseSyncDirection(cfg.Sync.Direction)
	s := syncer.New(protonClient, carddavClient, sqlDB, log, dir)

	log.Info("starting sync", "direction", cfg.Sync.Direction)
	if syncErr := s.Sync(ctx); syncErr != nil {
		return fmt.Errorf("sync: %w", syncErr)
	}
	log.Info("sync complete")
	return nil
}

func parseSyncDirection(s string) syncer.Direction {
	switch s {
	case "proton-to-carddav":
		return syncer.DirectionToCardDAV
	case "carddav-to-proton":
		return syncer.DirectionToProton
	default:
		return syncer.DirectionBoth
	}
}
