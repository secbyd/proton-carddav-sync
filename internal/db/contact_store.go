package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// ContactRecord is the persisted state of a synced contact.
type ContactRecord struct {
	UID         string
	ProtonID    string
	CardDAVHref string
	ProtonETag  string
	CardDAVETag string
	VCardData   string
	UpdatedAt   int64
}

// UpsertContact creates or updates a contact record.
func UpsertContact(ctx context.Context, db *sql.DB, r *ContactRecord) error {
	r.UpdatedAt = time.Now().Unix()
	_, err := db.ExecContext(ctx, `
		INSERT INTO contacts (uid, proton_id, carddav_href, proton_etag, carddav_etag, vcard_data, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(uid) DO UPDATE SET
			proton_id    = excluded.proton_id,
			carddav_href = excluded.carddav_href,
			proton_etag  = excluded.proton_etag,
			carddav_etag = excluded.carddav_etag,
			vcard_data   = excluded.vcard_data,
			updated_at   = excluded.updated_at`,
		r.UID, r.ProtonID, r.CardDAVHref, r.ProtonETag, r.CardDAVETag, r.VCardData, r.UpdatedAt)
	return err
}

// GetContact retrieves a contact record by UID.
func GetContact(ctx context.Context, db *sql.DB, uid string) (*ContactRecord, error) {
	row := db.QueryRowContext(ctx, `SELECT uid, proton_id, carddav_href, proton_etag, carddav_etag, vcard_data, updated_at FROM contacts WHERE uid=?`, uid)
	var r ContactRecord
	if err := row.Scan(&r.UID, &r.ProtonID, &r.CardDAVHref, &r.ProtonETag, &r.CardDAVETag, &r.VCardData, &r.UpdatedAt); err != nil {
		return nil, fmt.Errorf("get contact %q: %w", uid, err)
	}
	return &r, nil
}

// ListContacts returns all stored contact records.
func ListContacts(ctx context.Context, db *sql.DB) ([]*ContactRecord, error) {
	rows, err := db.QueryContext(ctx, `SELECT uid, proton_id, carddav_href, proton_etag, carddav_etag, vcard_data, updated_at FROM contacts`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []*ContactRecord
	for rows.Next() {
		var r ContactRecord
		if err := rows.Scan(&r.UID, &r.ProtonID, &r.CardDAVHref, &r.ProtonETag, &r.CardDAVETag, &r.VCardData, &r.UpdatedAt); err != nil {
			return nil, err
		}
		records = append(records, &r)
	}
	return records, rows.Err()
}

// DeleteContact removes a contact record by UID.
func DeleteContact(ctx context.Context, db *sql.DB, uid string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM contacts WHERE uid=?`, uid)
	return err
}

// GetContactByProtonID looks up a record by its Proton contact ID.
func GetContactByProtonID(ctx context.Context, db *sql.DB, protonID string) (*ContactRecord, error) {
	row := db.QueryRowContext(ctx, `SELECT uid, proton_id, carddav_href, proton_etag, carddav_etag, vcard_data, updated_at FROM contacts WHERE proton_id=?`, protonID)
	var r ContactRecord
	if err := row.Scan(&r.UID, &r.ProtonID, &r.CardDAVHref, &r.ProtonETag, &r.CardDAVETag, &r.VCardData, &r.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get contact by proton id %q: %w", protonID, err)
	}
	return &r, nil
}

// GetContactByCardDAVHref looks up a record by its CardDAV href.
func GetContactByCardDAVHref(ctx context.Context, db *sql.DB, href string) (*ContactRecord, error) {
	row := db.QueryRowContext(ctx, `SELECT uid, proton_id, carddav_href, proton_etag, carddav_etag, vcard_data, updated_at FROM contacts WHERE carddav_href=?`, href)
	var r ContactRecord
	if err := row.Scan(&r.UID, &r.ProtonID, &r.CardDAVHref, &r.ProtonETag, &r.CardDAVETag, &r.VCardData, &r.UpdatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get contact by carddav href %q: %w", href, err)
	}
	return &r, nil
}
