// Package protonmail wraps go-proton-api into a lifecycle-managed client.
package protonmail

import (
	"context"
	"errors"
	"fmt"

	proton "github.com/ProtonMail/go-proton-api"
	"github.com/ProtonMail/gopenpgp/v2/crypto"
)

// Sentinel errors.
var (
	// ErrNotLoggedIn is returned when an operation requires an authenticated
	// session but none exists.
	ErrNotLoggedIn = errors.New("not logged in")
	// ErrSessionExpired is returned when a stored session can no longer be
	// resumed (revoked or expired refresh token); the user must re-run init.
	ErrSessionExpired = errors.New("proton session expired: re-run 'proton-carddav-sync init'")
)

// Session is the durable, password-free Proton session persisted between runs.
// It carries enough to resume the API session (UID + refresh token) and to
// unlock the user keyring (the derived mailbox/key password), so the account
// password never has to be stored. Refresh tokens rotate on every resume.
type Session struct {
	UID          string
	RefreshToken string
	KeyPass      []byte
}

// Client manages an authenticated Proton Mail session.
// All methods are safe to call from a single goroutine.
// Thread-safety is not guaranteed — synchronise externally if needed.
type Client struct {
	manager *proton.Manager
	client  *proton.Client
	keyring *crypto.KeyRing
}

// NewClient creates a new unauthenticated Proton API client. appVersion sets
// the x-pm-appversion header; the upstream default is rejected by Proton, so an
// empty value falls back to nothing and the caller is expected to supply one.
func NewClient(appVersion string) *Client {
	opts := []proton.Option{}
	if appVersion != "" {
		opts = append(opts, proton.WithAppVersion(appVersion))
	}
	return &Client{
		manager: proton.New(opts...),
	}
}

// LoginWithPassword authenticates with username/password, completing TOTP 2FA
// via totp() when the account requires it, unlocks the keyring, and returns a
// durable Session for later password-free resumes.
//
// Only one-password accounts are supported (mailbox password == login
// password). go-logging: passwords/keys are never logged.
func (c *Client) LoginWithPassword(ctx context.Context, username, password string, totp func() (string, error)) (Session, error) {
	client, auth, err := c.manager.NewClientWithLogin(ctx, username, []byte(password))
	if err != nil {
		return Session{}, fmt.Errorf("proton login for %q: %w", username, err)
	}
	c.client = client

	if auth.TwoFA.Enabled&proton.HasTOTP != 0 {
		if totp == nil {
			return Session{}, errors.New("proton account requires TOTP but no code was provided")
		}
		code, codeErr := totp()
		if codeErr != nil {
			return Session{}, fmt.Errorf("read totp code: %w", codeErr)
		}
		if twoFAErr := client.Auth2FA(ctx, proton.Auth2FAReq{TwoFactorCode: code}); twoFAErr != nil {
			return Session{}, fmt.Errorf("proton 2fa: %w", twoFAErr)
		}
	}

	keyPass, err := c.unlock(ctx, password)
	if err != nil {
		return Session{}, err
	}

	return Session{UID: auth.UID, RefreshToken: auth.RefreshToken, KeyPass: keyPass}, nil
}

// ResumeSession resumes a previously stored Session without the account
// password. onRefresh is invoked with each rotated refresh token (immediately
// for the resume, and again on any mid-session auto-refresh) so the caller can
// persist it. Returns ErrSessionExpired when the refresh token is no longer
// valid.
func (c *Client) ResumeSession(ctx context.Context, sess Session, onRefresh func(refreshToken string)) error {
	client, auth, err := c.manager.NewClientWithRefresh(ctx, sess.UID, sess.RefreshToken)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrSessionExpired, err)
	}
	c.client = client

	if onRefresh != nil {
		onRefresh(auth.RefreshToken)
		client.AddAuthHandler(func(a proton.Auth) { onRefresh(a.RefreshToken) })
	}

	user, err := client.GetUser(ctx)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}
	kr, err := user.Keys.Unlock(sess.KeyPass, nil)
	if err != nil {
		return fmt.Errorf("unlock keyring: %w", err)
	}
	c.keyring = kr
	return nil
}

// unlock fetches the user and salts, derives the mailbox key password for the
// primary key, and unlocks the keyring. It returns the derived key password so
// it can be persisted for password-free resumes.
func (c *Client) unlock(ctx context.Context, password string) ([]byte, error) {
	user, err := c.client.GetUser(ctx)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}

	salts, err := c.client.GetSalts(ctx)
	if err != nil {
		return nil, fmt.Errorf("get key salts: %w", err)
	}

	keyPass, err := salts.SaltForKey([]byte(password), user.Keys.Primary().ID)
	if err != nil {
		return nil, fmt.Errorf("derive key password: %w", err)
	}

	kr, err := user.Keys.Unlock(keyPass, nil)
	if err != nil {
		return nil, fmt.Errorf("unlock keyring: %w", err)
	}
	c.keyring = kr
	return keyPass, nil
}

// Close drops the local session state without revoking it server-side, so the
// stored refresh token stays valid for the next run. Use Logout to revoke.
func (c *Client) Close() {
	if c.client != nil {
		c.client.Close()
		c.client = nil
	}
}

// Logout revokes the Proton session server-side. This invalidates the stored
// refresh token, so the user must re-run init afterwards.
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
// Returns ErrNotLoggedIn if no session has been established.
func (c *Client) Keyring() (*crypto.KeyRing, error) {
	if c.client == nil {
		return nil, ErrNotLoggedIn
	}
	return c.keyring, nil
}

// Raw returns the underlying *proton.Client for operations not wrapped by this
// package.
// Returns ErrNotLoggedIn if no session has been established.
func (c *Client) Raw() (*proton.Client, error) {
	if c.client == nil {
		return nil, ErrNotLoggedIn
	}
	return c.client, nil
}
