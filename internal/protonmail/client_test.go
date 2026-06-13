package protonmail

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestUserAgentTransportSetsHeader(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("User-Agent")
	}))
	defer srv.Close()

	rt := &protonTransport{base: http.DefaultTransport, ua: defaultUserAgent}
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

func TestRateLimiter(t *testing.T) {
	if newRateLimiter(0) != nil {
		t.Error("newRateLimiter(0) should be nil (no limiting)")
	}

	// nil limiter never blocks.
	var nilLimiter *rateLimiter
	if err := nilLimiter.wait(context.Background()); err != nil {
		t.Errorf("nil limiter wait: %v", err)
	}

	// Three calls spaced by a 40ms interval take at least ~2 intervals.
	l := &rateLimiter{interval: 40 * time.Millisecond}
	ctx := context.Background()
	start := time.Now()
	for i := 0; i < 3; i++ {
		if err := l.wait(ctx); err != nil {
			t.Fatalf("wait %d: %v", i, err)
		}
	}
	if elapsed := time.Since(start); elapsed < 70*time.Millisecond {
		t.Errorf("3 paced calls took %v, want >= ~80ms", elapsed)
	}

	// A cancelled context aborts the wait promptly.
	slow := &rateLimiter{interval: time.Hour}
	if err := slow.wait(context.Background()); err != nil { // consume the immediate slot
		t.Fatalf("prime slow limiter: %v", err)
	}
	cctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := slow.wait(cctx); err == nil {
		t.Error("wait with cancelled context should return an error")
	}
}
