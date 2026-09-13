package plugin

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
)

func newInstanceSettings(t *testing.T, jsonData map[string]interface{}, secrets map[string]string) backend.DataSourceInstanceSettings {
	t.Helper()
	raw, err := json.Marshal(jsonData)
	if err != nil {
		t.Fatalf("marshal jsonData: %v", err)
	}
	return backend.DataSourceInstanceSettings{
		JSONData:                raw,
		DecryptedSecureJSONData: secrets,
	}
}

func TestNewDatasource_ValidAPIKey(t *testing.T) {
	settings := newInstanceSettings(t, map[string]interface{}{
		"serverUrl": "https://prtg.example.com",
		"authMode":  "apiKey",
	}, map[string]string{"apiKey": "some-key"})

	inst, err := NewDatasource(context.Background(), settings)
	if err != nil {
		t.Fatalf("NewDatasource returned error: %v", err)
	}
	ds, ok := inst.(*Datasource)
	if !ok {
		t.Fatalf("expected *Datasource, got %T", inst)
	}
	if ds.client == nil {
		t.Fatal("expected client to be initialized")
	}
	if ds.resourceHandler == nil {
		t.Fatal("expected resourceHandler to be initialized")
	}
}

func TestNewDatasource_ValidCredentials(t *testing.T) {
	settings := newInstanceSettings(t, map[string]interface{}{
		"serverUrl": "https://prtg.example.com",
		"authMode":  "credentials",
		"username":  "alice",
	}, map[string]string{"password": "s3cret"})

	inst, err := NewDatasource(context.Background(), settings)
	if err != nil {
		t.Fatalf("NewDatasource returned error: %v", err)
	}
	if _, ok := inst.(*Datasource); !ok {
		t.Fatalf("expected *Datasource, got %T", inst)
	}
}

func TestNewDatasource_Errors(t *testing.T) {
	tests := []struct {
		name      string
		jsonData  map[string]interface{}
		secrets   map[string]string
		wantError bool
	}{
		{
			name:      "missing server url",
			jsonData:  map[string]interface{}{"authMode": "apiKey"},
			secrets:   map[string]string{"apiKey": "x"},
			wantError: true,
		},
		{
			name:      "missing api key",
			jsonData:  map[string]interface{}{"serverUrl": "https://prtg.example.com", "authMode": "apiKey"},
			secrets:   map[string]string{},
			wantError: true,
		},
		{
			name:      "missing username",
			jsonData:  map[string]interface{}{"serverUrl": "https://prtg.example.com", "authMode": "credentials"},
			secrets:   map[string]string{"password": "x"},
			wantError: true,
		},
		{
			name:      "missing password",
			jsonData:  map[string]interface{}{"serverUrl": "https://prtg.example.com", "authMode": "credentials", "username": "alice"},
			secrets:   map[string]string{},
			wantError: true,
		},
		{
			name:      "unknown auth mode",
			jsonData:  map[string]interface{}{"serverUrl": "https://prtg.example.com", "authMode": "bogus"},
			secrets:   map[string]string{},
			wantError: true,
		},
		{
			name:      "server url without scheme/host",
			jsonData:  map[string]interface{}{"serverUrl": "not-a-url", "authMode": "apiKey"},
			secrets:   map[string]string{"apiKey": "x"},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := newInstanceSettings(t, tt.jsonData, tt.secrets)
			_, err := NewDatasource(context.Background(), settings)
			if tt.wantError && err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !tt.wantError && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}
