// Command proton-carddav-sync synchronises Proton Mail contacts with a CardDAV
// server.
package main

import "github.com/secbyd/proton-carddav-sync/internal/cli"

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	cli.Version = version
	cli.Execute()
}
