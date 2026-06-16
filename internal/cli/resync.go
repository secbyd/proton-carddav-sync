package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/secbyd/proton-carddav-sync/internal/config"
	internallog "github.com/secbyd/proton-carddav-sync/internal/log"
	"github.com/secbyd/proton-carddav-sync/internal/syncer"
)

var (
	resyncUIDs []string
	resyncAll  bool
)

var resyncCmd = &cobra.Command{
	Use:   "resync",
	Short: "Force-(re)create or repair specific contacts by UID",
	Long: `Force-reconcile specific contacts, bypassing the normal guards that skip
contacts missing from one side or unchanged.

For each --uid:
  - if it exists on only one side, it is created on the other (resurrected);
  - if it exists on both, the CardDAV version is re-pushed to Proton, rebuilding
    the encrypted/signed cards (use this to repair a malformed contact).

Examples:
  proton-carddav-sync resync --uid e299611f-6c6f-44ea-a8c3-812752b01011
  proton-carddav-sync resync --uid <a> --uid <b>
  proton-carddav-sync resync --all`,
	RunE: runResync,
}

func init() {
	resyncCmd.Flags().StringArrayVar(&resyncUIDs, "uid", nil,
		"contact UID to force-resync (repeatable)")
	resyncCmd.Flags().BoolVar(&resyncAll, "all", false,
		"force-resync every contact found on either side")
	rootCmd.AddCommand(resyncCmd)
}

func runResync(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	if len(resyncUIDs) == 0 && !resyncAll {
		return errors.New("specify at least one --uid <uid> (repeatable) or --all")
	}

	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	log := internallog.New(cfg.Log.Format, cfg.Log.Level)

	return withSyncer(ctx, cfg, log, func(s *syncer.Syncer) error {
		if forceErr := s.ForceContacts(ctx, resyncUIDs, resyncAll); forceErr != nil {
			return fmt.Errorf("resync: %w", forceErr)
		}
		log.Info("resync complete")
		return nil
	})
}
