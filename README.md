# proton-carddav-sync

A lightweight Go daemon that keeps your **Proton Mail contacts** in sync with any **CardDAV** server (Nextcloud, Radicale, Baikal, iCloud, …).

## Features

- Bidirectional sync (Proton ↔ CardDAV, or one-way)
- Three-way vCard merge with configurable conflict resolution
- AES-256-GCM encrypted credential storage (never stores passwords in plain text)
- SQLite state database for efficient incremental syncing
- Structured logging (text or JSON via zap)
- Systemd-friendly daemon mode with graceful shutdown

## Requirements

- Go 1.21+
- gcc (for cgo, required by `mattn/go-sqlite3`)

## Quick Start

```bash
# 1. Clone
git clone https://github.com/secbyd/proton-carddav-sync
cd proton-carddav-sync

# 2. Copy and edit config
mkdir -p ~/.config/proton-carddav-sync
cp config.yaml.example ~/.config/proton-carddav-sync/config.yaml
$EDITOR ~/.config/proton-carddav-sync/config.yaml

# 3. Build
go build -o proton-carddav-sync ./cmd/proton-carddav-sync

# 4. Store encrypted credentials
./proton-carddav-sync init

# 5. One-shot sync (test)
PROTON_PASSWORD=yourpassword ./proton-carddav-sync sync

# 6. Start daemon
PROTON_PASSWORD=yourpassword ./proton-carddav-sync run
```

## Configuration

All settings live in `~/.config/proton-carddav-sync/config.yaml`.
See [`config.yaml.example`](config.yaml.example) for an annotated reference.

| Key | Default | Description |
|-----|---------|-------------|
| `proton.username` | — | Proton Mail email address |
| `carddav.url` | — | Full CardDAV collection URL |
| `carddav.username` | — | CardDAV username |
| `sync.direction` | `both` | `both` / `proton-to-carddav` / `carddav-to-proton` |
| `sync.merge_strategy` | `prefer-newer` | `prefer-newer` / `prefer-proton` / `prefer-carddav` |
| `sync.interval` | `15m` | Daemon sync interval (Go duration string) |
| `db.path` | `~/.local/share/…/sync.db` | SQLite state database path |
| `log.level` | `info` | `debug` / `info` / `warn` / `error` |
| `log.format` | `text` | `text` / `json` |

## Systemd Unit

```ini
[Unit]
Description=Proton CardDAV Sync Daemon
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
EnvironmentFile=%h/.config/proton-carddav-sync/secrets.env
ExecStart=%h/bin/proton-carddav-sync run
Restart=on-failure
RestartSec=30s

[Install]
WantedBy=default.target
```

Create `~/.config/proton-carddav-sync/secrets.env`:
```
PROTON_PASSWORD=your_proton_password
```
Chmod it `600`.

## Architecture

```
cmd/
  proton-carddav-sync/main.go   ← binary entry point
internal/
  cli/                          ← cobra commands (init, sync, run)
  config/                       ← viper config loader
  crypto/                       ← AES-256-GCM + PBKDF2
  db/                           ← SQLite state (credentials + contacts)
  log/                          ← zap logger
  protonmail/                   ← go-proton-api wrapper
  carddav/                      ← go-webdav CardDAV wrapper
  vcardsync/                    ← three-way vCard merge
  syncer/                       ← orchestration + crypto shim
```

## License

GPL-3.0 — see [LICENSE](LICENSE).
