package cli

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

var idleCmd = &cobra.Command{
	Use:   "idle",
	Short: "Keep the process running without syncing (for interactive setup in a container)",
	Long: `Idle does nothing but stay alive until it receives SIGINT/SIGTERM.

It exists so a container can be started before it has been configured: run the
container with this command, then attach and configure it interactively, e.g.

  docker exec -it <container> proton-carddav-sync init --config /config/config.yaml

(which prompts for passwords and a TOTP code). Once configured, restart the
container with its default command ('run') to begin syncing.`,
	RunE: runIdle,
}

func runIdle(cmd *cobra.Command, _ []string) error {
	parent := cmd.Context()
	if parent == nil {
		parent = context.Background()
	}
	ctx, stop := signal.NotifyContext(parent, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fmt.Println("proton-carddav-sync: idle — running but NOT syncing.")
	fmt.Println("Configure it from another shell, then restart with the default command:")
	fmt.Println("  docker exec -it <container> proton-carddav-sync init --config /config/config.yaml")

	<-ctx.Done()
	fmt.Println("proton-carddav-sync: idle stopped.")
	return nil
}
