# Patches

This directory documents patches applied to upstream Go module dependencies
that cannot be fixed in-place because the upstream projects are unmaintained for
current Go toolchains and we have no ability to publish fixed releases.

The patches are **applied in the committed `vendor/` directory** so that the
build is reproducible locally and in CI with no module-cache surgery. The files
here are reference copies of the diffs that have been baked into `vendor/`.

## go-proton-api-address-sortfunc.patch

**Affects:** `github.com/ProtonMail/go-proton-api v0.4.0`
**File patched:** `address.go`
**Vendored at:** `vendor/github.com/ProtonMail/go-proton-api/address.go`

**Root cause:** Go 1.21 changed `slices.SortFunc` to require a comparison
function returning `int` (negative / zero / positive) instead of `bool`. The
pinned `golang.org/x/exp/slices` already ships the new `int`-returning
signature, so the upstream `bool`-returning call in `address.go` no longer
compiles under Go 1.21+.

**Why not switch to the stdlib `cmp`/`slices` packages?** `go-proton-api`'s own
`go.mod` declares `go 1.18`. The Go toolchain forbids a module from importing
standard-library packages newer than its declared language version, so
importing `cmp` or `slices` from inside `go-proton-api` fails with
`could not import cmp`. The patch therefore keeps the existing
`golang.org/x/exp/slices` import and only rewrites the comparator body.

### How the patch is maintained

The patch lives in `vendor/` and is committed. **Do not run `go mod vendor`
without re-applying it** — `go mod vendor` regenerates `vendor/` faithfully from
the module cache and will revert this change. CI builds with `-mod=vendor` and
never re-vendors.

If you ever need to regenerate `vendor/`:

```bash
go mod vendor
git apply patches/go-proton-api-address-sortfunc.patch  # or hand-apply the diff
go build -mod=vendor ./...                               # verify
```

## go-proton-api-auth-v3.patch

**Affects:** `github.com/ProtonMail/go-proton-api v0.4.0`
**Files patched:** `manager_auth.go`, `auth.go`
**Vendored at:** `vendor/github.com/ProtonMail/go-proton-api/{manager_auth,auth}.go`

**Root cause:** go-proton-api authenticates against Proton's **v4** auth
endpoints (`/auth/v4/info`, `/auth/v4`, `/auth/v4/refresh`, `/auth/v4/2fa`,
`/auth/v4/sessions`). The v4 auth flow is **CAPTCHA-gated** (API error `9001`,
"please complete CAPTCHA") for non-browser clients — the same wall rclone's
Proton backend hits. The legacy **v3** endpoints (`/auth/info`, `/auth`,
`/auth/refresh`, …) used by hydroxide/ferroxide are not gated.

**Fix:** rewrite the auth endpoint paths from `/auth/v4/...` to `/auth/...`.
The SRP flow and request/response shapes are unchanged (Proton's v3 and v4 auth
share them), so go-proton-api's `AuthInfo`/`Auth` structs unmarshal the v3
responses unchanged. The client also sends `x-pm-apiversion: 3` and a browser
`User-Agent` (set in `internal/protonmail/client.go`) to complete the
hydroxide/ferroxide-style request. Verified against a live Proton account:
login, keyring unlock, and refresh-token resume all succeed with no CAPTCHA.

The maintenance rules are the same as the patch above — it lives in `vendor/`
and `go mod vendor` will revert it.

### Preferred long-term fix

Publish a forked release and add a `replace` directive, dropping the vendored
patch entirely:

```
# go.mod
replace github.com/ProtonMail/go-proton-api v0.4.0 => github.com/secbyd/go-proton-api v0.4.1-patched
```
