// Package db — contact state store.
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ContactRecord represents a locally cached contact state.
type ContactRecord struct {
	UID        string
	ETag       string
	VCardHash  string
	UpdatedAt  time.Time
}

// UpsertContact inserts or updates a contact record.
func UpsertContact(ctx context.Context, db *sql.DB, r ContactRecord) error {
	const q = `
INSERT INTO contacts (uid, etag, vcard_hash, updated_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(uid) DO UPDATE SET
    etag       = excluded.etag,
    vcard_hash = excluded.vcard_hash,
    updated_at = excluded.updated_at`

	if _, err := db.ExecContext(ctx, q, r.UID, r.ETag, r.VCardHash, r.UpdatedAt.Unix()); err != nil {
		return fmt.Errorf("upsert contact %q: %w", r.UID, err)
	}
	return nil
}

// GetContact fetches a single contact record by UID.
// Returns sql.ErrNoRows (via errors.Is) when the contact is not found.
func GetContact(ctx context.Context, db *sql.DB, uid string) (ContactRecord, error) {
	const q = `SELECT uid, etag, vcard_hash, updated_at FROM contacts WHERE uid = ?`

	var r ContactRecord
	var updatedAtUnix int64

	err := db.QueryRowContext(ctx, q, uid).Scan(&r.UID, &r.ETag, &r.VCardHash, &updatedAtUnix)
	if errors.Is(err, sql.ErrNoRows) {
		return ContactRecord{}, sql.ErrNoRows
	}
	if err != nil {
		return ContactRecord{}, fmt.Errorf("get contact %q: %w", uid, err)
	}

	r.UpdatedAt = time.Unix(updatedAtUnix, 0).UTC()
	return r, nil
}

// ListContacts returns all stored contact records.
// The returned slice is always non-nil.
func ListContacts(ctx context.Context, db *sql.DB) ([]ContactRecord, error) {
	const q = `SELECT uid, etag, vcard_hash, updated_at FROM contacts ORDER BY uid`

	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list contacts: %w", err)
	}
	defer rows.Close() // go-defensive: defer cleanup immediately after resource open

	var records []ContactRecord
	for rows.Next() {
		var r ContactRecord
		var updatedAtUnix int64
		if err := rows.Scan(&r.UID, &r.ETag, &r.VCardHash, &updatedAtUnix); err != nil {
			return nil, fmt.Errorf("scan contact row: %w", err)
		}
		r.UpdatedAt = time.Unix(updatedAtUnix, 0).UTC()
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate contact rows: %w", err)
	}

	if records == nil {
		records = []ContactRecord{} // go-defensive: always return non-nil slice
	}
	return records, nil
}

// DeleteContact removes a contact record by UID.
func DeleteContact(ctx context.Context, db *sql.DB, uid string) error {
	const q = `DELETE FROM contacts WHERE uid = ?`
	if _, err := db.ExecContext(ctx, q, uid); err != nil {
		return fmt.Errorf("delete contact %q: %w", uid, err)
	}
	return nil
}
