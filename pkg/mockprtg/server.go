// Package mockprtg implements a small, standalone, in-memory mock of PRTG
// APIv2's "Experimental" endpoints (groups/probes/devices/sensors/channels,
// timeseries, session login) -- realistic enough for a Grafana instance to
// browse and query as if it were a real PRTG server. It exists because there
// is no official PRTG container (Paessler's PRTG is Windows-only, closed
// source and license-bound), so local development/manual testing of this
// plugin needs a substitute to point Grafana at.
//
// This package has no dependency on grafana-plugin-sdk-go, matching
// pkg/prtg's own house style, and imports pkg/prtg only for its exported
// types (GroupInfo, DeviceInfo, SensorInfo, ChannelInfo, ...) so its JSON
// output is guaranteed to match what pkg/prtg's Client expects to decode.
//
// It is deliberately separate from pkg/plugin/fakeserver_test.go, this
// repo's other, test-only PRTG double: that one lives in a _test.go file and
// so cannot be imported by a standalone binary. Unifying the two is a
// plausible future cleanup, not attempted here.
package mockprtg

import (
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Options configures a Server's credentials. Any left empty fall back to a
// fixed default so the package is usable with zero configuration.
type Options struct {
	// APIKey is the bearer token accepted for the 'apiKey' auth mode.
	// Defaults to "mock-api-key".
	APIKey string
	// Username/Password are the credentials accepted by POST /session for
	// the 'credentials' auth mode. Default to "mock-user"/"mock-password".
	Username string
	Password string
}

// Server is a mock PRTG APIv2 server: an HTTP handler backed by a Dataset,
// with a small in-memory session-token store and a bearer-auth check in
// front of every /experimental/* route.
type Server struct {
	Dataset  *Dataset
	APIKey   string
	Username string
	Password string

	// Now overrides time.Now, for deterministic tests.
	Now func() time.Time

	mu       sync.Mutex
	sessions map[string]time.Time // token -> expiry
	tokenSeq uint64
}

// NewServer builds a Server over ds, applying opts' defaults.
func NewServer(ds *Dataset, opts Options) *Server {
	if opts.APIKey == "" {
		opts.APIKey = "mock-api-key"
	}
	if opts.Username == "" {
		opts.Username = "mock-user"
	}
	if opts.Password == "" {
		opts.Password = "mock-password"
	}
	return &Server{
		Dataset:  ds,
		APIKey:   opts.APIKey,
		Username: opts.Username,
		Password: opts.Password,
		sessions: map[string]time.Time{},
	}
}

func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now().UTC()
}

// Handler builds the mux implementing every route this mock serves,
// wrapped with request logging. /healthz and POST /session are exempt from
// auth (you can't present a session token before obtaining one); every
// /api/v2/experimental/* route requires one.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("POST /api/v2/session", s.handleSession)
	mux.HandleFunc("GET /api/v2/experimental/groups", s.requireAuth(s.handleGroups))
	mux.HandleFunc("GET /api/v2/experimental/probes", s.requireAuth(s.handleProbes))
	mux.HandleFunc("GET /api/v2/experimental/devices", s.requireAuth(s.handleDevices))
	mux.HandleFunc("GET /api/v2/experimental/sensors", s.requireAuth(s.handleSensors))
	mux.HandleFunc("GET /api/v2/experimental/channels", s.requireAuth(s.handleChannels))
	mux.HandleFunc("GET /api/v2/experimental/timeseries/{sensorId}/{window}", s.requireAuth(s.handleTimeseries))
	return withLogging(mux)
}

// requireAuth wraps next with a check that the request carries a valid
// bearer token -- either the configured static API key, or a live session
// token issued by POST /session -- returning PRTG's real AUTH_FAILED
// envelope otherwise.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || !s.tokenValid(token) {
			writeAPIError(w, http.StatusUnauthorized, "AUTH_FAILED", "invalid or missing API token")
			return
		}
		next(w, r)
	}
}

func (s *Server) tokenValid(token string) bool {
	if token == "" {
		return false
	}
	if s.APIKey != "" && token == s.APIKey {
		return true
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	expiresAt, ok := s.sessions[token]
	if !ok {
		return false
	}
	if s.now().After(expiresAt) {
		delete(s.sessions, token)
		return false
	}
	return true
}

// statusRecorder captures the status code an http.Handler wrote, for
// logging.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.RequestURI(), rec.status, time.Since(start))
	})
}
