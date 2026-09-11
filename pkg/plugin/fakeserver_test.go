package plugin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"testing"

	"github.com/grafana/grafana-plugin-sdk-go/backend/resource/httpadapter"
	"github.com/srgssr/prtg-datasource/pkg/prtg"
)

// fakeSensor/fakeDevice associate a PRTG object with the parent id(s) this
// test double's simplified filter interpreter needs -- a real PRTG server
// derives this from the object tree, but for these tests it's simplest to
// just declare it directly alongside the object.
type fakeDevice struct {
	GroupID string
	Info    prtg.DeviceInfo
}

type fakeSensor struct {
	DeviceID string
	GroupID  string // the device's owning group/probe, for ancestors.id scoping
	Info     prtg.SensorInfo
}

// fakePRTGServer is a minimal fake PRTG APIv2 server used to exercise
// pkg/plugin's QueryData/CallResource/CheckHealth against a real
// *prtg.Client, per the plan's guidance to prefer this over mocking the
// prtg package itself for higher-fidelity plugin-level tests.
//
// Its filter interpreter only understands the small set of `field = "value"`
// clauses this codebase's own prtg.Filter builder ever emits (parentid=,
// ancestors.id=, id=, possibly ANDed together) -- it is not a PRTG filter
// engine.
type fakePRTGServer struct {
	mu sync.Mutex

	Groups  []prtg.GroupInfo
	Probes  []prtg.GroupInfo
	Devices []fakeDevice
	Sensors []fakeSensor
	// ChannelsBySensor maps a sensor id to its channels.
	ChannelsBySensor map[string][]prtg.ChannelInfo
	// TimeSeries maps "<sensorId>/<window>" to the raw timeseries response
	// body (row 0 header, rows thereafter -- the actual PRTG wire shape).
	TimeSeries map[string][][]interface{}

	// GroupsStatus/GroupsErrorBody override the /experimental/groups
	// response (used by CheckHealth and the "groups" resource route);
	// GroupsStatus == 0 means "respond normally with Groups".
	GroupsStatus    int
	GroupsErrorBody string

	Requests []string // "METHOD PATH?QUERY" in call order, for assertions
}

func newFakePRTGServer() *fakePRTGServer {
	return &fakePRTGServer{ChannelsBySensor: map[string][]prtg.ChannelInfo{}, TimeSeries: map[string][][]interface{}{}}
}

var filterClauseRe = regexp.MustCompile(`([A-Za-z0-9_.]+)\s*=\s*"([^"]*)"`)

// parseSimpleFilter extracts every `field = "value"` clause from a filter
// string, regardless of `and`/parens -- sufficient for the filters this
// codebase's own Filter builder generates.
func parseSimpleFilter(filter string) map[string]string {
	out := map[string]string{}
	for _, m := range filterClauseRe.FindAllStringSubmatch(filter, -1) {
		out[m[1]] = m[2]
	}
	return out
}

func (s *fakePRTGServer) logRequest(r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	q := r.URL.RawQuery
	entry := r.Method + " " + r.URL.Path
	if q != "" {
		entry += "?" + q
	}
	s.Requests = append(s.Requests, entry)
}

func writeJSONBody(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if v == nil {
		_, _ = w.Write([]byte("[]"))
		return
	}
	_ = json.NewEncoder(w).Encode(v)
}

func (s *fakePRTGServer) mux() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v2/experimental/groups", func(w http.ResponseWriter, r *http.Request) {
		s.logRequest(r)
		if s.GroupsStatus != 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(s.GroupsStatus)
			_, _ = w.Write([]byte(s.GroupsErrorBody))
			return
		}
		writeJSONBody(w, s.Groups)
	})

	mux.HandleFunc("GET /api/v2/experimental/probes", func(w http.ResponseWriter, r *http.Request) {
		s.logRequest(r)
		writeJSONBody(w, s.Probes)
	})

	mux.HandleFunc("GET /api/v2/experimental/devices", func(w http.ResponseWriter, r *http.Request) {
		s.logRequest(r)
		clauses := parseSimpleFilter(r.URL.Query().Get("filter"))
		var out []prtg.DeviceInfo
		for _, d := range s.Devices {
			if parentID, ok := clauses["parentid"]; ok && d.GroupID != parentID {
				continue
			}
			out = append(out, d.Info)
		}
		writeJSONBody(w, out)
	})

	mux.HandleFunc("GET /api/v2/experimental/sensors", func(w http.ResponseWriter, r *http.Request) {
		s.logRequest(r)
		clauses := parseSimpleFilter(r.URL.Query().Get("filter"))
		var out []prtg.SensorInfo
		for _, sn := range s.Sensors {
			if parentID, ok := clauses["parentid"]; ok && sn.DeviceID != parentID {
				continue
			}
			if groupID, ok := clauses["ancestors.id"]; ok && sn.GroupID != groupID {
				continue
			}
			out = append(out, sn.Info)
		}
		writeJSONBody(w, out)
	})

	mux.HandleFunc("GET /api/v2/experimental/channels", func(w http.ResponseWriter, r *http.Request) {
		s.logRequest(r)
		clauses := parseSimpleFilter(r.URL.Query().Get("filter"))
		sensorID := clauses["parentid"]
		channels := s.ChannelsBySensor[sensorID]
		var out []prtg.ChannelInfo
		for _, ch := range channels {
			if wantID, ok := clauses["id"]; ok && ch.ID != wantID {
				continue
			}
			out = append(out, ch)
		}
		writeJSONBody(w, out)
	})

	mux.HandleFunc("GET /api/v2/experimental/timeseries/{sensorId}/{window}", func(w http.ResponseWriter, r *http.Request) {
		s.logRequest(r)
		key := r.PathValue("sensorId") + "/" + r.PathValue("window")
		rows, ok := s.TimeSeries[key]
		if !ok {
			writeJSONBody(w, [][]interface{}{})
			return
		}
		writeJSONBody(w, rows)
	})

	return mux
}

// newTestDatasource builds a *Datasource whose prtg.Client points at an
// httptest.Server backed by s, using a static API key authenticator (auth
// mode is irrelevant to these tests, which exercise QueryData/CallResource/
// CheckHealth, not auth itself -- see auth_test.go/client_test.go in
// pkg/prtg for that).
func newTestDatasource(t *testing.T, s *fakePRTGServer) *Datasource {
	t.Helper()
	ts := httptest.NewServer(s.mux())
	t.Cleanup(ts.Close)

	base, err := prtg.ParseServerURL(ts.URL)
	if err != nil {
		t.Fatalf("ParseServerURL: %v", err)
	}

	client, err := prtg.NewClient(base, ts.Client(), &prtg.APIKeyAuthenticator{APIKey: "test-key"})
	if err != nil {
		t.Fatalf("prtg.NewClient: %v", err)
	}

	ds := &Datasource{client: client}
	ds.resourceHandler = httpadapter.New(ds.registerRoutes())
	return ds
}
