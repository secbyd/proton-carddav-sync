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

# 2. Build (uses the committed vendor/ tree; no extra setup needed)
go build -o proton-carddav-sync ./cmd/proton-carddav-sync

# 3. Initialise — creates the config and stores an encrypted session
#    PCS_ENCRYPTION_KEY is the master key that encrypts everything at rest
#    (the Proton session + CardDAV password). Pick a long, random value and keep
#    it secret — the daemon needs the same value to decrypt.
#
#    With no config present, `init` prompts for every setting and writes
#    config.yaml, then logs in to Proton (asking for a TOTP code if 2FA is on)
#    and stores a long-lasting session — so the daemon never needs your Proton
#    password again. No passwords are written to config.yaml.
export PCS_ENCRYPTION_KEY="a-long-random-passphrase"
./proton-carddav-sync init

# 4. One-shot sync (test) — resumes the stored session
PCS_ENCRYPTION_KEY="a-long-random-passphrase" ./proton-carddav-sync sync

# 5. Start daemon
PCS_ENCRYPTION_KEY="a-long-random-passphrase" ./proton-carddav-sync run
```

> **Credential model.** Inspired by [hydroxide](https://github.com/emersion/hydroxide)
> and ferroxide, `init` exchanges your password for a durable Proton session
> (UID + a rotating refresh token + the derived mailbox key) rather than keeping
> the password. That session and the CardDAV password are encrypted with a key
> derived from `PCS_ENCRYPTION_KEY` (PBKDF2) and stored in SQLite — never in
> `config.yaml`. The daemon resumes the session via the refresh token, which it
> rotates and re-stores on each run.

## Configuration

All settings live in `~/.config/proton-carddav-sync/config.yaml`, which
`init` creates for you (or copy [`config.yaml.example`](config.yaml.example) and
edit it by hand). No secrets are stored in this file.

| Key | Default | Description |
|-----|---------|-------------|
| `proton.username` | — | Proton Mail email address |
| `proton.app_version` | `Other` | `x-pm-appversion` sent to the Proton API (override with `PCS_PROTON_APP_VERSION`) |
| `carddav.url` | — | Full CardDAV collection URL |
| `carddav.username` | — | CardDAV username |
| `sync.direction` | `both` | `both` / `proton-to-carddav` / `carddav-to-proton` |
| `sync.interval_seconds` | `300` | Daemon sync interval, in seconds |
| `database.path` | `~/.local/share/…/sync.db` | SQLite state database path |
| `log.level` | `info` | `debug` / `info` / `warn` / `error` |
| `log.format` | `text` | `text` / `json` |

### Environment variables

| Variable | Required | Description |
|----------|----------|-------------|
| `PCS_ENCRYPTION_KEY` | yes | Master key that encrypts the stored Proton session and CardDAV password. The same value must be set for `init` and the daemon. |
| `PCS_PROTON_APP_VERSION` | no | Overrides `proton.app_version` at runtime, for tracking Proton's accepted client versions without editing the config. |
| `PCS_PROTON_USER_AGENT` | no | Overrides the browser `User-Agent` sent to Proton. Defaults to a recent Firefox UA; set it to your browser's exact value if you still hit CAPTCHA. |

## Troubleshooting

### Login fails with a CAPTCHA / "human verification" / "unusual activity" error

This is Proton's anti-abuse **human verification** (API error `9001`), not a bug
in how this tool talks to the API — it logs in through Proton's normal SRP auth
flow. Proton enforces it on logins it deems bot-like or risky.

The biggest signal is the **User-Agent**: the underlying library otherwise sends
a `go-resty/...` agent that Proton treats as a bot. This tool sends a real
browser User-Agent by default (the same approach that lets hydroxide/ferroxide
log in), which resolves the CAPTCHA for most accounts. If you still hit it:

1. **Match a real browser User-Agent and app version.** In your browser on
   `mail.proton.me` (DevTools → Network), copy the exact `User-Agent` and
   `x-pm-appversion` request headers and set `PCS_PROTON_USER_AGENT` and
   `PCS_PROTON_APP_VERSION` to those values.
2. **Sign in via the official Proton web/mobile app at least once** on the
   account, so it is fully provisioned.
3. **Run `init` from a residential connection** (datacenter/VPS/VPN IPs are
   flagged hardest). Credentials are a portable, long-lasting session in the
   SQLite database, so you can run `init` on your laptop at home and copy
   `~/.local/share/proton-carddav-sync/sync.db` (with the same
   `PCS_ENCRYPTION_KEY`) to the server that runs the daemon.
4. **Ask Proton to relax it** via the
   [appeal-abuse form](https://proton.me/support/appeal-abuse), mentioning you
   use a third-party client — the same advice the
   [rclone Proton community](https://github.com/rclone/rclone/issues/9397) gives.
5. **Wait and retry** — the limit is sometimes temporary.

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
PCS_ENCRYPTION_KEY=a-long-random-passphrase
```
Chmod it `600`. Use the same value you passed to `init`.

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
