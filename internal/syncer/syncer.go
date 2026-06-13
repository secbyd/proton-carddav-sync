// Package syncer orchestrates a full ProtonMail ↔ CardDAV sync cycle.
package syncer

import (
	"context"
	"fmt"

	"github.com/secbyd/proton-carddav-sync/internal/carddav"
	"github.com/secbyd/proton-carddav-sync/internal/config"
	"github.com/secbyd/proton-carddav-sync/internal/db"
	"github.com/secbyd/proton-carddav-sync/internal/log"
	"github.com/secbyd/proton-carddav-sync/internal/protonmail"
)

// Syncer holds clients and state needed to run a sync cycle.
type Syncer struct {
	cfg     *config.Config
	db      *db.DB
	logger  log.Logger
	proton  *protonmail.Client
	carddav *carddav.Client
}

// New constructs a Syncer, initialising both API clients.
func New(cfg *config.Config, database *db.DB, logger log.Logger) (*Syncer, error) {
	ctx := context.Background()

	pc, err := protonmail.New(ctx, cfg.Proton.Username, cfg.Proton.Password)
	if err != nil {
		return nil, fmt.Errorf("protonmail client: %w", err)
	}

	cc, err := carddav.New(cfg.CardDAV.URL, cfg.CardDAV.Username, cfg.CardDAV.Password)
	if err != nil {
		return nil, fmt.Errorf("carddav client: %w", err)
	}

	return &Syncer{
		cfg:     cfg,
		db:      database,
		logger:  logger,
		proton:  pc,
		carddav: cc,
	}, nil
}

// Sync runs one complete sync cycle.
func (s *Syncer) Sync() error {
	ctx := context.Background()

	dir := s.cfg.Sync.Direction

	// --- ProtonMail → CardDAV ---
	if dir == "both" || dir == "proton-to-carddav" {
		if err := s.syncProtonToCardDAV(ctx); err != nil {
			return fmt.Errorf("proton→carddav: %w", err)
		}
	}

	// --- CardDAV → ProtonMail ---
	if dir == "both" || dir == "carddav-to-proton" {
		if err := s.syncCardDAVToProton(ctx); err != nil {
			return fmt.Errorf("carddav→proton: %w", err)
		}
	}
	return nil
}

func (s *Syncer) syncProtonToCardDAV(ctx context.Context) error {
	contacts, err := s.proton.ListContacts(ctx)
	if err != nil {
		return err
	}
	s.logger.Infof("ProtonMail contacts fetched: %d", len(contacts))

	for _, c := range contacts {
		vcard, err := s.proton.GetContactVCard(ctx, c.ID)
		if err != nil {
			s.logger.Warnf("Skipping contact %s: %v", c.ID, err)
			continue
		}

		rec, _ := s.db.GetContactByUID(c.ID)
		if rec != nil && rec.VCard == vcard {
			continue // unchanged
		}

		href := s.cfg.CardDAV.URL + c.ID + ".vcf"
		etag := ""
		if rec != nil {
			etag = rec.CardDAVEtag
		}
		newEtag, err := s.carddavPutRaw(ctx, href, vcard, etag)
		if err != nil {
			s.logger.Errorf("CardDAV put %s: %v", href, err)
			continue
		}

		_ = s.db.UpsertContact(&db.ContactRecord{
			UID:         c.ID,
			ProtonID:    c.ID,
			CardDAVHref: href,
			CardDAVEtag: newEtag,
			VCard:       vcard,
		})
	}
	return nil
}

func (s *Syncer) syncCardDAVToProton(ctx context.Context) error {
	contacts, err := s.carddav.ListContacts(ctx)
	if err != nil {
		return err
	}
	s.logger.Infof("CardDAV contacts fetched: %d", len(contacts))
	// TODO: implement full CardDAV→Proton reconciliation.
	_ = contacts
	return nil
}

// carddavPutRaw is a helper that accepts a raw vCard string.
func (s *Syncer) carddavPutRaw(_ context.Context, href, _ /*vcard*/ string, _ /*etag*/ string) (string, error) {
	s.logger.Debugf("carddav put (stub) %s", href)
	return "stub-etag", nil
}
