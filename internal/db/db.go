// Package db manages the SQLite database for credential and contact state
// storage.
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// Sentinel errors.
var (
	// ErrCredentialsNotFound is returned when no credentials row exists in the
	// database. Run `proton-carddav-sync init` to create it.
	ErrCredentialsNotFound = errors.New("credentials not found: run 'proton-carddav-sync init'")
)

// Credentials holds the encrypted credential material persisted to SQLite.
//
// Instead of the Proton account password, a long-lasting session is stored
// (UID + rotating refresh token + the derived mailbox/key password), so the
// daemon resumes the session via refresh token without ever holding the
// account password — the same approach used by hydroxide/ferroxide. Every
// field is encrypted with a key derived from PCS_ENCRYPTION_KEY and the stored
// Salt; the key itself is never persisted.
type Credentials struct {
	Salt               []byte
	ProtonUIDEnc       []byte
	ProtonRefreshEnc   []byte
	ProtonKeyPassEnc   []byte
	CardDAVPasswordEnc []byte
}

// Open opens (or creates) the SQLite database at path, enables WAL mode, and
// runs schema migrations. The parent directory is created if missing; SQLite
// creates the database file itself, but only when its directory already exists.
func Open(path string) (*sql.DB, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("create database directory %q: %w", dir, err)
		}
	}

	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open sqlite3 at %q: %w", path, err)
	}

	if err := migrate(db); err != nil {
		// go-defensive: defer cleanup — close on error so caller never holds a
		// half-initialised handle.
		_ = db.Close()
		return nil, fmt.Errorf("migrate schema: %w", err)
	}

	return db, nil
}

func migrate(db *sql.DB) error {
	const schema = `
CREATE TABLE IF NOT EXISTS credentials (
    id                   INTEGER PRIMARY KEY CHECK (id = 1),
    salt                 BLOB    NOT NULL,
    proton_uid_enc       BLOB    NOT NULL DEFAULT x'',
    proton_refresh_enc   BLOB    NOT NULL DEFAULT x'',
    proton_keypass_enc   BLOB    NOT NULL DEFAULT x'',
    carddav_password_enc BLOB    NOT NULL
);

CREATE TABLE IF NOT EXISTS contacts (
    uid          TEXT    PRIMARY KEY,
    etag         TEXT    NOT NULL DEFAULT '',
    vcard_hash   TEXT    NOT NULL DEFAULT '',
    proton_base  TEXT    NOT NULL DEFAULT '',
    carddav_base TEXT    NOT NULL DEFAULT '',
    updated_at   INTEGER NOT NULL DEFAULT 0
);`

	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("exec schema: %w", err)
	}

	// Forward-compat: add columns to databases created under an older schema.
	// ALTER TABLE ADD COLUMN errors if the column already exists, so each is
	// guarded by a table-definition check.
	for _, col := range []string{"proton_uid_enc", "proton_refresh_enc", "proton_keypass_enc"} {
		if err := ensureColumn(db, "credentials", col,
			fmt.Sprintf("ALTER TABLE credentials ADD COLUMN %s BLOB NOT NULL DEFAULT x''", col)); err != nil {
			return err
		}
	}
	// proton_base / carddav_base hold each side's last-synced vCard, the per-side
	// bases for the three-way merge.
	for _, col := range []string{"proton_base", "carddav_base"} {
		if err := ensureColumn(db, "contacts", col,
			fmt.Sprintf("ALTER TABLE contacts ADD COLUMN %s TEXT NOT NULL DEFAULT ''", col)); err != nil {
			return err
		}
	}
	return nil
}

// ensureColumn adds a column via alterStmt when it is missing from table.
func ensureColumn(db *sql.DB, table, column, alterStmt string) error {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return fmt.Errorf("inspect %s columns: %w", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid        int
			name, ctyp string
			notNull    int
			dfltValue  sql.NullString
			pk         int
		)
		if scanErr := rows.Scan(&cid, &name, &ctyp, &notNull, &dfltValue, &pk); scanErr != nil {
			return fmt.Errorf("scan %s column: %w", table, scanErr)
		}
		if name == column {
			return rows.Err() // column already present
		}
	}
	if rows.Err() != nil {
		return fmt.Errorf("iterate %s columns: %w", table, rows.Err())
	}

	if _, err := db.Exec(alterStmt); err != nil {
		return fmt.Errorf("add %s.%s column: %w", table, column, err)
	}
	return nil
}

// SaveCredentials upserts the encrypted credential material.
func SaveCredentials(ctx context.Context, db *sql.DB, creds Credentials) error {
	const q = `
INSERT INTO credentials (id, salt, proton_uid_enc, proton_refresh_enc, proton_keypass_enc, carddav_password_enc)
VALUES (1, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    salt                 = excluded.salt,
    proton_uid_enc       = excluded.proton_uid_enc,
    proton_refresh_enc   = excluded.proton_refresh_enc,
    proton_keypass_enc   = excluded.proton_keypass_enc,
    carddav_password_enc = excluded.carddav_password_enc`

	if _, err := db.ExecContext(ctx, q,
		creds.Salt, creds.ProtonUIDEnc, creds.ProtonRefreshEnc, creds.ProtonKeyPassEnc, creds.CardDAVPasswordEnc,
	); err != nil {
		return fmt.Errorf("save credentials: %w", err)
	}
	return nil
}

// UpdateProtonRefresh persists a rotated Proton refresh token (already
// encrypted). Proton rotates the refresh token on every session resume, so the
// daemon must write the new value back or the next start will fail.
func UpdateProtonRefresh(ctx context.Context, db *sql.DB, refreshEnc []byte) error {
	const q = `UPDATE credentials SET proton_refresh_enc = ? WHERE id = 1`
	if _, err := db.ExecContext(ctx, q, refreshEnc); err != nil {
		return fmt.Errorf("update proton refresh token: %w", err)
	}
	return nil
}

// LoadCredentials retrieves the stored credential material.
// It returns ErrCredentialsNotFound when no row exists.
func LoadCredentials(ctx context.Context, db *sql.DB) (Credentials, error) {
	const q = `SELECT salt, proton_uid_enc, proton_refresh_enc, proton_keypass_enc, carddav_password_enc FROM credentials WHERE id = 1`

	var c Credentials
	err := db.QueryRowContext(ctx, q).Scan(
		&c.Salt, &c.ProtonUIDEnc, &c.ProtonRefreshEnc, &c.ProtonKeyPassEnc, &c.CardDAVPasswordEnc)
	if errors.Is(err, sql.ErrNoRows) {
		return Credentials{}, ErrCredentialsNotFound
	}
	if err != nil {
		return Credentials{}, fmt.Errorf("load credentials: %w", err)
	}
	return c, nil
}
