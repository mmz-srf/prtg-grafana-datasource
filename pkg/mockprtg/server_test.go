package mockprtg

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/srgssr/prtg-datasource/pkg/prtg"
)

func newTestServer(t *testing.T) (*Server, *Dataset) {
	t.Helper()
	ds := DefaultDataset()
	srv := NewServer(ds, Options{APIKey: "test-key", Username: "u", Password: "p"})
	fixed := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	srv.Now = func() time.Time { return fixed }
	return srv, ds
}

func doRequest(t *testing.T, srv *Server, method, target, bearer string, body io.Reader) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, body)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	return rec
}

func decodeArray[T any](t *testing.T, rec *httptest.ResponseRecorder) []T {
	t.Helper()
	var items []T
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("decoding response: %v\nbody: %s", err, rec.Body.String())
	}
	return items
}

func findGroupByName(ds *Dataset, name string) prtg.GroupInfo {
	for _, g := range ds.Groups {
		if g.Name == name {
			return g
		}
	}
	return prtg.GroupInfo{}
}

func findDeviceByName(ds *Dataset, name string) prtg.DeviceInfo {
	for _, d := range ds.Devices {
		if d.Name == name {
			return d
		}
	}
	return prtg.DeviceInfo{}
}

func findSensorByName(ds *Dataset, deviceID, name string) prtg.SensorInfo {
	for _, s := range ds.Sensors {
		if s.Name == name && ds.parentOf[s.ID] == deviceID {
			return s
		}
	}
	return prtg.SensorInfo{}
}

func filterQuery(field, value string) string {
	return url.QueryEscape(fmt.Sprintf(`%s = "%s"`, field, value))
}

func TestAuth(t *testing.T) {
	srv, _ := newTestServer(t)

	rec := doRequest(t, srv, http.MethodGet, "/api/v2/experimental/groups", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no bearer token: expected 401, got %d", rec.Code)
	}
	var apiErr struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &apiErr); err != nil {
		t.Fatalf("decoding error body: %v", err)
	}
	if apiErr.Code != "AUTH_FAILED" || apiErr.RequestID == "" {
		t.Fatalf("expected AUTH_FAILED with a request id, got %+v", apiErr)
	}

	rec = doRequest(t, srv, http.MethodGet, "/api/v2/experimental/groups", "wrong-key", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong bearer token: expected 401, got %d", rec.Code)
	}

	rec = doRequest(t, srv, http.MethodGet, "/api/v2/experimental/groups", "test-key", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("correct bearer token: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGroupsAndProbes(t *testing.T) {
	srv, ds := newTestServer(t)

	rec := doRequest(t, srv, http.MethodGet, "/api/v2/experimental/groups", "test-key", nil)
	groups := decodeArray[prtg.GroupInfo](t, rec)
	if len(groups) != len(ds.Groups) {
		t.Fatalf("expected %d groups, got %d", len(ds.Groups), len(groups))
	}
	if got := rec.Header().Get("X-Total-Count"); got != strconv.Itoa(len(ds.Groups)) {
		t.Fatalf("X-Total-Count = %q, want %d", got, len(ds.Groups))
	}
	if len(groups) > 0 && len(groups[0].Path) == 0 {
		t.Fatal("expected a group's Path breadcrumb to be populated")
	}

	rec = doRequest(t, srv, http.MethodGet, "/api/v2/experimental/probes", "test-key", nil)
	probes := decodeArray[prtg.GroupInfo](t, rec)
	if len(probes) != len(ds.Probes) {
		t.Fatalf("expected %d probes, got %d", len(ds.Probes), len(probes))
	}
}

func TestDevicesFilterByParent(t *testing.T) {
	srv, ds := newTestServer(t)
	group := findGroupByName(ds, "Core Infrastructure")
	if group.ID == "" {
		t.Fatal("seed group 'Core Infrastructure' not found")
	}

	rec := doRequest(t, srv, http.MethodGet, "/api/v2/experimental/devices?filter="+filterQuery("parentid", group.ID), "test-key", nil)
	devices := decodeArray[prtg.DeviceInfo](t, rec)
	if len(devices) != 2 {
		t.Fatalf("expected 2 devices under %q, got %d: %+v", group.Name, len(devices), devices)
	}
}

func TestSensorsFilterByParentAndAncestor(t *testing.T) {
	srv, ds := newTestServer(t)
	device := findDeviceByName(ds, "app-server-01")
	group := findGroupByName(ds, "Application Servers")

	rec := doRequest(t, srv, http.MethodGet, "/api/v2/experimental/sensors?filter="+filterQuery("parentid", device.ID), "test-key", nil)
	sensors := decodeArray[prtg.SensorInfo](t, rec)
	if len(sensors) != 3 {
		t.Fatalf("expected 3 sensors under app-server-01, got %d", len(sensors))
	}

	rec = doRequest(t, srv, http.MethodGet, "/api/v2/experimental/sensors?filter="+filterQuery("ancestors.id", group.ID), "test-key", nil)
	sensors = decodeArray[prtg.SensorInfo](t, rec)
	if len(sensors) != 5 { // app-server-01 (3 sensors) + app-server-02 (2 sensors)
		t.Fatalf("expected 5 sensors under group %q, got %d", group.Name, len(sensors))
	}
}

func TestChannelsIncludesMeasurement(t *testing.T) {
	srv, ds := newTestServer(t)
	device := findDeviceByName(ds, "core-router-01")
	sensor := findSensorByName(ds, device.ID, "CPU Load")
	if sensor.ID == "" {
		t.Fatal("seed sensor 'CPU Load' not found under core-router-01")
	}

	rec := doRequest(t, srv, http.MethodGet, "/api/v2/experimental/channels?filter="+filterQuery("parentid", sensor.ID), "test-key", nil)
	channels := decodeArray[prtg.ChannelInfo](t, rec)
	if len(channels) != 2 {
		t.Fatalf("expected 2 channels, got %d", len(channels))
	}
	for _, ch := range channels {
		if ch.LastMeasurement.Value == nil {
			t.Fatalf("channel %q: expected a non-nil LastMeasurement.Value", ch.Name)
		}
		if *ch.LastMeasurement.Value < 0 || *ch.LastMeasurement.Value > 100 {
			t.Fatalf("channel %q: value %v out of expected [0,100] range", ch.Name, *ch.LastMeasurement.Value)
		}
		if ch.Unit.DisplayUnit != "%" {
			t.Fatalf("channel %q: expected unit %%, got %q", ch.Name, ch.Unit.DisplayUnit)
		}
		if ch.LastMeasurement.Average == nil || ch.LastMeasurement.Minimum == nil || ch.LastMeasurement.Maximum == nil {
			t.Fatalf("channel %q: expected Average/Minimum/Maximum to be populated", ch.Name)
		}
	}
}

func TestPagination(t *testing.T) {
	srv, ds := newTestServer(t)
	total := len(ds.Sensors)
	if total < 3 {
		t.Fatalf("need at least 3 seeded sensors for a pagination test, have %d", total)
	}

	rec := doRequest(t, srv, http.MethodGet, "/api/v2/experimental/sensors?limit=2&offset=0", "test-key", nil)
	page := decodeArray[prtg.SensorInfo](t, rec)
	if len(page) != 2 {
		t.Fatalf("expected a 2-item page, got %d", len(page))
	}
	if got := rec.Header().Get("X-Total-Count"); got != strconv.Itoa(total) {
		t.Fatalf("X-Total-Count = %q, want %d", got, total)
	}
	if got := rec.Header().Get("X-Result-Count"); got != "2" {
		t.Fatalf("X-Result-Count = %q, want 2", got)
	}

	rec = doRequest(t, srv, http.MethodGet, fmt.Sprintf("/api/v2/experimental/sensors?limit=100&offset=%d", total), "test-key", nil)
	page = decodeArray[prtg.SensorInfo](t, rec)
	if len(page) != 0 {
		t.Fatalf("expected an empty page past the end, got %d items", len(page))
	}
	if got := rec.Header().Get("X-Total-Count"); got != strconv.Itoa(total) {
		t.Fatalf("X-Total-Count on empty page = %q, want %d", got, total)
	}
}

func TestTimeseriesWindows(t *testing.T) {
	srv, ds := newTestServer(t)
	device := findDeviceByName(ds, "core-router-01")
	sensor := findSensorByName(ds, device.ID, "Ping")

	for window, wantPoints := range windowPoints {
		t.Run(window, func(t *testing.T) {
			rec := doRequest(t, srv, http.MethodGet, "/api/v2/experimental/timeseries/"+sensor.ID+"/"+window, "test-key", nil)
			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
			}
			var rows [][]interface{}
			if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
				t.Fatalf("decoding timeseries: %v", err)
			}
			if len(rows) != wantPoints+1 {
				t.Fatalf("window %q: expected %d rows (including header), got %d", window, wantPoints+1, len(rows))
			}

			header := rows[0]
			if len(header) != 3 { // time + 2 ping channels (latency, loss)
				t.Fatalf("window %q: expected 3 header cells, got %d: %v", window, len(header), header)
			}
			if header[0] != "time" {
				t.Fatalf("window %q: expected header[0] == \"time\", got %v", window, header[0])
			}
			wantCol := sensor.ID + ".0"
			if header[1] != wantCol {
				t.Fatalf("window %q: expected header[1] == %q, got %v", window, wantCol, header[1])
			}

			if v, ok := rows[1][1].(float64); !ok || v < 0 {
				t.Fatalf("window %q: expected a numeric, non-gap value in row 1, got %v", window, rows[1][1])
			}
		})
	}
}

func TestTimeseriesInvalidWindow(t *testing.T) {
	srv, ds := newTestServer(t)
	sensor := ds.Sensors[0]

	rec := doRequest(t, srv, http.MethodGet, "/api/v2/experimental/timeseries/"+sensor.ID+"/bogus", "test-key", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for an invalid window, got %d", rec.Code)
	}
}

func TestSessionLogin(t *testing.T) {
	srv, _ := newTestServer(t)

	rec := doRequest(t, srv, http.MethodPost, "/api/v2/session", "", strings.NewReader(`{"username":"u","password":"wrong"}`))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for wrong credentials, got %d", rec.Code)
	}

	rec = doRequest(t, srv, http.MethodPost, "/api/v2/session", "", strings.NewReader(`{"username":"u","password":"p"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for correct credentials, got %d: %s", rec.Code, rec.Body.String())
	}
	var loginResult struct {
		Token     string `json:"token"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &loginResult); err != nil {
		t.Fatalf("decoding session response: %v", err)
	}
	if loginResult.Token == "" {
		t.Fatal("expected a non-empty session token")
	}
	if _, err := time.Parse(time.RFC3339, loginResult.ExpiresAt); err != nil {
		t.Fatalf("expires_at %q did not parse as RFC3339: %v", loginResult.ExpiresAt, err)
	}

	// The freshly issued session token should now work as a bearer token.
	rec = doRequest(t, srv, http.MethodGet, "/api/v2/experimental/groups", loginResult.Token, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 using the issued session token, got %d", rec.Code)
	}
}
