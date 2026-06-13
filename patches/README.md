# Patches

This directory contains patches for upstream Go module dependencies that cannot
be fixed in-place because they are read-only entries in the module cache.

## go-proton-api-address-sortfunc.patch

**Affects:** `github.com/ProtonMail/go-proton-api v0.4.0`  
**File patched:** `address.go`  
**Root cause:** Go 1.21 changed `slices.SortFunc` to require a comparison
function returning `int` (negative / zero / positive) instead of `bool`.
`go-proton-api@v0.4.0` still uses the old `golang.org/x/exp/slices` with the
`bool`-returning form, which no longer compiles under Go 1.21+.

### Recommended fix — `replace` directive

The cleanest way to apply this without touching the module cache is to fork
the module and add a `replace` directive to `go.mod`:

```
# go.mod
replace github.com/ProtonMail/go-proton-api v0.4.0 => github.com/secbyd/go-proton-api v0.4.1-patched
```

Until that fork is published, the CI workflow applies the patch directly to the
module cache as a pre-build step (see `.github/workflows/ci.yml`).

### Manual application

```bash
MOD=$(go env GOMODCACHE)/github.com/\!proton\!mail/go-proton-api@v0.4.0
chmod -R u+w "$MOD"
patch -p1 -d "$MOD" < patches/go-proton-api-address-sortfunc.patch
```
