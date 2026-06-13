package db

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

// Open opens (or creates) the SQLite database at path, enables WAL mode,
// and runs schema migrations.
func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("open sqlite3: %w", err)
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return db, nil
}

func migrate(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS credentials (
			id                  INTEGER PRIMARY KEY CHECK (id = 1),
			salt                BLOB    NOT NULL,
			proton_password_enc BLOB    NOT NULL,
			carddav_password_enc BLOB   NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS contacts (
			uid          TEXT PRIMARY KEY,
			proton_id    TEXT,
			carddav_href TEXT,
			proton_etag  TEXT,
			carddav_etag TEXT,
			vcard_data   TEXT    NOT NULL,
			updated_at   INTEGER NOT NULL
		)`,
	}

	for _, stmt := range statements {
		if _, err := db.ExecContext(context.Background(), stmt); err != nil {
			return fmt.Errorf("exec migration %q: %w", stmt[:40], err)
		}
	}
	return nil
}

// Credentials holds encrypted credential blobs.
type Credentials struct {
	Salt               []byte
	ProtonPasswordEnc  []byte
	CardDAVPasswordEnc []byte
}

// StoreCredentials upserts credentials into the DB (only one row ever).
func StoreCredentials(ctx context.Context, db *sql.DB, creds *Credentials) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO credentials (id, salt, proton_password_enc, carddav_password_enc)
		VALUES (1, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			salt = excluded.salt,
			proton_password_enc  = excluded.proton_password_enc,
			carddav_password_enc = excluded.carddav_password_enc`,
		creds.Salt, creds.ProtonPasswordEnc, creds.CardDAVPasswordEnc)
	return err
}

// LoadCredentials retrieves the stored credential blobs.
func LoadCredentials(ctx context.Context, db *sql.DB) (*Credentials, error) {
	row := db.QueryRowContext(ctx, `SELECT salt, proton_password_enc, carddav_password_enc FROM credentials WHERE id=1`)

	var c Credentials
	if err := row.Scan(&c.Salt, &c.ProtonPasswordEnc, &c.CardDAVPasswordEnc); err != nil {
		return nil, fmt.Errorf("load credentials: %w", err)
	}
	return &c, nil
}
