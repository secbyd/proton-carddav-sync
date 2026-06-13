// Package protonmail wraps go-proton-api into a lifecycle-managed client.
package protonmail

import (
	"context"
	"errors"
	"fmt"

	proton "github.com/ProtonMail/go-proton-api"
)

// Sentinel errors.
var (
	// ErrNotLoggedIn is returned when an operation requires an authenticated
	// session but none exists.
	ErrNotLoggedIn = errors.New("not logged in")
)

// Client manages an authenticated Proton Mail session.
// All methods are safe to call from a single goroutine.
// Thread-safety is not guaranteed — synchronise externally if needed.
type Client struct {
	manager *proton.Manager
	client  *proton.Client
	keyring proton.Keyring
}

// NewClient creates a new unauthenticated Proton API client.
func NewClient() *Client {
	return &Client{
		manager: proton.New(),
	}
}

// Login authenticates with the Proton API and unlocks the keyring.
// ctx must not be stored in the returned Client (go-context: no struct storage).
func (c *Client) Login(ctx context.Context, username, password string) error {
	client, auth, err := c.manager.NewClientWithLogin(ctx, username, []byte(password))
	if err != nil {
		return fmt.Errorf("proton login for %q: %w", username, err)
	}
	c.client = client

	// Unlock the user keyring so we can decrypt contact cards.
	user, err := c.client.GetUser(ctx)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}

	salts, err := c.client.GetKeySalts(ctx)
	if err != nil {
		return fmt.Errorf("get key salts: %w", err)
	}

	keyPass, err := salts.KeyPassword(auth.KeySalt, []byte(password))
	if err != nil {
		return fmt.Errorf("derive key password: %w", err)
	}

	kr, err := user.Keys.Unlock(keyPass, nil)
	if err != nil {
		return fmt.Errorf("unlock keyring: %w", err)
	}
	c.keyring = kr

	return nil
}

// Logout closes the Proton session.
func (c *Client) Logout(ctx context.Context) error {
	if c.client == nil {
		return nil
	}
	if err := c.client.AuthDelete(ctx); err != nil {
		return fmt.Errorf("proton logout: %w", err)
	}
	c.client = nil
	return nil
}

// Keyring returns the unlocked user keyring.
// Returns ErrNotLoggedIn if Login has not been called.
func (c *Client) Keyring() (proton.Keyring, error) {
	if c.client == nil {
		return nil, ErrNotLoggedIn
	}
	return c.keyring, nil
}

// Raw returns the underlying *proton.Client for operations not wrapped by this
// package.
// Returns ErrNotLoggedIn if Login has not been called.
func (c *Client) Raw() (*proton.Client, error) {
	if c.client == nil {
		return nil, ErrNotLoggedIn
	}
	return c.client, nil
}
