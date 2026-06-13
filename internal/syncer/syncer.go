package syncer

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/secbyd/proton-carddav-sync/internal/carddav"
	"github.com/secbyd/proton-carddav-sync/internal/config"
	"github.com/secbyd/proton-carddav-sync/internal/db"
	"github.com/secbyd/proton-carddav-sync/internal/protonmail"
	"github.com/secbyd/proton-carddav-sync/internal/vcardsync"
	"go.uber.org/zap"
)

// Syncer orchestrates bidirectional contact synchronisation.
type Syncer struct {
	cfg    *config.Config
	db     *sql.DB
	log    *zap.SugaredLogger
	proton *protonmail.Client
	cdav   *carddav.Client
}

// New constructs a Syncer, loading and decrypting credentials from the DB.
func New(ctx context.Context, cfg *config.Config, sqlDB *sql.DB, logger *zap.SugaredLogger) (*Syncer, error) {
	protonPass, cardDAVPass, err := loadDecryptedCredentials(ctx, sqlDB)
	if err != nil {
		return nil, fmt.Errorf("load credentials: %w", err)
	}

	pmClient, err := protonmail.NewClient(ctx, cfg.Proton.Username, protonPass)
	if err != nil {
		return nil, fmt.Errorf("proton client: %w", err)
	}

	cdavClient, err := carddav.New(cfg.CardDAV.URL, cfg.CardDAV.Username, cardDAVPass)
	if err != nil {
		pmClient.Close()
		return nil, fmt.Errorf("carddav client: %w", err)
	}

	return &Syncer{
		cfg:    cfg,
		db:     sqlDB,
		log:    logger,
		proton: pmClient,
		cdav:   cdavClient,
	}, nil
}

// Close logs out and frees resources.
func (s *Syncer) Close() {
	s.proton.Close()
}

// Sync performs one full bidirectional sync pass.
func (s *Syncer) Sync(ctx context.Context) error {
	strategy := vcardsync.Strategy(s.cfg.Sync.MergeStrategy)
	direction := s.cfg.Sync.Direction

	// --- Fetch from both sides -----------------------------------------------
	s.log.Info("Fetching Proton contacts")
	protonContacts, err := s.proton.GetAllContacts(ctx)
	if err != nil {
		return fmt.Errorf("fetch proton contacts: %w", err)
	}
	s.log.Infof("Proton: %d contacts", len(protonContacts))

	s.log.Info("Fetching CardDAV contacts")
	cdavContacts, err := s.cdav.ListAll(ctx)
	if err != nil {
		return fmt.Errorf("fetch carddav contacts: %w", err)
	}
	s.log.Infof("CardDAV: %d contacts", len(cdavContacts))

	// --- Build index: uid -> record ------------------------------------------
	dbRecords, err := db.ListContacts(ctx, s.db)
	if err != nil {
		return fmt.Errorf("list db contacts: %w", err)
	}
	byUID := make(map[string]*db.ContactRecord, len(dbRecords))
	for _, r := range dbRecords {
		byUID[r.UID] = r
	}

	// --- Proton → CardDAV ---------------------------------------------------
	if direction == "both" || direction == "proton-to-carddav" {
		for protonID, vcardData := range protonContacts {
			if err := s.syncProtonToCardDAV(ctx, protonID, vcardData, byUID, strategy); err != nil {
				s.log.Warnf("Proton→CardDAV error for %q: %v", protonID, err)
			}
		}
	}

	// --- CardDAV → Proton ---------------------------------------------------
	if direction == "both" || direction == "carddav-to-proton" {
		for _, entry := range cdavContacts {
			if err := s.syncCardDAVToProton(ctx, entry, byUID, strategy); err != nil {
				s.log.Warnf("CardDAV→Proton error for %q: %v", entry.Href, err)
			}
		}
	}

	return nil
}

// syncProtonToCardDAV propagates a single Proton contact to CardDAV.
func (s *Syncer) syncProtonToCardDAV(
	ctx context.Context,
	protonID, vcardData string,
	byUID map[string]*db.ContactRecord,
	strategy vcardsync.Strategy,
) error {
	uid, err := vcardsync.GetUID(vcardData)
	if err != nil || uid == "" {
		s.log.Debugf("Skipping Proton contact %q: no UID", protonID)
		return nil
	}

	existing, ok := byUID[uid]
	if !ok {
		// New on Proton side – push to CardDAV.
		s.log.Infof("New Proton contact %q → CardDAV", uid)
		href, err := s.cdav.Put(ctx, "", vcardData)
		if err != nil {
			return err
		}
		return db.UpsertContact(ctx, s.db, &db.ContactRecord{
			UID: uid, ProtonID: protonID, CardDAVHref: href, VCardData: vcardData,
		})
	}

	// Already known – check if Proton side changed.
	if existing.VCardData == vcardData {
		return nil // no change
	}

	// Merge and push to CardDAV.
	merged, err := vcardsync.Merge(existing.VCardData, vcardData, existing.VCardData, strategy)
	if err != nil {
		return fmt.Errorf("merge: %w", err)
	}
	s.log.Infof("Updated Proton contact %q → CardDAV", uid)
	href, err := s.cdav.Put(ctx, existing.CardDAVHref, merged)
	if err != nil {
		return err
	}
	existing.VCardData = merged
	existing.CardDAVHref = href
	return db.UpsertContact(ctx, s.db, existing)
}

// syncCardDAVToProton propagates a single CardDAV contact to Proton.
func (s *Syncer) syncCardDAVToProton(
	ctx context.Context,
	entry *carddav.ContactEntry,
	byUID map[string]*db.ContactRecord,
	strategy vcardsync.Strategy,
) error {
	uid, err := vcardsync.GetUID(entry.VCard)
	if err != nil || uid == "" {
		s.log.Debugf("Skipping CardDAV contact %q: no UID", entry.Href)
		return nil
	}

	existing, ok := byUID[uid]
	if !ok {
		// New on CardDAV side – push to Proton.
		s.log.Infof("New CardDAV contact %q → Proton", uid)
		protonID, err := s.proton.CreateContact(ctx, entry.VCard)
		if err != nil {
			return err
		}
		return db.UpsertContact(ctx, s.db, &db.ContactRecord{
			UID: uid, ProtonID: protonID, CardDAVHref: entry.Href,
			CardDAVETag: entry.ETag, VCardData: entry.VCard,
		})
	}

	// Already known – check if CardDAV side changed.
	if existing.CardDAVETag == entry.ETag {
		return nil // no change
	}

	// Merge and push to Proton.
	merged, err := vcardsync.Merge(existing.VCardData, existing.VCardData, entry.VCard, strategy)
	if err != nil {
		return fmt.Errorf("merge: %w", err)
	}
	s.log.Infof("Updated CardDAV contact %q → Proton", uid)
	if err := s.proton.UpdateContact(ctx, existing.ProtonID, merged); err != nil {
		return err
	}
	existing.VCardData = merged
	existing.CardDAVETag = entry.ETag
	return db.UpsertContact(ctx, s.db, existing)
}
