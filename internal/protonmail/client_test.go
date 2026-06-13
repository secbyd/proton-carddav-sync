package protonmail

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUserAgentTransportSetsHeader(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("User-Agent")
	}))
	defer srv.Close()

	rt := &userAgentTransport{base: http.DefaultTransport, ua: defaultUserAgent}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	// resty/Go would otherwise set its own default UA here.
	req.Header.Set("User-Agent", "go-resty/should-be-overridden")

	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	resp.Body.Close()

	if got != defaultUserAgent {
		t.Fatalf("server saw User-Agent %q, want %q", got, defaultUserAgent)
	}
}
