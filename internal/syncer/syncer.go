// Package syncer orchestrates bidirectional Proton ↔ CardDAV contact sync.
package syncer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/emersion/go-vcard"

	"github.com/secbyd/proton-carddav-sync/internal/carddav"
	dbpkg "github.com/secbyd/proton-carddav-sync/internal/db"
	"github.com/secbyd/proton-carddav-sync/internal/protonmail"
)

// Direction controls which way contacts are synchronised.
type Direction int

const (
	// DirectionBoth syncs in both directions.
	DirectionBoth Direction = iota + 1 // go-defensive: iota+1 so zero == uninitialized
	// DirectionToCardDAV only pushes Proton contacts to CardDAV.
	DirectionToCardDAV
	// DirectionToProton only pushes CardDAV contacts to Proton.
	DirectionToProton
)

// Syncer orchestrates contact synchronisation.
// It does not store a context — contexts are passed to each method
// (go-context: never store context in a struct).
type Syncer struct {
	proton  protonmail.ContactsClient
	carddav carddav.ContactsClient
	db      *sql.DB
	log     *slog.Logger
	dir     Direction
}

// New constructs a Syncer.
// Accepts interfaces (go-interfaces: accept interfaces, return concrete types).
func New(
	protonClient protonmail.ContactsClient,
	carddavClient carddav.ContactsClient,
	db *sql.DB,
	log *slog.Logger,
	dir Direction,
) *Syncer {
	return &Syncer{
		proton:  protonClient,
		carddav: carddavClient,
		db:      db,
		log:     log,
		dir:     dir,
	}
}

// Sync performs one full synchronisation cycle.
func (s *Syncer) Sync(ctx context.Context) error {
	if s.dir == DirectionBoth || s.dir == DirectionToCardDAV {
		if err := s.syncProtonToCardDAV(ctx); err != nil {
			return fmt.Errorf("proton→carddav: %w", err)
		}
	}
	if s.dir == DirectionBoth || s.dir == DirectionToProton {
		if err := s.syncCardDAVToProton(ctx); err != nil {
			return fmt.Errorf("carddav→proton: %w", err)
		}
	}
	return nil
}

func (s *Syncer) syncProtonToCardDAV(ctx context.Context) error {
	contacts, err := s.proton.ListContacts(ctx)
	if err != nil {
		return fmt.Errorf("list proton contacts: %w", err)
	}

	for _, c := range contacts {
		// go-context: check cancellation in loops.
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		vcardStr, err := s.proton.GetContactVCard(ctx, c.ID)
		if err != nil {
			// go-logging: warn and skip per-contact errors (log OR return, not both).
			s.log.Warn("skip proton contact: get vcard failed",
				"contact_id", c.ID, "err", err)
			continue
		}

		uid := extractUID(vcardStr, c.ID)

		if err := s.carddav.PutContact(ctx, uid, vcardStr); err != nil {
			return fmt.Errorf("put carddav contact %q: %w", uid, err)
		}

		s.log.Info("synced proton→carddav", "uid", uid)

		if err := dbpkg.UpsertContact(ctx, s.db, dbpkg.ContactRecord{
			UID:       uid,
			VCardHash: hashString(vcardStr),
		}); err != nil {
			return fmt.Errorf("upsert local contact %q: %w", uid, err)
		}
	}
	return nil
}

func (s *Syncer) syncCardDAVToProton(ctx context.Context) error {
	objects, err := s.carddav.ListContacts(ctx)
	if err != nil {
		return fmt.Errorf("list carddav contacts: %w", err)
	}

	for _, obj := range objects {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var sb strings.Builder
		if obj.Data != nil {
			if _, err := sb.ReadFrom(obj.Data); err != nil {
				s.log.Warn("skip carddav contact: read data failed",
					"path", obj.Path, "err", err)
				continue
			}
		}
		vcardStr := sb.String()
		uid := extractUID(vcardStr, obj.Path)

		rec, err := dbpkg.GetContact(ctx, s.db, uid)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			if _, err := s.proton.CreateContact(ctx, vcardStr); err != nil {
				return fmt.Errorf("create proton contact %q: %w", uid, err)
			}
		case err != nil:
			return fmt.Errorf("get local contact %q: %w", uid, err)
		default:
			newHash := hashString(vcardStr)
			if rec.VCardHash == newHash {
				continue
			}
			if err := s.proton.UpdateContact(ctx, rec.UID, vcardStr); err != nil {
				return fmt.Errorf("update proton contact %q: %w", uid, err)
			}
		}

		s.log.Info("synced carddav→proton", "uid", uid)

		if err := dbpkg.UpsertContact(ctx, s.db, dbpkg.ContactRecord{
			UID:       uid,
			ETag:      obj.ETag,
			VCardHash: hashString(vcardStr),
		}); err != nil {
			return fmt.Errorf("upsert local contact %q: %w", uid, err)
		}
	}
	return nil
}

func extractUID(raw, fallback string) string {
	dec := vcard.NewDecoder(strings.NewReader(raw))
	card, err := dec.Decode()
	if err != nil {
		return fallback
	}
	if f := card.Get(vcard.FieldUID); f != nil && f.Value != "" {
		return f.Value
	}
	return fallback
}
