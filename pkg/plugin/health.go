package plugin

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/srgssr/prtg-datasource/pkg/prtg"
)

// checkHealthTimeout bounds the health-check probe request.
const checkHealthTimeout = 10 * time.Second

// CheckHealth performs a small, real, authenticated call against PRTG
// (a single-item page of /experimental/groups) and maps the outcome to a
// 3-way message so the "Save & Test" button in the ConfigEditor actually
// distinguishes:
//   - an unreachable server (network/DNS/TLS failure, no HTTP response at all)
//   - authentication failure (401/403 -- wrong API key, or bad username/password)
//   - PRTG APIv2 not activated on the target server (404 -- the "Application
//     Server"/new UI+API prerequisite from plan §1 isn't met)
//
// Any other upstream error (503/504/etc.) is still surfaced with PRTG's own
// code/message rather than a generic "HTTP nnn".
func (d *Datasource) CheckHealth(ctx context.Context, _ *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
	ctx, cancel := context.WithTimeout(ctx, checkHealthTimeout)
	defer cancel()

	_, err := prtg.FetchPage[prtg.GroupInfo](ctx, d.client, "/experimental/groups", "", "", 0, 1)
	if err == nil {
		return &backend.CheckHealthResult{
			Status:  backend.HealthStatusOk,
			Message: "Successfully connected to the PRTG API v2",
		}, nil
	}

	apiErr, ok := prtg.AsAPIError(err)
	if !ok {
		return &backend.CheckHealthResult{
			Status:  backend.HealthStatusError,
			Message: fmt.Sprintf("Unable to reach the PRTG server: %s", err.Error()),
		}, nil
	}

	switch {
	case apiErr.IsUnauthorized() || apiErr.IsForbidden():
		return &backend.CheckHealthResult{
			Status:  backend.HealthStatusError,
			Message: fmt.Sprintf("Authentication failed: %s", apiErr.Message),
		}, nil
	case apiErr.IsNotFound():
		return &backend.CheckHealthResult{
			Status: backend.HealthStatusError,
			Message: "PRTG API v2 endpoint not found. Make sure the PRTG \"Application Server\" and the new " +
				"UI/API are activated on this PRTG core server (Setup -> Activate New UI And New API).",
		}, nil
	case apiErr.StatusCode == http.StatusServiceUnavailable:
		return &backend.CheckHealthResult{
			Status:  backend.HealthStatusError,
			Message: fmt.Sprintf("PRTG server unavailable: %s", apiErr.Message),
		}, nil
	default:
		message := apiErr.Message
		if apiErr.Code != "" {
			message = fmt.Sprintf("%s: %s", apiErr.Code, apiErr.Message)
		}
		return &backend.CheckHealthResult{
			Status:  backend.HealthStatusError,
			Message: fmt.Sprintf("PRTG API error: %s", message),
		}, nil
	}
}
