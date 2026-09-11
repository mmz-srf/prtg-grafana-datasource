package prtg

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Authenticator resolves the bearer token to send with every PRTG APIv2
// request.
type Authenticator interface {
	// Token returns a valid bearer token, performing a login/refresh if
	// necessary. When force is true, any cached token must be ignored and a
	// fresh one obtained -- this is used both for proactive near-expiry
	// refresh and as the retry-once-on-401 safety net in Client.
	Token(ctx context.Context, force bool) (string, error)
}

// APIKeyAuthenticator is the trivial Authenticator for the static API Key
// auth mode: PRTG API keys don't expire, so the same key is returned
// forever, unconditionally of force.
type APIKeyAuthenticator struct {
	APIKey string
}

// Token implements Authenticator.
func (a *APIKeyAuthenticator) Token(_ context.Context, _ bool) (string, error) {
	if a.APIKey == "" {
		return "", errors.New("prtg: API key is empty")
	}
	return a.APIKey, nil
}

// defaultRefreshSkew is how long before a cached session token's expires_at
// it is proactively treated as stale and refreshed, rather than waiting to
// react to a 401.
const defaultRefreshSkew = 30 * time.Second

// SessionAuthenticator implements PRTG APIv2's username+password session
// login (POST /session) for the 'credentials' auth mode. It caches the
// returned token and its expires_at and proactively re-logs-in once the
// cached token is close to (or past) expiry; Client additionally retries
// once on a 401 as a safety net, forcing a fresh login regardless of the
// cached expiry.
//
// All fields except the mutex-guarded cache are read-only after
// construction, so a *SessionAuthenticator is safe for concurrent use.
type SessionAuthenticator struct {
	// BaseURL is the PRTG APIv2 base URL (i.e. "https://<server>/api/v2"),
	// matching the base URL the associated Client was built with.
	BaseURL *url.URL
	// HTTPClient performs the login HTTP request.
	HTTPClient *http.Client
	Username   string
	Password   string

	// RefreshSkew overrides defaultRefreshSkew if non-zero.
	RefreshSkew time.Duration

	// Now overrides time.Now, for tests.
	Now func() time.Time

	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

// Token implements Authenticator. Concurrent callers serialize on the
// internal mutex, so at most one login/refresh request is in flight at a
// time and every caller observes a consistent cache.
func (a *SessionAuthenticator) Token(ctx context.Context, force bool) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := a.now()
	if !force && a.token != "" && now.Before(a.expiresAt.Add(-a.refreshSkew())) {
		return a.token, nil
	}

	token, expiresAt, err := a.login(ctx)
	if err != nil {
		return "", err
	}
	a.token = token
	a.expiresAt = expiresAt
	return a.token, nil
}

func (a *SessionAuthenticator) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}

func (a *SessionAuthenticator) refreshSkew() time.Duration {
	if a.RefreshSkew > 0 {
		return a.RefreshSkew
	}
	return defaultRefreshSkew
}

// loginResult mirrors PRTG APIv2's LoginResult schema (POST /session),
// trimmed to the fields this package needs.
type loginResult struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
}

func (a *SessionAuthenticator) login(ctx context.Context) (string, time.Time, error) {
	if a.BaseURL == nil {
		return "", time.Time{}, errors.New("prtg: SessionAuthenticator.BaseURL is required")
	}
	if a.HTTPClient == nil {
		return "", time.Time{}, errors.New("prtg: SessionAuthenticator.HTTPClient is required")
	}

	body, err := json.Marshal(map[string]string{
		"username": a.Username,
		"password": a.Password,
	})
	if err != nil {
		return "", time.Time{}, fmt.Errorf("prtg: encoding session login request: %w", err)
	}

	u := *a.BaseURL
	u.Path = strings.TrimRight(u.Path, "/") + "/session"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("prtg: building session login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := a.HTTPClient.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("prtg: session login request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("prtg: reading session login response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", time.Time{}, parseAPIError(resp.StatusCode, respBody)
	}

	var result loginResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", time.Time{}, fmt.Errorf("prtg: decoding session login response: %w", err)
	}
	if result.Token == "" {
		return "", time.Time{}, errors.New("prtg: session login response had no token")
	}

	expiresAt, err := time.Parse(time.RFC3339, result.ExpiresAt)
	if err != nil {
		// Be lenient: we have a perfectly usable token even if expires_at
		// didn't parse as ISO-8601. Treat it as already expired so the next
		// call re-logs-in rather than caching it indefinitely.
		expiresAt = a.now()
	}
	return result.Token, expiresAt, nil
}
