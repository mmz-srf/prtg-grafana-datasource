package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

// Authentication modes, per the unified frontend<->backend contract
// (jsonData.authMode).
const (
	AuthModeAPIKey      = "apiKey"
	AuthModeCredentials = "credentials"
)

// PluginSettings is the datasource's configuration, decoded from
// jsonData/secureJsonData per the unified frontend<->backend contract:
//
//	// jsonData (non-secret)
//	{ serverUrl: string; authMode: 'apiKey' | 'credentials'; username?: string; tlsSkipVerify?: boolean }
//	// secureJsonData
//	{ apiKey?: string; password?: string }
type PluginSettings struct {
	ServerURL     string                `json:"serverUrl"`
	AuthMode      string                `json:"authMode"`
	Username      string                `json:"username,omitempty"`
	TLSSkipVerify bool                  `json:"tlsSkipVerify,omitempty"`
	Secrets       *SecretPluginSettings `json:"-"`
}

// SecretPluginSettings holds the datasource's secure/secret configuration.
type SecretPluginSettings struct {
	ApiKey   string `json:"apiKey"`
	Password string `json:"password"`
}

// LoadPluginSettings decodes a datasource instance's settings.
func LoadPluginSettings(source backend.DataSourceInstanceSettings) (*PluginSettings, error) {
	settings := PluginSettings{}
	if len(source.JSONData) > 0 {
		if err := json.Unmarshal(source.JSONData, &settings); err != nil {
			return nil, fmt.Errorf("could not unmarshal PluginSettings json: %w", err)
		}
	}

	settings.Secrets = loadSecretPluginSettings(source.DecryptedSecureJSONData)

	return &settings, nil
}

func loadSecretPluginSettings(source map[string]string) *SecretPluginSettings {
	return &SecretPluginSettings{
		ApiKey:   source["apiKey"],
		Password: source["password"],
	}
}

// Validate checks that settings are internally consistent and sufficient to
// build a prtg.Client: a server URL is always required, plus either an API
// key (authMode == "apiKey") or a username+password pair
// (authMode == "credentials").
func (s *PluginSettings) Validate() error {
	if strings.TrimSpace(s.ServerURL) == "" {
		return errors.New("PRTG server URL is required")
	}

	switch s.AuthMode {
	case AuthModeAPIKey:
		if s.Secrets == nil || s.Secrets.ApiKey == "" {
			return errors.New("API key is required when authentication mode is 'apiKey'")
		}
	case AuthModeCredentials:
		if strings.TrimSpace(s.Username) == "" {
			return errors.New("username is required when authentication mode is 'credentials'")
		}
		if s.Secrets == nil || s.Secrets.Password == "" {
			return errors.New("password is required when authentication mode is 'credentials'")
		}
	default:
		return fmt.Errorf("unknown authentication mode %q", s.AuthMode)
	}

	return nil
}
