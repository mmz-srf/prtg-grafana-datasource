package prtg

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// searchTestServer is a small fake PRTG server backing
// /experimental/sensors and /experimental/channels for search_test.go. It
// records every filter it was asked for.
type searchTestServer struct {
	sensors          []SensorInfo
	channelsBySensor map[string][]ChannelInfo
	sensorFilters    []string
	channelFilters   []string
}

func (s *searchTestServer) mux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/experimental/sensors", func(w http.ResponseWriter, r *http.Request) {
		filter := r.URL.Query().Get("filter")
		s.sensorFilters = append(s.sensorFilters, filter)

		items := s.sensors
		if filter != "" {
			// Only support the simple filters this package's Filter
			// builder actually generates, sufficient for these tests:
			// parentid = "X" and ancestors.id = "X".
			items = filterSensorsForTest(s.sensors, filter)
		}

		writeJSONArray(w, items)
	})
	mux.HandleFunc("/api/v2/experimental/channels", func(w http.ResponseWriter, r *http.Request) {
		filter := r.URL.Query().Get("filter")
		s.channelFilters = append(s.channelFilters, filter)

		// filter is always `parentid = "<sensorId>"` from this package.
		sensorID := extractQuotedValue(filter)
		writeJSONArray(w, s.channelsBySensor[sensorID])
	})
	return mux
}

// filterSensorsForTest applies just enough of the filter grammar this
// package emits (parentid=/ancestors.id=) to make the fake server useful,
// without reimplementing PRTG's real filter engine.
func filterSensorsForTest(all []SensorInfo, filter string) []SensorInfo {
	value := extractQuotedValue(filter)
	var out []SensorInfo
	for _, s := range all {
		switch {
		case strings.HasPrefix(filter, "parentid"):
			for _, ref := range s.Path {
				if ref.SimpleType() == "device" && ref.ID == value {
					out = append(out, s)
					break
				}
			}
		case strings.HasPrefix(filter, "ancestors.id"):
			for _, ref := range s.Path {
				if ref.ID == value {
					out = append(out, s)
					break
				}
			}
		default:
			out = append(out, s)
		}
	}
	return out
}

func extractQuotedValue(s string) string {
	start := strings.Index(s, `"`)
	if start < 0 {
		return ""
	}
	end := strings.Index(s[start+1:], `"`)
	if end < 0 {
		return ""
	}
	return s[start+1 : start+1+end]
}

func writeJSONArray(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if v == nil {
		_, _ = w.Write([]byte("[]"))
		return
	}
	_ = json.NewEncoder(w).Encode(v)
}

func sensorRef(deviceID, deviceName, sensorID, sensorName string) SensorInfo {
	return SensorInfo{
		ID:   sensorID,
		Name: sensorName,
		// Uses PRTG's real "REFERENCED_*" enum prefix (not the short form) so
		// this fixture doubles as a regression guard for deviceFromPath's use
		// of ReferencedObject.SimpleType() against the actual API shape.
		Path: []ReferencedObject{
			{ID: "0", Name: "Root", Type: "REFERENCED_GROUP"},
			{ID: deviceID, Name: deviceName, Type: "REFERENCED_DEVICE"},
			{ID: sensorID, Name: sensorName, Type: "REFERENCED_SENSOR"},
		},
	}
}

func newSearchTestClient(t *testing.T, s *searchTestServer) *Client {
	t.Helper()
	ts := httptest.NewServer(s.mux())
	t.Cleanup(ts.Close)
	base, err := ParseServerURL(ts.URL)
	if err != nil {
		t.Fatalf("ParseServerURL: %v", err)
	}
	c, err := NewClient(base, ts.Client(), &fakeAuthenticator{tokens: []string{"tok"}})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

func TestSearchSensors_NameAndExactChannelMatch(t *testing.T) {
	s := &searchTestServer{
		sensors: []SensorInfo{
			sensorRef("d1", "Device 1", "s1", "CPU Load"),
			sensorRef("d1", "Device 1", "s2", "Memory"),
			sensorRef("d2", "Device 2", "s3", "CPU Load"),
		},
		channelsBySensor: map[string][]ChannelInfo{
			"s1": {{ID: "s1.0", Name: "Total"}, {ID: "s1.1", Name: "Core 1"}},
			"s3": {{ID: "s3.0", Name: "Total"}},
		},
	}
	c := newSearchTestClient(t, s)

	result, err := SearchSensors(context.Background(), c, SearchSensorsParams{
		Pattern:          "^CPU Load$",
		ChannelMatchMode: ChannelMatchModeExact,
		ChannelPattern:   "Total",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalSensors != 2 {
		t.Fatalf("expected 2 matched sensors, got %d (%+v)", result.TotalSensors, result.Sensors)
	}
	if result.TotalChannels != 2 {
		t.Fatalf("expected 2 matched channels, got %d", result.TotalChannels)
	}
	if result.Truncated {
		t.Fatal("expected Truncated=false")
	}

	for _, sensor := range result.Sensors {
		if len(sensor.MatchedChannels) != 1 || sensor.MatchedChannels[0].Name != "Total" {
			t.Fatalf("expected sensor %q to match exactly channel 'Total', got %+v", sensor.Name, sensor.MatchedChannels)
		}
		if sensor.DeviceID == "" || sensor.DeviceName == "" {
			t.Fatalf("expected device id/name extracted from path, got %+v", sensor)
		}
	}
}

func TestSearchSensors_RegexChannelMatch(t *testing.T) {
	s := &searchTestServer{
		sensors: []SensorInfo{
			sensorRef("d1", "Device 1", "s1", "Bandwidth"),
		},
		channelsBySensor: map[string][]ChannelInfo{
			"s1": {
				{ID: "s1.0", Name: "Traffic In"},
				{ID: "s1.1", Name: "Traffic Out"},
				{ID: "s1.2", Name: "Downtime"},
			},
		},
	}
	c := newSearchTestClient(t, s)

	result, err := SearchSensors(context.Background(), c, SearchSensorsParams{
		Pattern:          "Bandwidth",
		ChannelMatchMode: ChannelMatchModeRegex,
		ChannelPattern:   "^Traffic",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalSensors != 1 || result.TotalChannels != 2 {
		t.Fatalf("expected 1 sensor / 2 channels, got %d/%d", result.TotalSensors, result.TotalChannels)
	}
}

func TestSearchSensors_EmptyChannelPatternMatchesAll(t *testing.T) {
	s := &searchTestServer{
		sensors: []SensorInfo{sensorRef("d1", "Device 1", "s1", "Ping")},
		channelsBySensor: map[string][]ChannelInfo{
			"s1": {{ID: "s1.0", Name: "Response Time"}, {ID: "s1.1", Name: "Packet Loss"}},
		},
	}
	c := newSearchTestClient(t, s)

	result, err := SearchSensors(context.Background(), c, SearchSensorsParams{Pattern: "Ping"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalChannels != 2 {
		t.Fatalf("expected all 2 channels matched when ChannelPattern is empty, got %d", result.TotalChannels)
	}
}

func TestSearchSensors_NoChannelMatchExcludesSensor(t *testing.T) {
	s := &searchTestServer{
		sensors: []SensorInfo{sensorRef("d1", "Device 1", "s1", "Ping")},
		channelsBySensor: map[string][]ChannelInfo{
			"s1": {{ID: "s1.0", Name: "Response Time"}},
		},
	}
	c := newSearchTestClient(t, s)

	result, err := SearchSensors(context.Background(), c, SearchSensorsParams{
		Pattern:        "Ping",
		ChannelPattern: "Nonexistent Channel",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalSensors != 0 {
		t.Fatalf("expected 0 sensors (no channel matched), got %d", result.TotalSensors)
	}
}

func TestSearchSensors_ScopeByDeviceID(t *testing.T) {
	s := &searchTestServer{
		sensors: []SensorInfo{
			sensorRef("d1", "Device 1", "s1", "CPU Load"),
			sensorRef("d2", "Device 2", "s2", "CPU Load"),
		},
		channelsBySensor: map[string][]ChannelInfo{
			"s1": {{ID: "s1.0", Name: "Total"}},
			"s2": {{ID: "s2.0", Name: "Total"}},
		},
	}
	c := newSearchTestClient(t, s)

	result, err := SearchSensors(context.Background(), c, SearchSensorsParams{Pattern: "CPU", DeviceID: "d1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalSensors != 1 || result.Sensors[0].ID != "s1" {
		t.Fatalf("expected only sensor s1 (scoped to device d1), got %+v", result.Sensors)
	}
	if len(s.sensorFilters) == 0 || !strings.Contains(s.sensorFilters[0], `parentid = "d1"`) {
		t.Fatalf("expected a parentid filter for d1, got %v", s.sensorFilters)
	}
}

func TestSearchSensors_ScopeByGroupIDUsesAncestors(t *testing.T) {
	s := &searchTestServer{
		sensors: []SensorInfo{sensorRef("d1", "Device 1", "s1", "CPU Load")},
		channelsBySensor: map[string][]ChannelInfo{
			"s1": {{ID: "s1.0", Name: "Total"}},
		},
	}
	c := newSearchTestClient(t, s)

	if _, err := SearchSensors(context.Background(), c, SearchSensorsParams{Pattern: "CPU", GroupID: "g1"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s.sensorFilters) == 0 || !strings.Contains(s.sensorFilters[0], `ancestors.id = "g1"`) {
		t.Fatalf("expected an ancestors.id filter for g1, got %v", s.sensorFilters)
	}
}

func TestSearchSensors_DeviceIDTakesPrecedenceOverGroupID(t *testing.T) {
	s := &searchTestServer{sensors: nil, channelsBySensor: map[string][]ChannelInfo{}}
	c := newSearchTestClient(t, s)

	if _, err := SearchSensors(context.Background(), c, SearchSensorsParams{Pattern: "x", DeviceID: "d1", GroupID: "g1"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(s.sensorFilters[0], `parentid = "d1"`) {
		t.Fatalf("expected deviceId to take precedence, got filter %q", s.sensorFilters[0])
	}
}

func TestSearchSensors_InvalidSensorPattern(t *testing.T) {
	c := newSearchTestClient(t, &searchTestServer{})
	_, err := SearchSensors(context.Background(), c, SearchSensorsParams{Pattern: "("})
	if err == nil {
		t.Fatal("expected an error for an invalid regex")
	}
	if !errors.Is(err, ErrInvalidPattern) {
		t.Fatalf("expected errors.Is(err, ErrInvalidPattern), got %v", err)
	}
}

func TestSearchSensors_InvalidChannelPattern(t *testing.T) {
	c := newSearchTestClient(t, &searchTestServer{})
	_, err := SearchSensors(context.Background(), c, SearchSensorsParams{
		Pattern:          "x",
		ChannelMatchMode: ChannelMatchModeRegex,
		ChannelPattern:   "(",
	})
	if err == nil {
		t.Fatal("expected an error for an invalid channel regex")
	}
	if !errors.Is(err, ErrInvalidPattern) {
		t.Fatalf("expected errors.Is(err, ErrInvalidPattern), got %v", err)
	}
}

func TestSearchSensors_TruncatedWhenMoreThanCapNameMatched(t *testing.T) {
	var sensors []SensorInfo
	channelsBySensor := map[string][]ChannelInfo{}
	for i := 0; i < maxSensorsWithChannels+5; i++ {
		id := "s" + string(rune('a'+i%26)) + string(rune('0'+i/26))
		sensors = append(sensors, sensorRef("d1", "Device 1", id, "Match Me"))
		channelsBySensor[id] = []ChannelInfo{{ID: id + ".0", Name: "Total"}}
	}
	s := &searchTestServer{sensors: sensors, channelsBySensor: channelsBySensor}
	c := newSearchTestClient(t, s)

	result, err := SearchSensors(context.Background(), c, SearchSensorsParams{Pattern: "Match Me"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalSensors != maxSensorsWithChannels {
		t.Fatalf("expected exactly %d sensors processed, got %d", maxSensorsWithChannels, result.TotalSensors)
	}
	if !result.Truncated {
		t.Fatal("expected Truncated=true when more than the channel-fetch cap matched by name")
	}
}

func TestCompileChannelMatcher(t *testing.T) {
	t.Run("empty pattern matches everything", func(t *testing.T) {
		match, err := CompileChannelMatcher("", "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !match("anything") {
			t.Fatal("expected empty pattern to match")
		}
	})
	t.Run("exact mode", func(t *testing.T) {
		match, err := CompileChannelMatcher(ChannelMatchModeExact, "Total")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !match("Total") || match("total") || match("Total 2") {
			t.Fatal("expected exact match semantics")
		}
	})
	t.Run("regex mode", func(t *testing.T) {
		match, err := CompileChannelMatcher(ChannelMatchModeRegex, "^Total.*")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !match("Total (Speed)") || match("Not Total") {
			t.Fatal("expected regex match semantics")
		}
	})
	t.Run("invalid regex", func(t *testing.T) {
		if _, err := CompileChannelMatcher(ChannelMatchModeRegex, "("); err == nil {
			t.Fatal("expected an error for invalid regex")
		}
	})
}
