// Package protonmail wraps the go-proton-api client.
package protonmail

import (
	"context"
	"fmt"

	proton "github.com/ProtonMail/go-proton-api"
)

// Client wraps the ProtonMail API manager and active session.
type Client struct {
	manager *proton.Manager
	client  *proton.Client
	ctx     context.Context
}

// New creates a new authenticated ProtonMail client.
func New(ctx context.Context, username, password string) (*Client, error) {
	mgr := proton.New(
		proton.WithHostURL(proton.DefaultHostURL),
	)

	client, _, err := mgr.NewClientWithLogin(ctx, username, []byte(password))
	if err != nil {
		return nil, fmt.Errorf("protonmail login: %w", err)
	}
	return &Client{manager: mgr, client: client, ctx: ctx}, nil
}

// Close logs out and frees the session.
func (c *Client) Close() error {
	if err := c.client.AuthDelete(c.ctx); err != nil {
		return fmt.Errorf("protonmail logout: %w", err)
	}
	c.manager.Close()
	return nil
}
