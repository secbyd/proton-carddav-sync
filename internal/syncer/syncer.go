// Package syncer orchestrates bidirectional Proton ↔ CardDAV contact sync.
package syncer

import (
	"bytes"
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
	"github.com/secbyd/proton-carddav-sync/internal/vcardsync"
)

// Direction controls which way contacts are synchronised.
type Direction int

const (
	DirectionBoth      Direction = iota + 1
	DirectionToCardDAV Direction = iota + 1
	DirectionToProton  Direction = iota + 1
)

// Syncer orchestrates contact synchronisation.
type Syncer struct {
	proton       protonmail.ContactsClient
	carddav      carddav.ContactsClient
	db           *sql.DB
	log          *slog.Logger
	dir          Direction
	policy       vcardsync.Policy
	maxNewProton int // cap on new Proton contacts created per run (0 = unlimited)
}

// New constructs a Syncer. policy selects how genuine field conflicts are
// resolved during a bidirectional (DirectionBoth) sync; maxNewProton caps how
// many brand-new contacts are pushed to Proton in a single run (0 = unlimited),
// to spread a large first sync over several runs.
func New(
	protonClient protonmail.ContactsClient,
	carddavClient carddav.ContactsClient,
	db *sql.DB,
	log *slog.Logger,
	dir Direction,
	policy vcardsync.Policy,
	maxNewProton int,
) *Syncer {
	return &Syncer{
		proton:       protonClient,
		carddav:      carddavClient,
		db:           db,
		log:          log,
		dir:          dir,
		policy:       policy,
		maxNewProton: maxNewProton,
	}
}

// Sync performs one full synchronisation cycle. The bidirectional case runs a
// per-contact three-way merge; the one-way cases push a single direction.
func (s *Syncer) Sync(ctx context.Context) error {
	switch s.dir {
	case DirectionToCardDAV:
		if err := s.syncProtonToCardDAV(ctx); err != nil {
			return fmt.Errorf("proton→carddav: %w", err)
		}
		return nil
	case DirectionToProton:
		if err := s.syncCardDAVToProton(ctx); err != nil {
			return fmt.Errorf("carddav→proton: %w", err)
		}
		return nil
	default: // DirectionBoth
		if err := s.reconcile(ctx); err != nil {
			return fmt.Errorf("reconcile: %w", err)
		}
		return nil
	}
}

func (s *Syncer) syncProtonToCardDAV(ctx context.Context) error {
	contacts, err := s.proton.ListContacts(ctx)
	if err != nil {
		return fmt.Errorf("list proton contacts: %w", err)
	}

	// Index existing CardDAV vCards by UID so we can overlay Proton's fields
	// onto them, preserving CardDAV-only properties Proton does not model.
	existing, err := s.cardDAVByUID(ctx)
	if err != nil {
		return err
	}

	for _, c := range contacts {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		protonVCard, vcErr := s.proton.GetContactVCard(ctx, c.ID)
		if vcErr != nil {
			s.log.Warn("skip proton contact: get vcard failed",
				"contact_id", c.ID, "err", vcErr)
			continue
		}

		uid := extractUID(protonVCard, c.ID)

		// For an existing CardDAV contact, overlay Proton's fields onto the
		// CardDAV card so CardDAV-only fields survive. For a brand-new contact,
		// write Proton's card as-is.
		finalVCard := protonVCard
		if existingVCard, ok := existing[uid]; ok {
			merged, mergeErr := vcardsync.OverlayString(existingVCard, protonVCard)
			if mergeErr != nil {
				s.log.Warn("skip proton contact: merge failed", "uid", uid, "err", mergeErr)
				continue
			}
			// Nothing changed on the CardDAV side — skip the write (and the REV
			// bump) entirely.
			if hashString(merged) == hashString(existingVCard) {
				continue
			}
			finalVCard = merged
		}

		if putErr := s.carddav.PutContact(ctx, uid, finalVCard); putErr != nil {
			return fmt.Errorf("put carddav contact %q: %w", uid, putErr)
		}

		s.log.Info("synced proton→carddav", "uid", uid)

		if upsertErr := dbpkg.UpsertContact(ctx, s.db, dbpkg.ContactRecord{
			UID:       uid,
			VCardHash: hashString(finalVCard),
		}); upsertErr != nil {
			return fmt.Errorf("upsert local contact %q: %w", uid, upsertErr)
		}
	}
	return nil
}

// cardDAVByUID lists CardDAV contacts and returns their encoded vCards keyed by
// UID, for overlay merging in the Proton→CardDAV direction.
func (s *Syncer) cardDAVByUID(ctx context.Context) (map[string]string, error) {
	objects, err := s.carddav.ListContacts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list carddav contacts: %w", err)
	}

	out := make(map[string]string, len(objects))
	for _, obj := range objects {
		var buf bytes.Buffer
		if encErr := vcard.NewEncoder(&buf).Encode(obj.Card); encErr != nil {
			s.log.Warn("skip carddav contact in index: encode failed",
				"path", obj.Path, "err", encErr)
			continue
		}
		str := buf.String()
		out[extractUID(str, obj.Path)] = str
	}
	return out, nil
}

func (s *Syncer) syncCardDAVToProton(ctx context.Context) error {
	objects, err := s.carddav.ListContacts(ctx)
	if err != nil {
		return fmt.Errorf("list carddav contacts: %w", err)
	}

	newProton, deferred := 0, 0
	for _, obj := range objects {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var buf bytes.Buffer
		if encErr := vcard.NewEncoder(&buf).Encode(obj.Card); encErr != nil {
			s.log.Warn("skip carddav contact: encode vcard failed",
				"path", obj.Path, "err", encErr)
			continue
		}
		vcardStr := buf.String()
		uid := extractUID(vcardStr, obj.Path)

		rec, getErr := dbpkg.GetContact(ctx, s.db, uid)
		switch {
		case errors.Is(getErr, sql.ErrNoRows):
			if s.maxNewProton > 0 && newProton >= s.maxNewProton {
				deferred++
				continue
			}
			if _, createErr := s.proton.CreateContact(ctx, vcardStr); createErr != nil {
				return fmt.Errorf("create proton contact %q: %w", uid, createErr)
			}
			newProton++
		case getErr != nil:
			return fmt.Errorf("get local contact %q: %w", uid, getErr)
		default:
			newHash := hashString(vcardStr)
			if rec.VCardHash == newHash {
				continue
			}
			if updateErr := s.proton.UpdateContact(ctx, rec.UID, vcardStr); updateErr != nil {
				return fmt.Errorf("update proton contact %q: %w", uid, updateErr)
			}
		}

		s.log.Info("synced carddav→proton", "uid", uid)

		if upsertErr := dbpkg.UpsertContact(ctx, s.db, dbpkg.ContactRecord{
			UID:       uid,
			ETag:      obj.ETag,
			VCardHash: hashString(vcardStr),
		}); upsertErr != nil {
			return fmt.Errorf("upsert local contact %q: %w", uid, upsertErr)
		}
	}
	if deferred > 0 {
		s.log.Info("deferred new Proton contacts to a later run (per-run cap reached)",
			"deferred", deferred, "created", newProton, "cap", s.maxNewProton)
	}
	return nil
}

// contactSide holds a contact's current vCard plus the handles needed to write
// back to that side (id for Proton, etag for CardDAV).
type contactSide struct {
	id    string
	etag  string
	vcard string
}

// reconcile performs a bidirectional, per-contact three-way merge over the union
// of contacts on both sides. Field additions, edits, and deletions propagate in
// both directions; genuine conflicts are resolved by s.policy. Whole-contact
// deletion is intentionally NOT propagated (a contact present in the last-synced
// state but now gone from one side is left untouched, not deleted or resurrected).
func (s *Syncer) reconcile(ctx context.Context) error {
	protonIdx, err := s.protonByUID(ctx)
	if err != nil {
		return err
	}
	carddavIdx, err := s.carddavIndex(ctx)
	if err != nil {
		return err
	}

	s.log.Info("reconcile: indexed contacts",
		"proton", len(protonIdx), "carddav", len(carddavIdx))

	uids := make(map[string]struct{}, len(protonIdx)+len(carddavIdx))
	for uid := range protonIdx {
		uids[uid] = struct{}{}
	}
	for uid := range carddavIdx {
		uids[uid] = struct{}{}
	}

	newProton, deferred := 0, 0
	for uid := range uids {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		pe, hasP := protonIdx[uid]
		ce, hasC := carddavIdx[uid]

		rec, getErr := dbpkg.GetContact(ctx, s.db, uid)
		hasBase := getErr == nil
		if getErr != nil && !errors.Is(getErr, sql.ErrNoRows) {
			return fmt.Errorf("get local contact %q: %w", uid, getErr)
		}

		switch {
		case hasP && hasC:
			if err := s.reconcileBoth(ctx, uid, pe, ce, rec); err != nil {
				return err
			}
		case hasP && !hasC:
			if hasBase {
				s.log.Warn("contact gone from carddav; not deleting/resurrecting "+
					"(whole-contact deletion is not synced)", "uid", uid)
				continue
			}
			if putErr := s.carddav.PutContact(ctx, uid, pe.vcard); putErr != nil {
				return fmt.Errorf("put carddav contact %q: %w", uid, putErr)
			}
			s.log.Info("created proton→carddav", "uid", uid)
			if err := s.saveBases(ctx, uid, "", pe.vcard, pe.vcard); err != nil {
				return err
			}
		case !hasP && hasC:
			if hasBase {
				s.log.Warn("contact gone from proton; not deleting/resurrecting "+
					"(whole-contact deletion is not synced)", "uid", uid)
				continue
			}
			// Bound the number of new Proton contacts per run so a large first
			// sync does not burst against Proton's anti-abuse limits. Deferred
			// contacts have no base record and are retried next run.
			if s.maxNewProton > 0 && newProton >= s.maxNewProton {
				deferred++
				continue
			}
			newID, createErr := s.proton.CreateContact(ctx, ce.vcard)
			if createErr != nil {
				return fmt.Errorf("create proton contact %q: %w", uid, createErr)
			}
			newProton++
			protonBase := ce.vcard
			if refetched, refErr := s.proton.GetContactVCard(ctx, newID); refErr != nil {
				s.log.Warn("proton refetch after create failed; base may be stale", "uid", uid, "err", refErr)
			} else {
				protonBase = refetched
			}
			s.log.Info("created carddav→proton", "uid", uid)
			if err := s.saveBases(ctx, uid, ce.etag, protonBase, ce.vcard); err != nil {
				return err
			}
		}
	}
	if deferred > 0 {
		s.log.Info("deferred new Proton contacts to a later run (per-run cap reached)",
			"deferred", deferred, "created", newProton, "cap", s.maxNewProton)
	}
	return nil
}

// reconcileBoth merges a contact that exists on both sides and writes back any
// changes, refreshing the stored per-side bases.
func (s *Syncer) reconcileBoth(ctx context.Context, uid string, p, c contactSide, rec dbpkg.ContactRecord) error {
	merged, conflicts, err := vcardsync.ThreeWayString(rec.ProtonBase, rec.CardDAVBase, p.vcard, c.vcard, s.policy)
	if err != nil {
		s.log.Warn("skip contact: merge failed", "uid", uid, "err", err)
		return nil
	}
	if len(conflicts) > 0 {
		s.log.Info("merge conflicts resolved by policy", "uid", uid, "fields", conflicts)
	}

	// Write CardDAV when the merge changed anything (CardDAV is lossless).
	cdEqual, err := vcardsync.EqualString(merged, c.vcard)
	if err != nil {
		return fmt.Errorf("compare carddav contact %q: %w", uid, err)
	}
	if !cdEqual {
		if putErr := s.carddav.PutContact(ctx, uid, merged); putErr != nil {
			return fmt.Errorf("put carddav contact %q: %w", uid, putErr)
		}
		s.log.Info("merged proton↔carddav → carddav", "uid", uid)
	}

	// Write Proton only when a Proton-modelled property changed (avoids churning
	// on CardDAV-only fields Proton would drop again). Re-fetch afterwards so the
	// stored Proton base reflects Proton's own normalised representation.
	protonBase := p.vcard
	protonChanged, err := vcardsync.ProtonRelevantDiff(merged, p.vcard, rec.ProtonBase)
	if err != nil {
		return fmt.Errorf("compare proton contact %q: %w", uid, err)
	}
	if protonChanged {
		if updErr := s.proton.UpdateContact(ctx, p.id, merged); updErr != nil {
			return fmt.Errorf("update proton contact %q: %w", uid, updErr)
		}
		s.log.Info("merged proton↔carddav → proton", "uid", uid)
		if refetched, refErr := s.proton.GetContactVCard(ctx, p.id); refErr != nil {
			s.log.Warn("proton refetch after update failed; base may be stale", "uid", uid, "err", refErr)
		} else {
			protonBase = refetched
		}
	}

	return s.saveBases(ctx, uid, c.etag, protonBase, merged)
}

// saveBases persists the per-side bases (and a hash for quick equality) for uid.
func (s *Syncer) saveBases(ctx context.Context, uid, etag, protonBase, carddavBase string) error {
	if err := dbpkg.UpsertContact(ctx, s.db, dbpkg.ContactRecord{
		UID:         uid,
		ETag:        etag,
		VCardHash:   hashString(carddavBase),
		ProtonBase:  protonBase,
		CardDAVBase: carddavBase,
	}); err != nil {
		return fmt.Errorf("upsert local contact %q: %w", uid, err)
	}
	return nil
}

// protonByUID fetches and decodes every Proton contact, keyed by vCard UID.
func (s *Syncer) protonByUID(ctx context.Context) (map[string]contactSide, error) {
	contacts, err := s.proton.ListContacts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list proton contacts: %w", err)
	}

	out := make(map[string]contactSide, len(contacts))
	for _, ct := range contacts {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		v, vcErr := s.proton.GetContactVCard(ctx, ct.ID)
		if vcErr != nil {
			s.log.Warn("skip proton contact: get vcard failed", "contact_id", ct.ID, "err", vcErr)
			continue
		}
		out[extractUID(v, ct.ID)] = contactSide{id: ct.ID, vcard: v}
	}
	return out, nil
}

// carddavIndex returns the current CardDAV contacts keyed by vCard UID.
func (s *Syncer) carddavIndex(ctx context.Context) (map[string]contactSide, error) {
	objects, err := s.carddav.ListContacts(ctx)
	if err != nil {
		return nil, fmt.Errorf("list carddav contacts: %w", err)
	}

	out := make(map[string]contactSide, len(objects))
	for _, obj := range objects {
		var buf bytes.Buffer
		if encErr := vcard.NewEncoder(&buf).Encode(obj.Card); encErr != nil {
			s.log.Warn("skip carddav contact in index: encode failed", "path", obj.Path, "err", encErr)
			continue
		}
		str := buf.String()
		out[extractUID(str, obj.Path)] = contactSide{etag: obj.ETag, vcard: str}
	}
	return out, nil
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
