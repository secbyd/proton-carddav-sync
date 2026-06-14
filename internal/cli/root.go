// Package cli defines the cobra command tree for proton-carddav-sync.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var cfgFile string

// Version is the build version, set from main (which receives it via ldflags).
var Version = "dev"

// rootCmd is the base command when called without any sub-commands.
var rootCmd = &cobra.Command{
	Use:   "proton-carddav-sync",
	Short: "Synchronise Proton Mail contacts with a CardDAV server",
	Long: `proton-carddav-sync keeps your Proton Mail contacts in sync with any
CardDAV server (Nextcloud, Radicale, iCloud, etc.).

Run 'proton-carddav-sync init' once to store encrypted credentials, then
'proton-carddav-sync run' to start the background daemon.`,
}

// Execute adds all child commands to the root command and runs it.
// Errors are written to stderr and the process exits with code 1.
func Execute() {
	rootCmd.Version = Version
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "",
		"config file (default: $HOME/.config/proton-carddav-sync/config.yaml)")

	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(syncCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(idleCmd)
}
