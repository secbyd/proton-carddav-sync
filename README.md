# proton-carddav-sync

A daemon that keeps your **ProtonMail contacts** in sync with any **CardDAV** address book.

---

## Features

- Bidirectional sync (ProtonMail ↔ CardDAV)
- Incremental, state-tracked syncs via SQLite
- End-to-end encrypted credential storage
- vCard merge with conflict resolution
- Configurable sync interval and direction
- Structured logging (text or JSON)

---

## Installation

```bash
git clone https://github.com/secbyd/proton-carddav-sync
cd proton-carddav-sync
go build -o proton-carddav-sync ./cmd/proton-carddav-sync
```

> **Requires Go 1.22+**

---

## Quick Start

```bash
# 1. Copy and edit the example config
cp config.yaml.example config.yaml
$EDITOR config.yaml

# 2. Initialise the database (stores credentials encrypted at rest)
proton-carddav-sync init

# 3. Run a one-shot sync
proton-carddav-sync sync

# 4. Start the background daemon
proton-carddav-sync run
```

---

## Configuration

See [`config.yaml.example`](config.yaml.example) for all available options.

| Key | Default | Description |
|-----|---------|-------------|
| `proton.username` | — | ProtonMail login |
| `carddav.url` | — | CardDAV address-book URL |
| `sync.interval` | `1h` | How often the daemon syncs |
| `sync.direction` | `both` | Sync direction |
| `sync.db_path` | `~/.proton-carddav-sync/state.db` | SQLite state file |
| `log.level` | `info` | Logging verbosity |

---

## Architecture

```
cmd/proton-carddav-sync/
  main.go                 ← entry point
internal/
  cli/                    ← Cobra commands (root, init, sync, run)
  config/                 ← Viper config loader
  crypto/                 ← AES-GCM credential encryption
  db/                     ← SQLite state + contact store
  log/                    ← Zap logger initialisation
  protonmail/             ← ProtonMail API client + contacts
  carddav/                ← CardDAV client
  vcardsync/              ← vCard merge / conflict resolution
  syncer/                 ← Orchestrates a full sync cycle
```

---

## Acknowledgements

- [go-proton-api](https://github.com/ProtonMail/go-proton-api)
- [go-webdav](https://github.com/emersion/go-webdav)
- [go-vcard](https://github.com/emersion/go-vcard)
- [hydroxide](https://github.com/emersion/hydroxide) (inspiration)

---

## License

MIT — see [LICENSE](LICENSE).
