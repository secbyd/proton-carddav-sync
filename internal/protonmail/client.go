package protonmail

import (
	"context"
	"fmt"

	proton "github.com/ProtonMail/go-proton-api"
	"github.com/ProtonMail/gopenpgp/v2/crypto"
)

// Client wraps a go-proton-api client together with the user keyring.
type Client struct {
	mgr    *proton.Manager
	client *proton.Client
	kr     *crypto.KeyRing
}

// NewClient creates a Proton Manager, authenticates, and returns a ready Client.
func NewClient(ctx context.Context, username, password string) (*Client, error) {
	mgr := proton.New(
		proton.WithHostURL("https://mail-api.proton.me"),
	)

	client, auth, err := mgr.NewClientWithLogin(ctx, username, []byte(password))
	if err != nil {
		return nil, fmt.Errorf("proton login: %w", err)
	}
	_ = auth // auth holds session info; the client manages re-auth internally

	// Unlock the user key to obtain the keyring for contact card decryption.
	salts, err := client.GetKeySalts(ctx)
	if err != nil {
		return nil, fmt.Errorf("get key salts: %w", err)
	}

	user, err := client.GetUser(ctx)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	kr, err := user.Keys.Unlock(salts, []byte(password))
	if err != nil {
		return nil, fmt.Errorf("unlock user keyring: %w", err)
	}

	return &Client{
		mgr:    mgr,
		client: client,
		kr:     kr,
	}, nil
}

// Close logs out and cleans up.
func (c *Client) Close() {
	_ = c.client.AuthDelete(context.Background())
	c.mgr.Close()
}

// KeyRing exposes the user keyring (needed for contact card encoding/decoding).
func (c *Client) KeyRing() *crypto.KeyRing {
	return c.kr
}

// Underlying returns the raw go-proton-api client.
func (c *Client) Underlying() *proton.Client {
	return c.client
}
