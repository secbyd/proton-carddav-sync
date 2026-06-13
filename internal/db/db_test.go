package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// TestMigrateAddsContactBaseColumns simulates a database created under the older
// schema (no proton_base/carddav_base) and verifies Open migrates it and that
// the new ContactRecord fields round-trip.
func TestMigrateAddsContactBaseColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")

	// Create an old-schema contacts table directly.
	raw, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	_, err = raw.Exec(`CREATE TABLE contacts (
        uid TEXT PRIMARY KEY,
        etag TEXT NOT NULL DEFAULT '',
        vcard_hash TEXT NOT NULL DEFAULT '',
        updated_at INTEGER NOT NULL DEFAULT 0
    );`)
	if err != nil {
		t.Fatalf("create old schema: %v", err)
	}
	if _, seedErr := raw.Exec(`INSERT INTO contacts (uid) VALUES ('legacy')`); seedErr != nil {
		t.Fatalf("seed legacy row: %v", seedErr)
	}
	raw.Close()

	// Open through the real code path — this must run the migration.
	conn, err := Open(path)
	if err != nil {
		t.Fatalf("Open (migrate): %v", err)
	}
	defer conn.Close()

	ctx := context.Background()

	// Legacy row should still be readable, with empty bases.
	legacy, err := GetContact(ctx, conn, "legacy")
	if err != nil {
		t.Fatalf("get legacy: %v", err)
	}
	if legacy.ProtonBase != "" || legacy.CardDAVBase != "" {
		t.Errorf("legacy bases = %q/%q, want empty", legacy.ProtonBase, legacy.CardDAVBase)
	}

	// New fields round-trip.
	want := ContactRecord{
		UID:         "c1",
		ETag:        "etag-1",
		VCardHash:   "hash-1",
		ProtonBase:  "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:c1\r\nEND:VCARD\r\n",
		CardDAVBase: "BEGIN:VCARD\r\nVERSION:4.0\r\nUID:c1\r\nNOTE:keep\r\nEND:VCARD\r\n",
	}
	if upErr := UpsertContact(ctx, conn, want); upErr != nil {
		t.Fatalf("upsert: %v", upErr)
	}
	got, err := GetContact(ctx, conn, "c1")
	if err != nil {
		t.Fatalf("get c1: %v", err)
	}
	if got.ProtonBase != want.ProtonBase || got.CardDAVBase != want.CardDAVBase {
		t.Errorf("bases round-trip mismatch:\n got proton=%q carddav=%q\nwant proton=%q carddav=%q",
			got.ProtonBase, got.CardDAVBase, want.ProtonBase, want.CardDAVBase)
	}
}
