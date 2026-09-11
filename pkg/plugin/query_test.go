package plugin

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/srgssr/prtg-datasource/pkg/prtg"
)

func mustJSON(t *testing.T, v interface{}) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal query: %v", err)
	}
	return raw
}

func runSingleQuery(t *testing.T, ds *Datasource, qm queryModel, timeRange backend.TimeRange) backend.DataResponse {
	t.Helper()
	resp, err := ds.QueryData(context.Background(), &backend.QueryDataRequest{
		Queries: []backend.DataQuery{
			{RefID: "A", JSON: mustJSON(t, qm), TimeRange: timeRange},
		},
	})
	if err != nil {
		t.Fatalf("QueryData error: %v", err)
	}
	return resp.Responses["A"]
}

func fieldByName(t *testing.T, frame *data.Frame, name string) *data.Field {
	t.Helper()
	for _, f := range frame.Fields {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("frame %q has no field %q", frame.Name, name)
	return nil
}

func chPath(segments ...prtg.ReferencedObject) []prtg.ReferencedObject { return segments }

func TestQueryData_CurrentValue_SingleChannel(t *testing.T) {
	s := newFakePRTGServer()
	value := 42.5
	s.ChannelsBySensor["s1"] = []prtg.ChannelInfo{
		{
			ID:   "s1.0",
			Name: "Total",
			Unit: prtg.Unit{DisplayUnit: "%"},
			LastMeasurement: prtg.Measurement{
				Value:        &value,
				DisplayValue: "42.5 %",
				Timestamp:    "2024-01-01T12:00:00Z",
			},
			Path: chPath(
				prtg.ReferencedObject{ID: "g1", Name: "Network", Type: "REFERENCED_GROUP"},
				prtg.ReferencedObject{ID: "d1", Name: "Router 1", Type: "REFERENCED_DEVICE"},
				prtg.ReferencedObject{ID: "s1", Name: "CPU", Type: "REFERENCED_SENSOR"},
				prtg.ReferencedObject{ID: "s1.0", Name: "Total", Type: "REFERENCED_CHANNEL"},
			),
		},
	}
	ds := newTestDatasource(t, s)

	resp := runSingleQuery(t, ds, queryModel{QueryType: "current", SensorID: "s1", ChannelID: "s1.0"}, backend.TimeRange{})
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	if len(resp.Frames) != 1 {
		t.Fatalf("expected 1 frame, got %d", len(resp.Frames))
	}

	frame := resp.Frames[0]
	if frame.Name != "Total" {
		t.Fatalf("expected frame name 'Total', got %q", frame.Name)
	}

	valueField := fieldByName(t, frame, "value")
	got, ok := valueField.At(0).(*float64)
	if !ok || got == nil || *got != 42.5 {
		t.Fatalf("expected value 42.5, got %v", valueField.At(0))
	}

	wantLabels := data.Labels{"group": "Network", "device": "Router 1", "sensor": "CPU", "channel": "Total"}
	if valueField.Labels.String() != wantLabels.String() {
		t.Fatalf("expected labels %v, got %v", wantLabels, valueField.Labels)
	}

	timeField := fieldByName(t, frame, "time")
	gotTime, ok := timeField.At(0).(time.Time)
	if !ok || !gotTime.Equal(time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("expected the parsed measurement timestamp, got %v", timeField.At(0))
	}
}

func TestQueryData_CurrentValue_AllChannelsFanOut(t *testing.T) {
	s := newFakePRTGServer()
	s.ChannelsBySensor["s1"] = []prtg.ChannelInfo{
		{ID: "s1.0", Name: "Total"},
		{ID: "s1.1", Name: "Core 1"},
		{ID: "s1.2", Name: "Core 2"},
	}
	ds := newTestDatasource(t, s)

	for _, channelID := range []string{"", "*"} {
		resp := runSingleQuery(t, ds, queryModel{QueryType: "current", SensorID: "s1", ChannelID: channelID}, backend.TimeRange{})
		if resp.Error != nil {
			t.Fatalf("unexpected error (channelId=%q): %v", channelID, resp.Error)
		}
		if len(resp.Frames) != 3 {
			t.Fatalf("expected 3 frames (channelId=%q), got %d", channelID, len(resp.Frames))
		}
	}
}

func TestQueryData_CurrentValue_ChannelNotFound(t *testing.T) {
	s := newFakePRTGServer()
	s.ChannelsBySensor["s1"] = []prtg.ChannelInfo{{ID: "s1.0", Name: "Total"}}
	ds := newTestDatasource(t, s)

	resp := runSingleQuery(t, ds, queryModel{QueryType: "current", SensorID: "s1", ChannelID: "does-not-exist"}, backend.TimeRange{})
	if resp.Error == nil {
		t.Fatal("expected an error for a missing channel")
	}
	if resp.Status != backend.StatusNotFound {
		t.Fatalf("expected StatusNotFound, got %v", resp.Status)
	}
}

func TestQueryData_IncompleteHierarchyQuery_NoFramesNoError(t *testing.T) {
	s := newFakePRTGServer()
	ds := newTestDatasource(t, s)

	resp := runSingleQuery(t, ds, queryModel{QueryType: "current"}, backend.TimeRange{})
	if resp.Error != nil {
		t.Fatalf("expected no error for an incomplete query, got %v", resp.Error)
	}
	if len(resp.Frames) != 0 {
		t.Fatalf("expected no frames, got %d", len(resp.Frames))
	}
}

func TestQueryData_RegexFanOut_DistinctLabels(t *testing.T) {
	s := newFakePRTGServer()
	s.Sensors = []fakeSensor{
		{DeviceID: "d1", Info: prtg.SensorInfo{ID: "s1", Name: "CPU Load A", Path: chPath(
			prtg.ReferencedObject{ID: "d1", Name: "Device A", Type: "REFERENCED_DEVICE"},
			prtg.ReferencedObject{ID: "s1", Name: "CPU Load A", Type: "REFERENCED_SENSOR"},
		)}},
		{DeviceID: "d2", Info: prtg.SensorInfo{ID: "s2", Name: "CPU Load B", Path: chPath(
			prtg.ReferencedObject{ID: "d2", Name: "Device B", Type: "REFERENCED_DEVICE"},
			prtg.ReferencedObject{ID: "s2", Name: "CPU Load B", Type: "REFERENCED_SENSOR"},
		)}},
		{DeviceID: "d1", Info: prtg.SensorInfo{ID: "s3", Name: "Memory", Path: chPath(
			prtg.ReferencedObject{ID: "d1", Name: "Device A", Type: "REFERENCED_DEVICE"},
			prtg.ReferencedObject{ID: "s3", Name: "Memory", Type: "REFERENCED_SENSOR"},
		)}},
	}
	s.ChannelsBySensor["s1"] = []prtg.ChannelInfo{{ID: "s1.0", Name: "Total", Path: chPath(
		prtg.ReferencedObject{ID: "d1", Name: "Device A", Type: "REFERENCED_DEVICE"},
		prtg.ReferencedObject{ID: "s1", Name: "CPU Load A", Type: "REFERENCED_SENSOR"},
		prtg.ReferencedObject{ID: "s1.0", Name: "Total", Type: "REFERENCED_CHANNEL"},
	)}}
	s.ChannelsBySensor["s2"] = []prtg.ChannelInfo{{ID: "s2.0", Name: "Total", Path: chPath(
		prtg.ReferencedObject{ID: "d2", Name: "Device B", Type: "REFERENCED_DEVICE"},
		prtg.ReferencedObject{ID: "s2", Name: "CPU Load B", Type: "REFERENCED_SENSOR"},
		prtg.ReferencedObject{ID: "s2.0", Name: "Total", Type: "REFERENCED_CHANNEL"},
	)}}
	s.ChannelsBySensor["s3"] = []prtg.ChannelInfo{{ID: "s3.0", Name: "Used", Path: chPath()}}
	ds := newTestDatasource(t, s)

	resp := runSingleQuery(t, ds, queryModel{
		QueryType:        "current",
		UseRegex:         true,
		SensorPattern:    "^CPU Load",
		ChannelMatchMode: "exact",
		ChannelPattern:   "Total",
	}, backend.TimeRange{})
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	if len(resp.Frames) != 2 {
		t.Fatalf("expected 2 frames (sensors matching ^CPU Load with a 'Total' channel), got %d", len(resp.Frames))
	}

	labelSets := map[string]bool{}
	for _, frame := range resp.Frames {
		valueField := fieldByName(t, frame, "value")
		labelSets[valueField.Labels.String()] = true
	}
	if len(labelSets) != 2 {
		t.Fatalf("expected 2 distinct label sets across frames, got %d: %v", len(labelSets), labelSets)
	}
}

func TestQueryData_RegexFanOut_InvalidSensorPattern(t *testing.T) {
	ds := newTestDatasource(t, newFakePRTGServer())
	resp := runSingleQuery(t, ds, queryModel{QueryType: "current", UseRegex: true, SensorPattern: "("}, backend.TimeRange{})
	if resp.Error == nil {
		t.Fatal("expected an error for an invalid sensor pattern")
	}
	if resp.Status != backend.StatusBadRequest {
		t.Fatalf("expected StatusBadRequest, got %v", resp.Status)
	}
}

func rfc3339(t *testing.T, ts time.Time) interface{} { return ts.UTC().Format(time.RFC3339) }

func TestQueryData_TimeSeries_NoticeSeverity(t *testing.T) {
	base := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name         string
		span         time.Duration
		window       string
		wantSeverity data.NoticeSeverity
	}{
		{name: "within short window, info notice", span: 10 * time.Hour, window: "short", wantSeverity: data.NoticeSeverityInfo},
		{name: "exceeds 365d, clamped to long, warning notice", span: 400 * 24 * time.Hour, window: "long", wantSeverity: data.NoticeSeverityWarning},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newFakePRTGServer()
			from := base
			to := base.Add(tt.span)
			s.ChannelsBySensor["s1"] = []prtg.ChannelInfo{{ID: "s1.0", Name: "Total"}}
			s.TimeSeries["s1/"+tt.window] = [][]interface{}{
				{"time", "s1.s1.0"},
				{rfc3339(t, from), 1.0},
				{rfc3339(t, from.Add(tt.span/2)), 2.0},
			}
			ds := newTestDatasource(t, s)

			resp := runSingleQuery(t, ds, queryModel{QueryType: "timeseries", SensorID: "s1", ChannelID: "s1.0"}, backend.TimeRange{From: from, To: to})
			if resp.Error != nil {
				t.Fatalf("unexpected error: %v", resp.Error)
			}
			if len(resp.Frames) != 1 {
				t.Fatalf("expected 1 frame, got %d", len(resp.Frames))
			}

			frame := resp.Frames[0]
			if frame.Meta == nil || len(frame.Meta.Notices) != 1 {
				t.Fatalf("expected exactly 1 notice, got %+v", frame.Meta)
			}
			if frame.Meta.Notices[0].Severity != tt.wantSeverity {
				t.Fatalf("expected severity %v, got %v (%s)", tt.wantSeverity, frame.Meta.Notices[0].Severity, frame.Meta.Notices[0].Text)
			}

			timeField := fieldByName(t, frame, "time")
			if timeField.Len() != 2 {
				t.Fatalf("expected 2 trimmed rows, got %d", timeField.Len())
			}
		})
	}
}

func TestQueryData_UnknownQueryTypeDefaultsToCurrent(t *testing.T) {
	s := newFakePRTGServer()
	value := 1.0
	s.ChannelsBySensor["s1"] = []prtg.ChannelInfo{{ID: "s1.0", Name: "Total", LastMeasurement: prtg.Measurement{Value: &value}}}
	ds := newTestDatasource(t, s)

	resp := runSingleQuery(t, ds, queryModel{SensorID: "s1", ChannelID: "s1.0"}, backend.TimeRange{})
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	if len(resp.Frames) != 1 {
		t.Fatalf("expected 1 current-value frame by default, got %d", len(resp.Frames))
	}
	if resp.Frames[0].Meta != nil && len(resp.Frames[0].Meta.Notices) != 0 {
		t.Fatalf("did not expect a timeseries notice on a current-value frame")
	}
}
