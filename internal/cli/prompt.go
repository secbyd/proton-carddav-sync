package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"

	"github.com/secbyd/proton-carddav-sync/internal/config"
)

// promptLine reads a single trimmed line from r, showing prompt with an
// optional default (used when the user just presses Enter).
func promptLine(r *bufio.Reader, prompt, def string) (string, error) {
	if def != "" {
		fmt.Printf("%s [%s]: ", prompt, def)
	} else {
		fmt.Printf("%s: ", prompt)
	}
	line, err := r.ReadString('\n')
	if err != nil && (err != io.EOF || line == "") {
		return "", fmt.Errorf("read input: %w", err)
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return def, nil
	}
	return line, nil
}

// promptSecret reads a password without echoing it to the terminal.
func promptSecret(prompt string) (string, error) {
	fmt.Print(prompt + ": ")
	b, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", fmt.Errorf("read secret: %w", err)
	}
	return string(b), nil
}

// gatherConfigInteractively prompts for every config.yaml field (no secrets)
// and returns a populated, validated-by-Save Config. Defaults mirror those in
// the config package.
func gatherConfigInteractively() (*config.Config, error) {
	r := bufio.NewReader(os.Stdin)
	fmt.Println("No config file found — let's create one. Press Enter to accept defaults.")

	cfg := &config.Config{}
	var err error

	if cfg.Proton.Username, err = promptLine(r, "Proton Mail username (email)", ""); err != nil {
		return nil, err
	}
	if cfg.Proton.AppVersion, err = promptLine(r, "Proton app version", config.DefaultProtonAppVersion); err != nil {
		return nil, err
	}
	if cfg.CardDAV.URL, err = promptLine(r, "CardDAV collection URL", ""); err != nil {
		return nil, err
	}
	if cfg.CardDAV.Username, err = promptLine(r, "CardDAV username", ""); err != nil {
		return nil, err
	}
	if cfg.Sync.Direction, err = promptLine(r, "Sync direction (both|proton-to-carddav|carddav-to-proton)", "both"); err != nil {
		return nil, err
	}
	if cfg.Sync.Conflict, err = promptLine(r, "Conflict policy (prefer-newer|prefer-proton|prefer-carddav)", "prefer-newer"); err != nil {
		return nil, err
	}
	// Proton write-pacing: keep the safe defaults rather than prompting.
	cfg.Sync.ProtonMaxRequestsPerMinute = config.DefaultProtonMaxRequestsPerMinute
	cfg.Sync.MaxNewProtonContactsPerRun = config.DefaultMaxNewProtonContactsPerRun

	intervalStr, err := promptLine(r, "Sync interval (seconds)", "300")
	if err != nil {
		return nil, err
	}
	cfg.Sync.IntervalSeconds, err = strconv.Atoi(intervalStr)
	if err != nil {
		return nil, fmt.Errorf("invalid interval %q: %w", intervalStr, err)
	}

	if cfg.Database.Path, err = promptLine(r, "Database path", "~/.local/share/proton-carddav-sync/sync.db"); err != nil {
		return nil, err
	}
	if cfg.Log.Level, err = promptLine(r, "Log level (debug|info|warn|error)", "info"); err != nil {
		return nil, err
	}
	if cfg.Log.Format, err = promptLine(r, "Log format (text|json)", "text"); err != nil {
		return nil, err
	}

	return cfg, nil
}
