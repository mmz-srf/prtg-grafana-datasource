package plugin

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/srgssr/prtg-datasource/pkg/prtg"
)

func TestCheckHealth_ThreeWayMessages(t *testing.T) {
	tests := []struct {
		name            string
		groupsStatus    int
		groupsErrorBody string
		wantStatus      backend.HealthStatus
		wantMessageHas  string
	}{
		{
			name:       "success",
			wantStatus: backend.HealthStatusOk,
		},
		{
			name:            "unauthorized -> authentication failed",
			groupsStatus:    401,
			groupsErrorBody: `{"code":"AUTH_FAILED","message":"invalid API key"}`,
			wantStatus:      backend.HealthStatusError,
			wantMessageHas:  "Authentication failed",
		},
		{
			name:            "forbidden -> authentication failed",
			groupsStatus:    403,
			groupsErrorBody: `{"code":"FORBIDDEN","message":"insufficient permissions"}`,
			wantStatus:      backend.HealthStatusError,
			wantMessageHas:  "Authentication failed",
		},
		{
			name:            "not found -> apiv2 not activated hint",
			groupsStatus:    404,
			groupsErrorBody: `{"code":"NOT_FOUND","message":"no such route"}`,
			wantStatus:      backend.HealthStatusError,
			wantMessageHas:  "Application Server",
		},
		{
			name:            "service unavailable",
			groupsStatus:    503,
			groupsErrorBody: `{"code":"SERVICE_UNAVAILABLE","message":"core server unreachable"}`,
			wantStatus:      backend.HealthStatusError,
			wantMessageHas:  "PRTG server unavailable",
		},
		{
			name:            "other upstream error still surfaces code/message",
			groupsStatus:    504,
			groupsErrorBody: `{"code":"GATEWAY_TIMEOUT","message":"took too long"}`,
			wantStatus:      backend.HealthStatusError,
			wantMessageHas:  "GATEWAY_TIMEOUT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newFakePRTGServer()
			s.Groups = []prtg.GroupInfo{{ID: "1", Name: "Core"}}
			s.GroupsStatus = tt.groupsStatus
			s.GroupsErrorBody = tt.groupsErrorBody

			ds := newTestDatasource(t, s)
			result, err := ds.CheckHealth(context.Background(), &backend.CheckHealthRequest{})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Status != tt.wantStatus {
				t.Fatalf("expected status %v, got %v (%s)", tt.wantStatus, result.Status, result.Message)
			}
			if tt.wantMessageHas != "" && !strings.Contains(result.Message, tt.wantMessageHas) {
				t.Fatalf("expected message to contain %q, got %q", tt.wantMessageHas, result.Message)
			}
		})
	}
}

func TestCheckHealth_Unreachable(t *testing.T) {
	// A server that's already closed simulates "unreachable" -- a
	// network-level failure rather than an HTTP error response, so
	// CheckHealth must fall into its non-APIError branch.
	ts := httptest.NewServer(newFakePRTGServer().mux())
	ts.Close()

	base, err := prtg.ParseServerURL(ts.URL)
	if err != nil {
		t.Fatalf("ParseServerURL: %v", err)
	}
	client, err := prtg.NewClient(base, ts.Client(), &prtg.APIKeyAuthenticator{APIKey: "test-key"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	ds := &Datasource{client: client}

	result, err := ds.CheckHealth(context.Background(), &backend.CheckHealthRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != backend.HealthStatusError {
		t.Fatalf("expected HealthStatusError, got %v", result.Status)
	}
	if !strings.Contains(result.Message, "Unable to reach the PRTG server") {
		t.Fatalf("expected an 'unable to reach' message, got %q", result.Message)
	}
}
