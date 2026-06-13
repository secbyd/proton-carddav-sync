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
	"github.com/secbyd/proton-carddav-sync/internal/vcardsync"
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

	creds, err := syncer.LoadDecryptedCredentials(ctx, sqlDB)
	if err != nil {
		return fmt.Errorf("load credentials: %w", err)
	}

	protonClient := protonmail.NewClient(cfg.Proton.AppVersion)

	// Persist rotated refresh tokens so the next run can resume. Rotation only
	// happens during live API calls (which share ctx), so propagating ctx here
	// is correct.
	onRefresh := func(token string) {
		enc, encErr := creds.EncryptRefreshToken(token)
		if encErr != nil {
			log.Warn("encrypt rotated proton refresh token failed", "err", encErr)
			return
		}
		if upErr := db.UpdateProtonRefresh(ctx, sqlDB, enc); upErr != nil {
			log.Warn("persist rotated proton refresh token failed", "err", upErr)
		}
	}

	if resumeErr := protonClient.ResumeSession(ctx, creds.Session, onRefresh); resumeErr != nil {
		return fmt.Errorf("resume proton session: %w", resumeErr)
	}
	// go-defensive: Close drops local state without revoking the session, so
	// the stored refresh token stays valid for the next run.
	defer protonClient.Close()

	carddavClient, cdErr := carddav.New(ctx, cfg.CardDAV.URL, cfg.CardDAV.Username, creds.CardDAVPass)
	if cdErr != nil {
		return fmt.Errorf("create carddav client: %w", cdErr)
	}

	dir := parseSyncDirection(cfg.Sync.Direction)
	s := syncer.New(protonClient, carddavClient, sqlDB, log, dir, parseConflictPolicy(cfg.Sync.Conflict))

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

func parseConflictPolicy(s string) vcardsync.Policy {
	switch s {
	case "prefer-proton":
		return vcardsync.PolicyPreferProton
	case "prefer-carddav":
		return vcardsync.PolicyPreferCardDAV
	default:
		return vcardsync.PolicyPreferNewer
	}
}
