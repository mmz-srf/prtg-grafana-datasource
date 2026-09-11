package prtg

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAPIKeyAuthenticator_Token(t *testing.T) {
	auth := &APIKeyAuthenticator{APIKey: "my-key"}

	token, err := auth.Token(context.Background(), false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "my-key" {
		t.Fatalf("expected 'my-key', got %q", token)
	}

	// force=true shouldn't matter -- API keys never expire.
	token, err = auth.Token(context.Background(), true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "my-key" {
		t.Fatalf("expected 'my-key', got %q", token)
	}
}

func TestAPIKeyAuthenticator_EmptyKey(t *testing.T) {
	auth := &APIKeyAuthenticator{}
	if _, err := auth.Token(context.Background(), false); err == nil {
		t.Fatal("expected an error for an empty API key")
	}
}

// fakeClock is a manually-advanced clock for deterministic expiry tests.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock(start time.Time) *fakeClock {
	return &fakeClock{now: start}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// sessionServer is a minimal test double for PRTG APIv2's POST /session.
type sessionServer struct {
	mu          sync.Mutex
	logins      int32
	nextToken   string
	nextExpires string // pre-formatted expires_at, or "" for none
	statusCode  int
	errBody     string
}

func newSessionServer() *sessionServer {
	return &sessionServer{nextToken: "token-1", statusCode: http.StatusOK}
}

func (s *sessionServer) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&s.logins, 1)

		s.mu.Lock()
		status := s.statusCode
		token := s.nextToken
		expires := s.nextExpires
		errBody := s.errBody
		s.mu.Unlock()

		if status != http.StatusOK {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = w.Write([]byte(errBody))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"token":      token,
			"expires_at": expires,
		})
	}
}

func (s *sessionServer) loginCount() int {
	return int(atomic.LoadInt32(&s.logins))
}

func mustParseTestBaseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := ParseServerURL(raw)
	if err != nil {
		t.Fatalf("ParseServerURL(%q): %v", raw, err)
	}
	return u
}

func TestSessionAuthenticator_LoginAndCache(t *testing.T) {
	srv := newSessionServer()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/session", srv.handler())
	ts := httptest.NewServer(mux)
	defer ts.Close()

	clock := newFakeClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	srv.nextExpires = clock.Now().Add(1 * time.Hour).Format(time.RFC3339)

	auth := &SessionAuthenticator{
		BaseURL:    mustParseTestBaseURL(t, ts.URL),
		HTTPClient: ts.Client(),
		Username:   "alice",
		Password:   "secret",
		Now:        clock.Now,
	}

	token, err := auth.Token(context.Background(), false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "token-1" {
		t.Fatalf("expected 'token-1', got %q", token)
	}
	if got := srv.loginCount(); got != 1 {
		t.Fatalf("expected 1 login call, got %d", got)
	}

	// Second call, well within expiry: should be served from cache, no new
	// login call.
	clock.Advance(1 * time.Minute)
	token, err = auth.Token(context.Background(), false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "token-1" {
		t.Fatalf("expected cached 'token-1', got %q", token)
	}
	if got := srv.loginCount(); got != 1 {
		t.Fatalf("expected still 1 login call (cache hit), got %d", got)
	}
}

func TestSessionAuthenticator_ProactiveRefreshNearExpiry(t *testing.T) {
	srv := newSessionServer()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/session", srv.handler())
	ts := httptest.NewServer(mux)
	defer ts.Close()

	clock := newFakeClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	srv.nextExpires = clock.Now().Add(1 * time.Minute).Format(time.RFC3339)

	auth := &SessionAuthenticator{
		BaseURL:     mustParseTestBaseURL(t, ts.URL),
		HTTPClient:  ts.Client(),
		Username:    "alice",
		Password:    "secret",
		Now:         clock.Now,
		RefreshSkew: 30 * time.Second,
	}

	if _, err := auth.Token(context.Background(), false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := srv.loginCount(); got != 1 {
		t.Fatalf("expected 1 login call, got %d", got)
	}

	// Advance past (expiresAt - refreshSkew): 1 minute - 30s = 30s in.
	// Advancing 40s means we're now within the refresh skew of expiry.
	clock.Advance(40 * time.Second)
	srv.nextToken = "token-2"
	srv.nextExpires = clock.Now().Add(1 * time.Hour).Format(time.RFC3339)

	token, err := auth.Token(context.Background(), false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "token-2" {
		t.Fatalf("expected proactively-refreshed 'token-2', got %q", token)
	}
	if got := srv.loginCount(); got != 2 {
		t.Fatalf("expected 2 login calls (proactive refresh), got %d", got)
	}
}

func TestSessionAuthenticator_ForceRefresh(t *testing.T) {
	srv := newSessionServer()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/session", srv.handler())
	ts := httptest.NewServer(mux)
	defer ts.Close()

	clock := newFakeClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	srv.nextExpires = clock.Now().Add(1 * time.Hour).Format(time.RFC3339)

	auth := &SessionAuthenticator{
		BaseURL:    mustParseTestBaseURL(t, ts.URL),
		HTTPClient: ts.Client(),
		Username:   "alice",
		Password:   "secret",
		Now:        clock.Now,
	}

	if _, err := auth.Token(context.Background(), false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	srv.nextToken = "token-forced"
	// force=true must re-login even though the cached token is nowhere
	// near expiry -- this is the retry-once-on-401 safety net's mechanism.
	token, err := auth.Token(context.Background(), true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "token-forced" {
		t.Fatalf("expected 'token-forced', got %q", token)
	}
	if got := srv.loginCount(); got != 2 {
		t.Fatalf("expected 2 login calls, got %d", got)
	}
}

func TestSessionAuthenticator_UnparseableExpiryTreatedAsExpired(t *testing.T) {
	srv := newSessionServer()
	srv.nextExpires = "not-a-timestamp"
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/session", srv.handler())
	ts := httptest.NewServer(mux)
	defer ts.Close()

	clock := newFakeClock(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	auth := &SessionAuthenticator{
		BaseURL:    mustParseTestBaseURL(t, ts.URL),
		HTTPClient: ts.Client(),
		Username:   "alice",
		Password:   "secret",
		Now:        clock.Now,
	}

	// First call still succeeds and returns a usable token even though
	// expires_at didn't parse.
	token, err := auth.Token(context.Background(), false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "token-1" {
		t.Fatalf("expected 'token-1', got %q", token)
	}

	// But the bad expiry should be treated as "already expired", so the
	// very next call re-logs-in rather than caching indefinitely.
	if _, err := auth.Token(context.Background(), false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := srv.loginCount(); got != 2 {
		t.Fatalf("expected 2 login calls (no caching of unparseable expiry), got %d", got)
	}
}

func TestSessionAuthenticator_LoginFailure(t *testing.T) {
	srv := newSessionServer()
	srv.statusCode = http.StatusUnauthorized
	srv.errBody = `{"code":"AUTH_FAILED","message":"invalid username or password","request_id":"req-1"}`
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/session", srv.handler())
	ts := httptest.NewServer(mux)
	defer ts.Close()

	auth := &SessionAuthenticator{
		BaseURL:    mustParseTestBaseURL(t, ts.URL),
		HTTPClient: ts.Client(),
		Username:   "alice",
		Password:   "wrong",
	}

	_, err := auth.Token(context.Background(), false)
	if err == nil {
		t.Fatal("expected an error")
	}
	apiErr, ok := AsAPIError(err)
	if !ok {
		t.Fatalf("expected an *APIError, got %T: %v", err, err)
	}
	if apiErr.Code != "AUTH_FAILED" {
		t.Fatalf("expected code AUTH_FAILED, got %q", apiErr.Code)
	}
	if !apiErr.IsUnauthorized() {
		t.Fatalf("expected IsUnauthorized() true, got status %d", apiErr.StatusCode)
	}
}

func TestSessionAuthenticator_ConcurrentCallsShareOneLogin(t *testing.T) {
	srv := newSessionServer()
	srv.nextExpires = time.Now().Add(1 * time.Hour).Format(time.RFC3339)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/session", srv.handler())
	ts := httptest.NewServer(mux)
	defer ts.Close()

	auth := &SessionAuthenticator{
		BaseURL:    mustParseTestBaseURL(t, ts.URL),
		HTTPClient: ts.Client(),
		Username:   "alice",
		Password:   "secret",
	}

	const n = 20
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := auth.Token(context.Background(), false); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("unexpected error from concurrent Token call: %v", err)
	}

	if got := srv.loginCount(); got != 1 {
		t.Fatalf("expected exactly 1 login call across %d concurrent callers, got %d", n, got)
	}
}
