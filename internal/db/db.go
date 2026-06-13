// Package db manages the SQLite state database.
package db

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

// DB wraps a SQLite connection.
type DB struct {
	*sql.DB
}

// Open opens (or creates) the SQLite database at path and applies migrations.
func Open(path string) (*DB, error) {
	conn, err := sql.Open("sqlite3", path+"?_journal=WAL&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	d := &DB{conn}
	if err := d.migrate(); err != nil {
		conn.Close()
		return nil, err
	}
	return d, nil
}

func (d *DB) migrate() error {
	schema := `
CREATE TABLE IF NOT EXISTS credentials (
    id              INTEGER PRIMARY KEY CHECK (id = 1),
    proton_password BLOB NOT NULL,
    carddav_password BLOB NOT NULL
);

CREATE TABLE IF NOT EXISTS contacts (
    uid             TEXT PRIMARY KEY,
    proton_id       TEXT,
    carddav_href    TEXT,
    proton_etag     TEXT,
    carddav_etag    TEXT,
    vcard           TEXT NOT NULL,
    last_synced_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);
`
	_, err := d.Exec(schema)
	return err
}

// StoreCredentials persists encrypted credential blobs.
func (d *DB) StoreCredentials(protonPw, cardDAVPw []byte) error {
	_, err := d.Exec(`
		INSERT INTO credentials (id, proton_password, carddav_password)
		VALUES (1, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			proton_password  = excluded.proton_password,
			carddav_password = excluded.carddav_password`,
		protonPw, cardDAVPw)
	return err
}

// LoadCredentials retrieves encrypted credential blobs.
func (d *DB) LoadCredentials() (protonPw, cardDAVPw []byte, err error) {
	row := d.QueryRow(`SELECT proton_password, carddav_password FROM credentials WHERE id = 1`)
	err = row.Scan(&protonPw, &cardDAVPw)
	return
}
