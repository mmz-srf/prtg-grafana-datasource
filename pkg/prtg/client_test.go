package prtg

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
)

// fakeAuthenticator is a scripted Authenticator for client_test.go: each
// call to Token records whether force was requested and returns the next
// token in the sequence (or the last one, if the sequence is exhausted).
type fakeAuthenticator struct {
	mu     sync.Mutex
	tokens []string
	forces []bool
	err    error
}

func (f *fakeAuthenticator) Token(_ context.Context, force bool) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.forces = append(f.forces, force)
	if f.err != nil {
		return "", f.err
	}
	idx := len(f.forces) - 1
	if idx < len(f.tokens) {
		return f.tokens[idx], nil
	}
	if len(f.tokens) == 0 {
		return "", nil
	}
	return f.tokens[len(f.tokens)-1], nil
}

func (f *fakeAuthenticator) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.forces)
}

func TestParseServerURL(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "no trailing slash", in: "https://prtg.example.com", want: "https://prtg.example.com/api/v2"},
		{name: "trailing slash", in: "https://prtg.example.com/", want: "https://prtg.example.com/api/v2"},
		{name: "empty", in: "", wantErr: true},
		{name: "whitespace only", in: "   ", wantErr: true},
		{name: "no scheme/host", in: "not-a-url", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseServerURL(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tt.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.String() != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got.String())
			}
		})
	}
}

func TestNewClient_Validation(t *testing.T) {
	base, err := ParseServerURL("https://prtg.example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	auth := &fakeAuthenticator{tokens: []string{"tok"}}

	if _, err := NewClient(nil, http.DefaultClient, auth); err == nil {
		t.Fatal("expected error for nil baseURL")
	}
	if _, err := NewClient(base, nil, auth); err == nil {
		t.Fatal("expected error for nil httpClient")
	}
	if _, err := NewClient(base, http.DefaultClient, nil); err == nil {
		t.Fatal("expected error for nil auth")
	}
	if _, err := NewClient(base, http.DefaultClient, auth); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func newTestClient(t *testing.T, handler http.HandlerFunc, auth Authenticator) (*Client, *httptest.Server) {
	t.Helper()
	ts := httptest.NewServer(handler)
	base, err := ParseServerURL(ts.URL)
	if err != nil {
		t.Fatalf("ParseServerURL: %v", err)
	}
	c, err := NewClient(base, ts.Client(), auth)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c, ts
}

func TestClient_do_Success(t *testing.T) {
	var gotAuthHeader string
	var gotPath string
	var gotQuery string

	handler := func(w http.ResponseWriter, r *http.Request) {
		gotAuthHeader = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode([]map[string]string{{"id": "1", "name": "Group A"}})
	}

	auth := &fakeAuthenticator{tokens: []string{"my-token"}}
	c, ts := newTestClient(t, handler, auth)
	defer ts.Close()

	var out []map[string]string
	headers, err := c.do(context.Background(), http.MethodGet, "/experimental/groups", urlValues("offset", "0", "limit", "5"), &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if headers == nil {
		t.Fatal("expected non-nil headers")
	}
	if gotAuthHeader != "Bearer my-token" {
		t.Fatalf("expected 'Bearer my-token', got %q", gotAuthHeader)
	}
	if gotPath != "/api/v2/experimental/groups" {
		t.Fatalf("expected path '/api/v2/experimental/groups', got %q", gotPath)
	}
	if gotQuery != "limit=5&offset=0" {
		t.Fatalf("expected query 'limit=5&offset=0', got %q", gotQuery)
	}
	if len(out) != 1 || out[0]["name"] != "Group A" {
		t.Fatalf("unexpected decoded output: %+v", out)
	}
}

func TestClient_do_NonSuccessStatus(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":"NOT_FOUND","message":"sensor not found","request_id":"req-42"}`))
	}
	auth := &fakeAuthenticator{tokens: []string{"tok"}}
	c, ts := newTestClient(t, handler, auth)
	defer ts.Close()

	_, err := c.do(context.Background(), http.MethodGet, "/experimental/sensors", nil, nil)
	if err == nil {
		t.Fatal("expected an error")
	}
	apiErr, ok := AsAPIError(err)
	if !ok {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.Code != "NOT_FOUND" || apiErr.Message != "sensor not found" || apiErr.RequestID != "req-42" {
		t.Fatalf("unexpected APIError: %+v", apiErr)
	}
	if !apiErr.IsNotFound() {
		t.Fatal("expected IsNotFound() true")
	}
}

func TestClient_do_RetryOnceOn401_ThenSucceeds(t *testing.T) {
	var requestCount int
	var authHeaders []string

	handler := func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		authHeaders = append(authHeaders, r.Header.Get("Authorization"))
		if requestCount == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"code":"AUTH_FAILED","message":"token expired"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode([]string{"ok"})
	}

	auth := &fakeAuthenticator{tokens: []string{"stale-token", "fresh-token"}}
	c, ts := newTestClient(t, handler, auth)
	defer ts.Close()

	var out []string
	_, err := c.do(context.Background(), http.MethodGet, "/experimental/groups", nil, &out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if requestCount != 2 {
		t.Fatalf("expected exactly 2 requests (1 retry), got %d", requestCount)
	}
	if authHeaders[0] != "Bearer stale-token" || authHeaders[1] != "Bearer fresh-token" {
		t.Fatalf("unexpected auth header sequence: %v", authHeaders)
	}
	if auth.callCount() != 2 {
		t.Fatalf("expected Authenticator.Token called twice, got %d", auth.callCount())
	}
	if len(auth.forces) != 2 || auth.forces[0] != false || auth.forces[1] != true {
		t.Fatalf("expected forces=[false,true] (normal, then forced retry), got %v", auth.forces)
	}
}

func TestClient_do_RetryOnceOn401_StillFails(t *testing.T) {
	var requestCount int
	handler := func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":"AUTH_FAILED","message":"invalid credentials"}`))
	}
	auth := &fakeAuthenticator{tokens: []string{"tok1", "tok2"}}
	c, ts := newTestClient(t, handler, auth)
	defer ts.Close()

	_, err := c.do(context.Background(), http.MethodGet, "/experimental/groups", nil, nil)
	if err == nil {
		t.Fatal("expected an error")
	}
	apiErr, ok := AsAPIError(err)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if !apiErr.IsUnauthorized() {
		t.Fatal("expected IsUnauthorized() true")
	}
	if requestCount != 2 {
		t.Fatalf("expected exactly 2 requests (no infinite retry loop), got %d", requestCount)
	}
}

// urlValues is a small helper to build url.Values from alternating
// key/value strings, to keep call sites in this file terse.
func urlValues(kv ...string) url.Values {
	m := url.Values{}
	for i := 0; i+1 < len(kv); i += 2 {
		m.Set(kv[i], kv[i+1])
	}
	return m
}
