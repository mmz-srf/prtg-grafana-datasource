package mockprtg

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/srgssr/prtg-datasource/pkg/prtg"
)

// requestIDCounter backs nextRequestID -- a plain incrementing counter is
// enough for a mock's error envelope's request_id, no randomness needed.
var requestIDCounter uint64

func nextRequestID() string {
	n := atomic.AddUint64(&requestIDCounter, 1)
	return fmt.Sprintf("mock-%d", n)
}

// writeJSONArray writes v (expected to be a slice) as the response body,
// falling back to a literal "[]" for a nil slice so callers always get a
// JSON array, never `null`.
func writeJSONArray(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v == nil {
		_, _ = w.Write([]byte("[]"))
		return
	}
	_ = json.NewEncoder(w).Encode(v)
}

// writeAPIError writes PRTG APIv2's structured error envelope
// ({"code","message","request_id"}), matching pkg/prtg/errors.go's APIError
// shape so the real client's error handling (AsAPIError, Is*) works
// unmodified against this mock.
func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"code":       code,
		"message":    message,
		"request_id": nextRequestID(),
	})
}

// parseOffsetLimit reads offset/limit query params, defaulting to PRTG
// APIv2's own documented defaults (see pkg/prtg/pagination.go).
func parseOffsetLimit(r *http.Request) (offset, limit int) {
	offset, limit = 0, 100
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	return offset, limit
}

// paginate slices items down to one [offset, offset+limit) page, returning
// the page and the pre-pagination total.
func paginate[T any](items []T, r *http.Request) (page []T, total int) {
	offset, limit := parseOffsetLimit(r)
	total = len(items)
	end := offset + limit
	if offset > total {
		offset = total
	}
	if end > total {
		end = total
	}
	if offset < end {
		page = items[offset:end]
	}
	return page, total
}

// serveList writes items (already filtered by the caller) as one paginated
// JSON array response, setting X-Total-Count/X-Result-Count on every
// response -- including an empty one -- since pkg/prtg/pagination.go's
// FetchAll/parseCountHeader rely on these headers, not a missing-header
// fallback, to terminate correctly.
func serveList[T any](w http.ResponseWriter, r *http.Request, items []T) {
	page, total := paginate(items, r)
	w.Header().Set("X-Total-Count", strconv.Itoa(total))
	w.Header().Set("X-Result-Count", strconv.Itoa(len(page)))
	writeJSONArray(w, http.StatusOK, page)
}

// handleGroups implements GET /api/v2/experimental/groups: the plugin's
// "groups" resource route fetches this (and /probes) unfiltered, so no
// filter clause is honored here.
func (s *Server) handleGroups(w http.ResponseWriter, r *http.Request) {
	serveList(w, r, s.Dataset.Groups)
}

// handleProbes implements GET /api/v2/experimental/probes.
func (s *Server) handleProbes(w http.ResponseWriter, r *http.Request) {
	serveList(w, r, s.Dataset.Probes)
}

// handleDevices implements GET /api/v2/experimental/devices, honoring an
// optional `filter=parentid = "<groupOrProbeId>"` clause.
func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	clauses := ParseSimpleFilter(r.URL.Query().Get("filter"))
	parentID, hasParent := clauses["parentid"]

	items := make([]prtg.DeviceInfo, 0, len(s.Dataset.Devices))
	for _, d := range s.Dataset.Devices {
		if hasParent && s.Dataset.parentOf[d.ID] != parentID {
			continue
		}
		items = append(items, d)
	}
	serveList(w, r, items)
}

// handleSensors implements GET /api/v2/experimental/sensors, honoring
// optional `filter=parentid = "<deviceId>"` and/or
// `ancestors.id = "<groupOrProbeId>"` clauses (ANDed together, matching the
// only combination pkg/prtg's Filter builder/search.go ever emits).
func (s *Server) handleSensors(w http.ResponseWriter, r *http.Request) {
	clauses := ParseSimpleFilter(r.URL.Query().Get("filter"))
	parentID, hasParent := clauses["parentid"]
	ancestorID, hasAncestor := clauses["ancestors.id"]

	items := make([]prtg.SensorInfo, 0, len(s.Dataset.Sensors))
	for _, sn := range s.Dataset.Sensors {
		if hasParent && s.Dataset.parentOf[sn.ID] != parentID {
			continue
		}
		if hasAncestor && !containsString(s.Dataset.ancestorsOf[sn.ID], ancestorID) {
			continue
		}
		items = append(items, sn)
	}
	serveList(w, r, items)
}

// handleChannels implements GET /api/v2/experimental/channels, honoring
// optional `filter=parentid = "<sensorId>"` and/or `id = "<channelId>"`
// clauses, and fills in each channel's LastMeasurement by evaluating the
// mock's signal generator at "now" -- channels are never stored with a
// stale, precomputed measurement.
func (s *Server) handleChannels(w http.ResponseWriter, r *http.Request) {
	clauses := ParseSimpleFilter(r.URL.Query().Get("filter"))
	sensorID := clauses["parentid"]
	wantID, hasWantID := clauses["id"]

	template := s.Dataset.ChannelsBySensor[sensorID]
	now := s.now()
	values := s.Dataset.EvaluateSensor(sensorID, now)

	items := make([]prtg.ChannelInfo, 0, len(template))
	for _, ch := range template {
		if hasWantID && ch.ID != wantID {
			continue
		}
		if v, ok := values[ch.ID]; ok {
			ch.LastMeasurement = s.Dataset.measurementFor(sensorID, ch.ID, v, ch.Unit, now)
		}
		items = append(items, ch)
	}
	serveList(w, r, items)
}

// validWindows maps the four fixed PRTG APIv2 timeseries windows (see
// pkg/prtg/timeseries.go) to their span, since that map is unexported there.
var validWindows = map[string]time.Duration{
	"live":   4 * time.Hour,
	"short":  2 * 24 * time.Hour,
	"medium": 60 * 24 * time.Hour,
	"long":   365 * 24 * time.Hour,
}

// windowPoints controls how many samples this mock generates per fixed
// window, coarsening resolution for longer windows the way PRTG's own
// historic data does, while keeping every window's payload small.
var windowPoints = map[string]int{
	"live":   240,
	"short":  576,
	"medium": 720,
	"long":   365,
}

// handleTimeseries implements
// GET /api/v2/experimental/timeseries/{sensorId}/{window}, returning the
// exact [][]interface{} wire shape pkg/prtg/timeseries.go's
// parseTimeSeriesRows expects: row 0 is ["time", "<sensorId>.<channelId>",
// ...], each row thereafter is [RFC3339 string, float64, ...]. An optional
// `channels=<sensorId>.<channelId>,...` query param (as FetchTimeSeries
// always sends) selects a channel subset.
func (s *Server) handleTimeseries(w http.ResponseWriter, r *http.Request) {
	sensorID := r.PathValue("sensorId")
	windowParam := r.PathValue("window")

	span, ok := validWindows[windowParam]
	if !ok {
		writeAPIError(w, http.StatusBadRequest, "INVALID_FILTER", "window must be one of live, short, medium, long")
		return
	}

	template := s.Dataset.ChannelsBySensor[sensorID]
	selected := template
	if raw := r.URL.Query().Get("channels"); raw != "" {
		want := map[string]bool{}
		for _, part := range strings.Split(raw, ",") {
			want[strings.TrimSpace(part)] = true
		}
		selected = make([]prtg.ChannelInfo, 0, len(template))
		for _, ch := range template {
			if want[sensorID+"."+ch.ID] {
				selected = append(selected, ch)
			}
		}
	}

	if len(selected) == 0 {
		writeJSONArray(w, http.StatusOK, [][]interface{}{})
		return
	}

	points := windowPoints[windowParam]
	now := s.now()
	step := span / time.Duration(points-1)

	header := make([]interface{}, 0, len(selected)+1)
	header = append(header, "time")
	for _, ch := range selected {
		header = append(header, sensorID+"."+ch.ID)
	}

	rows := make([][]interface{}, 0, points+1)
	rows = append(rows, header)
	for i := 0; i < points; i++ {
		t := now.Add(-span + time.Duration(i)*step)
		values := s.Dataset.EvaluateSensor(sensorID, t)

		row := make([]interface{}, 0, len(selected)+1)
		row = append(row, t.UTC().Format(time.RFC3339))
		for _, ch := range selected {
			if v, exists := values[ch.ID]; exists {
				row = append(row, v)
			} else {
				row = append(row, nil)
			}
		}
		rows = append(rows, row)
	}
	writeJSONArray(w, http.StatusOK, rows)
}

// handleHealthz implements GET /healthz: an unauthenticated liveness check
// for the Dockerfile HEALTHCHECK and manual `curl` probing.
func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func containsString(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

// formatDisplayValue renders a plain human-readable value+unit string, e.g.
// "42.31 %". Not consumed by pkg/plugin (which only reads LastMeasurement's
// Value/Timestamp), but realistic enough to be useful when curling the mock
// directly.
func formatDisplayValue(v float64, unit prtg.Unit) string {
	if unit.DisplayUnit != "" {
		return fmt.Sprintf("%.2f %s", v, unit.DisplayUnit)
	}
	return fmt.Sprintf("%.2f", v)
}

// measurementFor evaluates channel chID (belonging to sensorID, already
// known to have value v at time t) into a full Measurement, filling
// Average/Minimum/Maximum by sampling the last 4 hours of the same signal.
func (ds *Dataset) measurementFor(sensorID, chID string, v float64, unit prtg.Unit, t time.Time) prtg.Measurement {
	value := v
	m := prtg.Measurement{
		Value:        &value,
		DisplayValue: formatDisplayValue(v, unit),
		Timestamp:    t.UTC().Format(time.RFC3339),
	}
	if avg, min, max, ok := ds.sampleChannelOverWindow(sensorID, chID, t, 4*time.Hour, 12); ok {
		m.Average, m.Minimum, m.Maximum = &avg, &min, &max
	}
	return m
}
