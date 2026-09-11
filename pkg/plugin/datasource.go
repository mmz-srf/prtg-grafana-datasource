package plugin

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/httpclient"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/backend/resource/httpadapter"
	"github.com/srgssr/prtg-datasource/pkg/models"
	"github.com/srgssr/prtg-datasource/pkg/prtg"
)

// Make sure Datasource implements required interfaces. This is important to
// do since otherwise we will only get a not implemented error response from
// the plugin at runtime.
var (
	_ backend.QueryDataHandler      = (*Datasource)(nil)
	_ backend.CheckHealthHandler    = (*Datasource)(nil)
	_ backend.CallResourceHandler   = (*Datasource)(nil)
	_ instancemgmt.InstanceDisposer = (*Datasource)(nil)
)

// requestTimeout bounds every HTTP request this datasource makes to PRTG.
// It's kept comfortably under PRTG APIv2's documented 25s server-side
// execution timeout so we see our own timeout (a clearer error) rather than
// PRTG's GATEWAY_TIMEOUT in the common case.
const requestTimeout = 20 * time.Second

// Datasource implements the PRTG APIv2 Grafana datasource: QueryData,
// CheckHealth and CallResource, backed by a pkg/prtg.Client.
type Datasource struct {
	client          *prtg.Client
	resourceHandler backend.CallResourceHandler
}

// NewDatasource creates a new datasource instance: it loads and validates
// the configured settings, builds an HTTP client (via the SDK's
// backend/httpclient, honoring TLS-skip-verify and a bounded timeout) and an
// Authenticator matching the configured auth mode, and wires up the
// CallResource route table.
func NewDatasource(_ context.Context, settings backend.DataSourceInstanceSettings) (instancemgmt.Instance, error) {
	pluginSettings, err := models.LoadPluginSettings(settings)
	if err != nil {
		return nil, err
	}
	if err := pluginSettings.Validate(); err != nil {
		return nil, err
	}

	baseURL, err := prtg.ParseServerURL(pluginSettings.ServerURL)
	if err != nil {
		return nil, err
	}

	httpClient, err := httpclient.New(httpclient.Options{
		Timeouts: &httpclient.TimeoutOptions{
			Timeout:               requestTimeout,
			DialTimeout:           10 * time.Second,
			KeepAlive:             30 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   100,
			IdleConnTimeout:       90 * time.Second,
		},
		TLS: &httpclient.TLSOptions{
			InsecureSkipVerify: pluginSettings.TLSSkipVerify,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("building HTTP client: %w", err)
	}

	auth, err := newAuthenticator(pluginSettings, baseURL, httpClient)
	if err != nil {
		return nil, err
	}

	client, err := prtg.NewClient(baseURL, httpClient, auth)
	if err != nil {
		return nil, err
	}

	ds := &Datasource{client: client}
	ds.resourceHandler = httpadapter.New(ds.registerRoutes())
	return ds, nil
}

func newAuthenticator(settings *models.PluginSettings, baseURL *url.URL, httpClient *http.Client) (prtg.Authenticator, error) {
	switch settings.AuthMode {
	case models.AuthModeAPIKey:
		return &prtg.APIKeyAuthenticator{APIKey: settings.Secrets.ApiKey}, nil
	case models.AuthModeCredentials:
		return &prtg.SessionAuthenticator{
			BaseURL:    baseURL,
			HTTPClient: httpClient,
			Username:   settings.Username,
			Password:   settings.Secrets.Password,
		}, nil
	default:
		return nil, fmt.Errorf("unknown authentication mode %q", settings.AuthMode)
	}
}

// Dispose tells the plugin SDK that this instance wants to clean up
// resources when a new instance is created (e.g. on settings change). The
// datasource holds no resources that need explicit cleanup (the HTTP client
// closes its idle connections on GC), so this is a no-op.
func (d *Datasource) Dispose() {}

// CallResource routes resource calls (see resources.go) through an
// http.ServeMux via the SDK's resource/httpadapter.
func (d *Datasource) CallResource(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
	return d.resourceHandler.CallResource(ctx, req, sender)
}
