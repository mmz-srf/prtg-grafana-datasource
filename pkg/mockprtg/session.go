package mockprtg

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// sessionTokenTTL is how long a token issued by POST /session stays valid.
// An hour is long enough not to be annoying during a dev session, but short
// enough that leaving Grafana open overnight genuinely exercises
// SessionAuthenticator's proactive-refresh/retry-on-401 path.
const sessionTokenTTL = time.Hour

// handleSession implements POST /api/v2/session, the login endpoint
// pkg/prtg's SessionAuthenticator uses for the 'credentials' auth mode (see
// pkg/prtg/auth.go's loginResult: {"token","expires_at"}).
func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeAPIError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}

	if body.Username != s.Username || body.Password != s.Password {
		writeAPIError(w, http.StatusUnauthorized, "LOGIN_FAILED", "invalid username or password")
		return
	}

	token, expiresAt := s.issueSessionToken()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"token":      token,
		"expires_at": expiresAt.UTC().Format(time.RFC3339),
	})
}

// issueSessionToken mints and records a new session token, valid for
// sessionTokenTTL from now.
func (s *Server) issueSessionToken() (string, time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.tokenSeq++
	token := fmt.Sprintf("mock-session-token-%d", s.tokenSeq)
	expiresAt := s.now().Add(sessionTokenTTL)
	s.sessions[token] = expiresAt
	return token, expiresAt
}
