// Package prtg is a pure PRTG APIv2 (https://<server>/api/v2/...) client. It
// has no dependency on grafana-plugin-sdk-go so it can be developed and
// unit-tested standalone; pkg/plugin wires it into the Grafana datasource
// contract (backend.QueryDataHandler, backend.CheckHealthHandler,
// backend.CallResourceHandler).
package prtg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// ParseServerURL normalizes a configured PRTG server URL (e.g.
// "https://prtg.example.com" or "https://prtg.example.com/") into the base
// URL used for every APIv2 call, i.e. "https://prtg.example.com/api/v2".
func ParseServerURL(serverURL string) (*url.URL, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(serverURL), "/")
	if trimmed == "" {
		return nil, errors.New("prtg: server URL is required")
	}

	u, err := url.Parse(trimmed + "/api/v2")
	if err != nil {
		return nil, fmt.Errorf("prtg: invalid server URL %q: %w", serverURL, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("prtg: server URL %q must be absolute (include scheme and host)", serverURL)
	}
	return u, nil
}

// Client is a PRTG APIv2 HTTP client. It injects the Authorization: Bearer
// header on every request (via Authenticator) and retries exactly once,
// forcing a fresh token, if a request comes back 401 -- a safety net on top
// of the Authenticator's own proactive refresh.
type Client struct {
	baseURL    *url.URL
	httpClient *http.Client
	auth       Authenticator
}

// NewClient builds a Client for the given base URL (see ParseServerURL),
// HTTP client (see backend/httpclient in pkg/plugin) and Authenticator (see
// APIKeyAuthenticator / SessionAuthenticator).
func NewClient(baseURL *url.URL, httpClient *http.Client, auth Authenticator) (*Client, error) {
	if baseURL == nil {
		return nil, errors.New("prtg: baseURL is required")
	}
	if httpClient == nil {
		return nil, errors.New("prtg: httpClient is required")
	}
	if auth == nil {
		return nil, errors.New("prtg: auth is required")
	}
	return &Client{baseURL: baseURL, httpClient: httpClient, auth: auth}, nil
}

// BaseURL returns the client's PRTG APIv2 base URL.
func (c *Client) BaseURL() *url.URL {
	return c.baseURL
}

// do executes an authenticated GET/POST/etc. against a path relative to the
// APIv2 base URL (e.g. "/experimental/sensors"), decoding a successful JSON
// response body into out (skipped when out is nil or the body is empty). It
// returns the response headers, which callers use to read pagination
// metadata (X-Total-Count, X-Result-Count).
//
// On a non-2xx response, do returns a non-nil *APIError (via the returned
// error, which satisfies errors.As(&*APIError)).
func (c *Client) do(ctx context.Context, method, path string, query url.Values, out interface{}) (http.Header, error) {
	resp, body, err := c.doOnce(ctx, method, path, query, false)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusUnauthorized {
		// Safety net: proactive refresh in the Authenticator should
		// normally prevent this, but retry exactly once with a forced
		// re-authentication in case the server-side session was
		// invalidated early or the API key rotated out from under us.
		resp, body, err = c.doOnce(ctx, method, path, query, true)
		if err != nil {
			return nil, err
		}
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.Header, parseAPIError(resp.StatusCode, body)
	}

	if out != nil && len(body) > 0 {
		if err := json.Unmarshal(body, out); err != nil {
			return resp.Header, fmt.Errorf("prtg: decoding response from %s: %w", path, err)
		}
	}
	return resp.Header, nil
}

func (c *Client) doOnce(ctx context.Context, method, path string, query url.Values, forceAuthRefresh bool) (*http.Response, []byte, error) {
	token, err := c.auth.Token(ctx, forceAuthRefresh)
	if err != nil {
		return nil, nil, fmt.Errorf("prtg: authenticating: %w", err)
	}

	reqURL := *c.baseURL
	reqURL.Path = strings.TrimRight(c.baseURL.Path, "/") + path
	if len(query) > 0 {
		reqURL.RawQuery = query.Encode()
	} else {
		reqURL.RawQuery = ""
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL.String(), nil)
	if err != nil {
		return nil, nil, fmt.Errorf("prtg: building request for %s: %w", path, err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("prtg: request to %s failed: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("prtg: reading response from %s: %w", path, err)
	}
	return resp, body, nil
}
