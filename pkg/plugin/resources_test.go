package plugin

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/srgssr/prtg-datasource/pkg/prtg"
)

func callResource(t *testing.T, ds *Datasource, path, query string) *backend.CallResourceResponse {
	t.Helper()
	req := &backend.CallResourceRequest{
		Method: http.MethodGet,
		Path:   path,
	}
	if query != "" {
		req.URL = "?" + query
	}

	var resp *backend.CallResourceResponse
	err := ds.CallResource(context.Background(), req, backend.CallResourceResponseSenderFunc(func(r *backend.CallResourceResponse) error {
		resp = r
		return nil
	}))
	if err != nil {
		t.Fatalf("CallResource error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected a response, got nil")
	}
	return resp
}

func decodeBody(t *testing.T, resp *backend.CallResourceResponse, out interface{}) {
	t.Helper()
	if err := json.Unmarshal(resp.Body, out); err != nil {
		t.Fatalf("decoding response body %q: %v", string(resp.Body), err)
	}
}

func TestHandleGroups_CombinesGroupsAndProbes(t *testing.T) {
	s := newFakePRTGServer()
	s.Groups = []prtg.GroupInfo{
		{ID: "g1", Name: "Network", Path: []prtg.ReferencedObject{{ID: "0", Name: "Root", Type: "group"}, {ID: "g1", Name: "Network", Type: "group"}}},
	}
	s.Probes = []prtg.GroupInfo{
		{ID: "p1", Name: "Local Probe", Path: []prtg.ReferencedObject{{ID: "p1", Name: "Local Probe", Type: "probe"}}},
	}
	ds := newTestDatasource(t, s)

	resp := callResource(t, ds, "groups", "")
	if resp.Status != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", resp.Status, string(resp.Body))
	}

	var groups []groupDTO
	decodeBody(t, resp, &groups)
	if len(groups) != 2 {
		t.Fatalf("expected 2 combined groups+probes, got %d: %+v", len(groups), groups)
	}

	byID := map[string]groupDTO{}
	for _, g := range groups {
		byID[g.ID] = g
	}
	if byID["g1"].Name != "Network" || byID["g1"].Path != "Root / Network" {
		t.Fatalf("unexpected group DTO: %+v", byID["g1"])
	}
	if byID["p1"].Name != "Local Probe" {
		t.Fatalf("unexpected probe DTO: %+v", byID["p1"])
	}
}

func TestHandleGroups_UpstreamError(t *testing.T) {
	s := newFakePRTGServer()
	s.GroupsStatus = http.StatusServiceUnavailable
	s.GroupsErrorBody = `{"code":"SERVICE_UNAVAILABLE","message":"core down"}`
	ds := newTestDatasource(t, s)

	resp := callResource(t, ds, "groups", "")
	if resp.Status != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", resp.Status)
	}
	var body map[string]string
	decodeBody(t, resp, &body)
	if body["error"] == "" {
		t.Fatalf("expected a non-empty error message, got %+v", body)
	}
}

func TestHandleDevices(t *testing.T) {
	s := newFakePRTGServer()
	s.Devices = []fakeDevice{
		{GroupID: "g1", Info: prtg.DeviceInfo{ID: "d1", Name: "Router 1"}},
		{GroupID: "g1", Info: prtg.DeviceInfo{ID: "d2", Name: "Router 2"}},
		{GroupID: "g2", Info: prtg.DeviceInfo{ID: "d3", Name: "Switch 1"}},
	}
	ds := newTestDatasource(t, s)

	t.Run("missing groupId", func(t *testing.T) {
		resp := callResource(t, ds, "devices", "")
		if resp.Status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", resp.Status)
		}
	})

	t.Run("filters by groupId", func(t *testing.T) {
		resp := callResource(t, ds, "devices", "groupId=g1")
		if resp.Status != http.StatusOK {
			t.Fatalf("expected 200, got %d (%s)", resp.Status, string(resp.Body))
		}
		var devices []idNameDTO
		decodeBody(t, resp, &devices)
		if len(devices) != 2 {
			t.Fatalf("expected 2 devices for g1, got %d: %+v", len(devices), devices)
		}
	})
}

func TestHandleSensors(t *testing.T) {
	s := newFakePRTGServer()
	s.Sensors = []fakeSensor{
		{DeviceID: "d1", Info: prtg.SensorInfo{ID: "s1", Name: "CPU Load"}},
		{DeviceID: "d1", Info: prtg.SensorInfo{ID: "s2", Name: "Memory"}},
		{DeviceID: "d2", Info: prtg.SensorInfo{ID: "s3", Name: "Ping"}},
	}
	ds := newTestDatasource(t, s)

	t.Run("missing deviceId", func(t *testing.T) {
		resp := callResource(t, ds, "sensors", "")
		if resp.Status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", resp.Status)
		}
	})

	t.Run("filters by deviceId", func(t *testing.T) {
		resp := callResource(t, ds, "sensors", "deviceId=d1")
		var sensors []idNameDTO
		decodeBody(t, resp, &sensors)
		if len(sensors) != 2 {
			t.Fatalf("expected 2 sensors for d1, got %d: %+v", len(sensors), sensors)
		}
	})
}

func TestHandleChannels(t *testing.T) {
	s := newFakePRTGServer()
	s.ChannelsBySensor["s1"] = []prtg.ChannelInfo{
		{ID: "s1.0", Name: "Total", Unit: prtg.Unit{DisplayUnit: "%"}},
		{ID: "s1.1", Name: "Core 1", Unit: prtg.Unit{DisplayUnit: "%"}},
	}
	ds := newTestDatasource(t, s)

	t.Run("missing sensorId", func(t *testing.T) {
		resp := callResource(t, ds, "channels", "")
		if resp.Status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", resp.Status)
		}
	})

	t.Run("returns channels with unit", func(t *testing.T) {
		resp := callResource(t, ds, "channels", "sensorId=s1")
		var channels []channelDTO
		decodeBody(t, resp, &channels)
		if len(channels) != 2 {
			t.Fatalf("expected 2 channels, got %d: %+v", len(channels), channels)
		}
		if channels[0].Unit != "%" {
			t.Fatalf("expected unit '%%', got %+v", channels[0])
		}
	})
}

func TestHandleSensorsSearch(t *testing.T) {
	s := newFakePRTGServer()
	s.Sensors = []fakeSensor{
		{DeviceID: "d1", GroupID: "g1", Info: prtg.SensorInfo{
			ID: "s1", Name: "CPU Load",
			Path: []prtg.ReferencedObject{{ID: "d1", Name: "Device 1", Type: "device"}},
		}},
		{DeviceID: "d1", GroupID: "g1", Info: prtg.SensorInfo{
			ID: "s2", Name: "Memory",
			Path: []prtg.ReferencedObject{{ID: "d1", Name: "Device 1", Type: "device"}},
		}},
	}
	s.ChannelsBySensor["s1"] = []prtg.ChannelInfo{{ID: "s1.0", Name: "Total"}}
	ds := newTestDatasource(t, s)

	t.Run("missing pattern", func(t *testing.T) {
		resp := callResource(t, ds, "sensors/search", "")
		if resp.Status != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", resp.Status)
		}
	})

	t.Run("invalid pattern", func(t *testing.T) {
		resp := callResource(t, ds, "sensors/search", "pattern=(")
		if resp.Status != http.StatusBadRequest {
			t.Fatalf("expected 400 for invalid regex, got %d", resp.Status)
		}
	})

	t.Run("matches sensors and channels, exact contract shape", func(t *testing.T) {
		resp := callResource(t, ds, "sensors/search", "pattern=CPU&channelMatchMode=exact&channelPattern=Total")
		if resp.Status != http.StatusOK {
			t.Fatalf("expected 200, got %d (%s)", resp.Status, string(resp.Body))
		}

		var result sensorSearchResponseDTO
		decodeBody(t, resp, &result)
		if result.TotalSensors != 1 || result.TotalChannels != 1 {
			t.Fatalf("unexpected totals: %+v", result)
		}
		if result.Truncated {
			t.Fatal("expected Truncated=false")
		}
		if len(result.Sensors) != 1 {
			t.Fatalf("expected 1 sensor, got %d", len(result.Sensors))
		}
		got := result.Sensors[0]
		if got.ID != "s1" || got.Name != "CPU Load" || got.DeviceID != "d1" || got.DeviceName != "Device 1" {
			t.Fatalf("unexpected sensor match DTO: %+v", got)
		}
		if len(got.MatchedChannels) != 1 || got.MatchedChannels[0].ID != "s1.0" || got.MatchedChannels[0].Name != "Total" {
			t.Fatalf("unexpected matched channels: %+v", got.MatchedChannels)
		}

		// Verify the raw JSON keys match the unified contract exactly.
		var raw map[string]interface{}
		decodeBody(t, resp, &raw)
		for _, key := range []string{"sensors", "totalSensors", "totalChannels", "truncated"} {
			if _, ok := raw[key]; !ok {
				t.Fatalf("expected top-level key %q in response, got %v", key, raw)
			}
		}
		sensorsRaw := raw["sensors"].([]interface{})
		firstSensor := sensorsRaw[0].(map[string]interface{})
		for _, key := range []string{"id", "name", "deviceId", "deviceName", "matchedChannels"} {
			if _, ok := firstSensor[key]; !ok {
				t.Fatalf("expected sensor key %q, got %v", key, firstSensor)
			}
		}
	})
}
