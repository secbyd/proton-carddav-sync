# syntax=docker/dockerfile:1

# ─── build ─────────────────────────────────────────────────────────────────
# golang:bookworm already ships gcc, so CGO (required by go-sqlite3) works with
# no extra packages. glibc here matches the glibc runtime below.
FROM golang:1.25-bookworm AS build

ENV CGO_ENABLED=1 GOTOOLCHAIN=local
WORKDIR /src

# vendor/ is committed, so the build needs no network for modules.
COPY . .

ARG VERSION=dev
RUN go build -mod=vendor -trimpath \
      -ldflags "-s -w -X main.version=${VERSION}" \
      -o /out/proton-carddav-sync ./cmd/proton-carddav-sync

# ─── runtime ───────────────────────────────────────────────────────────────
# debian:bookworm-slim (~30 MB) is glibc, so the dynamically-linked go-sqlite3
# binary runs as-is. For an even smaller, shell-less image you can swap this for
# gcr.io/distroless/base-debian12:nonroot.
FROM debian:bookworm-slim

RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates tzdata \
 && rm -rf /var/lib/apt/lists/* \
 && useradd --uid 10001 --create-home --home-dir /home/pcs pcs \
 && mkdir -p /config /data \
 && chown pcs:pcs /config /data

COPY --from=build /out/proton-carddav-sync /usr/local/bin/proton-carddav-sync

USER pcs

# /config holds config.yaml; /data holds the SQLite database (set
# database.path: /data/sync.db in your config). PCS_ENCRYPTION_KEY must be
# supplied at runtime (-e PCS_ENCRYPTION_KEY=...).
VOLUME ["/config", "/data"]

ENTRYPOINT ["proton-carddav-sync"]
CMD ["run", "--config", "/config/config.yaml"]
