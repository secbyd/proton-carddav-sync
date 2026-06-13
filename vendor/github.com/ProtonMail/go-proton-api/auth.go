package proton

import (
	"context"

	"github.com/go-resty/resty/v2"
)

// PATCH(secbyd/proton-carddav-sync): all auth endpoints use the v3 API
// (/auth/...) instead of /auth/v4/...; the v4 auth flow is CAPTCHA-gated while
// the legacy v3 flow used by hydroxide/ferroxide is not. See
// patches/go-proton-api-auth-v3.patch.
func (c *Client) Auth2FA(ctx context.Context, req Auth2FAReq) error {
	return c.do(ctx, func(r *resty.Request) (*resty.Response, error) {
		return r.SetBody(req).Post("/auth/2fa")
	})
}

func (c *Client) AuthDelete(ctx context.Context) error {
	return c.do(ctx, func(r *resty.Request) (*resty.Response, error) {
		return r.Delete("/auth")
	})
}

func (c *Client) AuthSessions(ctx context.Context) ([]AuthSession, error) {
	var res struct {
		Sessions []AuthSession
	}

	if err := c.do(ctx, func(r *resty.Request) (*resty.Response, error) {
		return r.SetResult(&res).Get("/auth/sessions")
	}); err != nil {
		return nil, err
	}

	return res.Sessions, nil
}

func (c *Client) AuthRevoke(ctx context.Context, authUID string) error {
	return c.do(ctx, func(r *resty.Request) (*resty.Response, error) {
		return r.Delete("/auth/sessions/" + authUID)
	})
}

func (c *Client) AuthRevokeAll(ctx context.Context) error {
	return c.do(ctx, func(r *resty.Request) (*resty.Response, error) {
		return r.Delete("/auth/sessions")
	})
}
