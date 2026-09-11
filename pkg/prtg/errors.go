package prtg

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// APIError represents PRTG APIv2's structured error envelope:
//
//	{"code": "...", "message": "...", "request_id": "..."}
//
// returned on non-2xx responses. Callers should prefer Code/Message over
// generic "HTTP <status>" text wherever the error is surfaced to a user
// (CheckHealth, QueryData error responses, CallResource error bodies).
type APIError struct {
	// StatusCode is the HTTP status code of the response.
	StatusCode int
	// Code is PRTG's machine-readable error code, e.g. "AUTH_FAILED",
	// "LOGIN_FAILED", "FORBIDDEN", "NOT_FOUND", "INVALID_FILTER",
	// "SERVICE_UNAVAILABLE", "LICENSE_INACTIVE", "GATEWAY_TIMEOUT". May be
	// empty if the response body didn't parse as the expected envelope
	// (e.g. a plain-text error from a reverse proxy in front of PRTG).
	Code string
	// Message is PRTG's human-readable error message, or the raw response
	// body if it didn't parse as the expected envelope.
	Message string
	// RequestID is PRTG's request correlation id, if provided.
	RequestID string
}

// Error implements the error interface.
func (e *APIError) Error() string {
	switch {
	case e.Code != "" && e.Message != "":
		return fmt.Sprintf("prtg api error: %s: %s (http %d)", e.Code, e.Message, e.StatusCode)
	case e.Code != "":
		return fmt.Sprintf("prtg api error: %s (http %d)", e.Code, e.StatusCode)
	case e.Message != "":
		return fmt.Sprintf("prtg api error: http %d: %s", e.StatusCode, e.Message)
	default:
		return fmt.Sprintf("prtg api error: http %d", e.StatusCode)
	}
}

// IsUnauthorized reports whether the error is a 401 (e.g. PRTG's AUTH_FAILED
// / LOGIN_FAILED codes).
func (e *APIError) IsUnauthorized() bool { return e.StatusCode == http.StatusUnauthorized }

// IsForbidden reports whether the error is a 403 (PRTG's FORBIDDEN code).
func (e *APIError) IsForbidden() bool { return e.StatusCode == http.StatusForbidden }

// IsNotFound reports whether the error is a 404 (PRTG's NOT_FOUND code).
func (e *APIError) IsNotFound() bool { return e.StatusCode == http.StatusNotFound }

// IsServiceUnavailable reports whether the error is a 503 (PRTG's
// SERVICE_UNAVAILABLE / LICENSE_INACTIVE codes).
func (e *APIError) IsServiceUnavailable() bool { return e.StatusCode == http.StatusServiceUnavailable }

// IsTimeout reports whether the error is a 504 (PRTG's GATEWAY_TIMEOUT code).
func (e *APIError) IsTimeout() bool { return e.StatusCode == http.StatusGatewayTimeout }

// IsBadRequest reports whether the error is a 400 (e.g. PRTG's
// INVALID_FILTER code).
func (e *APIError) IsBadRequest() bool { return e.StatusCode == http.StatusBadRequest }

// AsAPIError extracts an *APIError from err if err is, or wraps, one.
func AsAPIError(err error) (*APIError, bool) {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr, true
	}
	return nil, false
}

// parseAPIError decodes a non-2xx PRTG APIv2 response body into an APIError.
// If the body doesn't parse as the {code,message,request_id} envelope, the
// raw body text (trimmed) is used as the Message so the caller still has
// something useful to show.
func parseAPIError(statusCode int, body []byte) *APIError {
	apiErr := &APIError{StatusCode: statusCode}

	var envelope struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
	}
	if len(body) > 0 && json.Unmarshal(body, &envelope) == nil {
		apiErr.Code = envelope.Code
		apiErr.Message = envelope.Message
		apiErr.RequestID = envelope.RequestID
	}

	if apiErr.Message == "" {
		apiErr.Message = strings.TrimSpace(string(body))
	}
	if apiErr.Message == "" {
		apiErr.Message = http.StatusText(statusCode)
	}

	return apiErr
}
