package db

import "time"

// ContactRecord represents a synced contact entry in the DB.
type ContactRecord struct {
	UID          string
	ProtonID     string
	CardDAVHref  string
	ProtonEtag   string
	CardDAVEtag  string
	VCard        string
	LastSyncedAt time.Time
}

// UpsertContact inserts or updates a contact record.
func (d *DB) UpsertContact(r *ContactRecord) error {
	_, err := d.Exec(`
		INSERT INTO contacts (uid, proton_id, carddav_href, proton_etag, carddav_etag, vcard, last_synced_at)
		VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(uid) DO UPDATE SET
			proton_id     = excluded.proton_id,
			carddav_href  = excluded.carddav_href,
			proton_etag   = excluded.proton_etag,
			carddav_etag  = excluded.carddav_etag,
			vcard         = excluded.vcard,
			last_synced_at = CURRENT_TIMESTAMP`,
		r.UID, r.ProtonID, r.CardDAVHref, r.ProtonEtag, r.CardDAVEtag, r.VCard)
	return err
}

// GetContactByUID returns a contact record by UID.
func (d *DB) GetContactByUID(uid string) (*ContactRecord, error) {
	row := d.QueryRow(`
		SELECT uid, proton_id, carddav_href, proton_etag, carddav_etag, vcard, last_synced_at
		FROM contacts WHERE uid = ?`, uid)
	r := &ContactRecord{}
	return r, row.Scan(&r.UID, &r.ProtonID, &r.CardDAVHref, &r.ProtonEtag, &r.CardDAVEtag, &r.VCard, &r.LastSyncedAt)
}

// AllContacts returns all contact records.
func (d *DB) AllContacts() ([]*ContactRecord, error) {
	rows, err := d.Query(`
		SELECT uid, proton_id, carddav_href, proton_etag, carddav_etag, vcard, last_synced_at
		FROM contacts ORDER BY uid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []*ContactRecord
	for rows.Next() {
		r := &ContactRecord{}
		if err := rows.Scan(&r.UID, &r.ProtonID, &r.CardDAVHref, &r.ProtonEtag, &r.CardDAVEtag, &r.VCard, &r.LastSyncedAt); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

// DeleteContact removes a contact by UID.
func (d *DB) DeleteContact(uid string) error {
	_, err := d.Exec(`DELETE FROM contacts WHERE uid = ?`, uid)
	return err
}
