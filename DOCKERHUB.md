# proton-carddav-sync

Sync your **Proton Mail contacts** with any **CardDAV** server — Nextcloud,
Radicale, Baïkal, iCloud, and others. A small, self-hosted Go daemon.

📦 **Source & full docs:** https://github.com/secbyd/proton-carddav-sync

## What it does

- **Bidirectional sync** (Proton ↔ CardDAV) with a per-property **three-way
  merge**, so changing one field on one side never wipes fields the other side
  has. One-way modes are available too.
- **Password-free, long-lasting Proton session** (à la hydroxide/ferroxide): you
  log in once; the daemon resumes via a refresh token and never stores your
  account password.
- **Encrypted at rest** — the Proton session and CardDAV password are
  AES-256-GCM encrypted in a local SQLite database with a key derived from
  `PCS_ENCRYPTION_KEY`. Nothing secret is written to the config file.
- **Gentle on Proton** — all Proton API calls are rate-limited (default ~1/sec)
  to stay well under Proton's anti-abuse limits, with an optional per-run cap on
  new contacts for large first imports.
- **TOTP/2FA** supported during setup.

## Tags & architectures

- `latest` — the current `main` build
- `vX.Y.Z` — tagged releases
- `sha-<short>` — a specific commit

Images are multi-arch: **`linux/amd64`** and **`linux/arm64`**.

## Volumes & configuration

| Mount | Purpose |
|-------|---------|
| `/config` | holds `config.yaml` (set `database.path: /data/sync.db`) |
| `/data`   | holds the SQLite database (credentials + sync state) |

`PCS_ENCRYPTION_KEY` (a long random passphrase) must be passed at runtime — it is
never baked into the image, and the **same value** is required by `init` and the
daemon.

> The container runs as a non-root user (uid 10001). **Named volumes** (as below)
> work out of the box; if you bind-mount host directories, make them writable by
> uid 10001.

## Usage

### 1. Initialise (interactive — prompts for passwords + TOTP)

```bash
docker run --rm -it \
  -e PCS_ENCRYPTION_KEY="a-long-random-passphrase" \
  -v pcs-config:/config -v pcs-data:/data \
  secbyd/proton-carddav-sync \
  init --config /config/config.yaml
```

With no config present, `init` asks for every setting (set `database.path` to
`/data/sync.db`), logs in to Proton (asking for a TOTP code if 2FA is enabled),
and stores the encrypted session.

If your platform can't run an interactive one-off container (e.g. Portainer or
Unraid), start the container with the **`idle`** command instead, then:

```bash
docker exec -it <container> proton-carddav-sync init --config /config/config.yaml
```

…and restart it with the default command afterwards.

### 2. Run the daemon

```bash
docker run -d --name proton-carddav-sync --restart unless-stopped \
  -e PCS_ENCRYPTION_KEY="a-long-random-passphrase" \
  -v pcs-config:/config -v pcs-data:/data \
  secbyd/proton-carddav-sync
```

### docker compose

```yaml
services:
  proton-carddav-sync:
    image: secbyd/proton-carddav-sync:latest
    restart: unless-stopped
    environment:
      PCS_ENCRYPTION_KEY: "a-long-random-passphrase"
    volumes:
      - pcs-config:/config
      - pcs-data:/data
volumes:
  pcs-config:
  pcs-data:
```

Run `init` once before `up -d`:
`docker compose run --rm -it proton-carddav-sync init --config /config/config.yaml`

## Environment variables

| Variable | Required | Description |
|----------|----------|-------------|
| `PCS_ENCRYPTION_KEY` | yes | Master key encrypting the stored Proton session + CardDAV password |
| `PCS_PROTON_APP_VERSION` | no | Override the Proton `x-pm-appversion` header |
| `PCS_PROTON_USER_AGENT` | no | Override the browser `User-Agent` sent to Proton |

## Notes

- Proton enforces a strict Acceptable Use Policy. A first sync of a large address
  book is paced automatically; for hundreds of contacts you can also cap new
  creates per run. See the GitHub README for details.
- Built on a glibc `debian:bookworm-slim` base so the CGO SQLite driver works
  without musl/static-link complications.

## License

See the [repository](https://github.com/secbyd/proton-carddav-sync) for license
and full documentation.
