package prtg

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestSelectWindow(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		span        time.Duration
		wantWindow  Window
		wantClamped bool
	}{
		{name: "zero span", span: 0, wantWindow: WindowLive, wantClamped: false},
		{name: "exactly 4h -> live", span: 4 * time.Hour, wantWindow: WindowLive, wantClamped: false},
		{name: "just over 4h -> short", span: 4*time.Hour + time.Second, wantWindow: WindowShort, wantClamped: false},
		{name: "exactly 2d -> short", span: 2 * 24 * time.Hour, wantWindow: WindowShort, wantClamped: false},
		{name: "just over 2d -> medium", span: 2*24*time.Hour + time.Second, wantWindow: WindowMedium, wantClamped: false},
		{name: "exactly 60d -> medium", span: 60 * 24 * time.Hour, wantWindow: WindowMedium, wantClamped: false},
		{name: "just over 60d -> long", span: 60*24*time.Hour + time.Second, wantWindow: WindowLong, wantClamped: false},
		{name: "exactly 365d -> long", span: 365 * 24 * time.Hour, wantWindow: WindowLong, wantClamped: false},
		{name: "just over 365d -> long, clamped", span: 365*24*time.Hour + time.Second, wantWindow: WindowLong, wantClamped: true},
		{name: "way over 365d -> long, clamped", span: 1000 * 24 * time.Hour, wantWindow: WindowLong, wantClamped: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			window, clamped := SelectWindow(base, base.Add(tt.span))
			if window != tt.wantWindow {
				t.Fatalf("expected window %q, got %q", tt.wantWindow, window)
			}
			if clamped != tt.wantClamped {
				t.Fatalf("expected clamped=%v, got %v", tt.wantClamped, clamped)
			}
		})
	}
}

func f64(v float64) *float64 { return &v }

func TestFetchTimeSeries_ParsesRowsDefensively(t *testing.T) {
	fixture, err := os.ReadFile("testdata/timeseries_with_gap.json")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	var gotPath string
	handler := func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(fixture)
	}
	ts := httptest.NewServer(http.HandlerFunc(handler))
	defer ts.Close()

	base, err := ParseServerURL(ts.URL)
	if err != nil {
		t.Fatalf("ParseServerURL: %v", err)
	}
	c, err := NewClient(base, ts.Client(), &fakeAuthenticator{tokens: []string{"tok"}})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	result, err := c.FetchTimeSeries(context.Background(), "3074", WindowShort, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotPath != "/api/v2/experimental/timeseries/3074/short" {
		t.Fatalf("unexpected request path: %q", gotPath)
	}

	if got, want := result.ChannelIDs, []string{"3074.1", "3074.2"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("unexpected ChannelIDs: %v", got)
	}
	if len(result.Times) != 5 {
		t.Fatalf("expected 5 rows, got %d", len(result.Times))
	}

	wantCol0 := []*float64{f64(12.5), nil, nil, f64(14.25), f64(15.0)}
	wantCol1 := []*float64{f64(100), f64(101), f64(102), nil, f64(103)}

	assertFloatPtrSlice(t, "column 0 (3074.1)", result.Values[0], wantCol0)
	assertFloatPtrSlice(t, "column 1 (3074.2)", result.Values[1], wantCol1)
}

func assertFloatPtrSlice(t *testing.T, label string, got, want []*float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: expected %d values, got %d", label, len(want), len(got))
	}
	for i := range want {
		switch {
		case want[i] == nil && got[i] != nil:
			t.Fatalf("%s[%d]: expected nil (gap), got %v", label, i, *got[i])
		case want[i] != nil && got[i] == nil:
			t.Fatalf("%s[%d]: expected %v, got nil (gap)", label, i, *want[i])
		case want[i] != nil && got[i] != nil && *want[i] != *got[i]:
			t.Fatalf("%s[%d]: expected %v, got %v", label, i, *want[i], *got[i])
		}
	}
}

func TestFetchTimeSeries_ChannelsQueryParam(t *testing.T) {
	var gotQuery string
	handler := func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[["time"]]`))
	}
	ts := httptest.NewServer(http.HandlerFunc(handler))
	defer ts.Close()

	base, _ := ParseServerURL(ts.URL)
	c, _ := NewClient(base, ts.Client(), &fakeAuthenticator{tokens: []string{"tok"}})

	if _, err := c.FetchTimeSeries(context.Background(), "3074", WindowLive, []string{"3074.1", "3074.2"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotQuery != "channels=3074.1%2C3074.2" {
		t.Fatalf("unexpected query: %q", gotQuery)
	}

	if _, err := c.FetchTimeSeries(context.Background(), "3074", WindowLive, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotQuery != "" {
		t.Fatalf("expected no query params when channelIDs is empty, got %q", gotQuery)
	}
}

func TestFetchTimeSeries_RequiresSensorID(t *testing.T) {
	base, _ := ParseServerURL("https://prtg.example.com")
	c, _ := NewClient(base, http.DefaultClient, &fakeAuthenticator{tokens: []string{"tok"}})
	if _, err := c.FetchTimeSeries(context.Background(), "", WindowLive, nil); err == nil {
		t.Fatal("expected an error for an empty sensorID")
	}
}

func TestParseTimeSeriesRows_EmptyResponse(t *testing.T) {
	result, err := parseTimeSeriesRows(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.ChannelIDs) != 0 || len(result.Times) != 0 {
		t.Fatalf("expected empty result, got %+v", result)
	}
}

func TestParseTimeSeriesRows_EmptyHeaderRowErrors(t *testing.T) {
	rows := mustDecodeRows(t, `[[]]`)
	if _, err := parseTimeSeriesRows(rows); err == nil {
		t.Fatal("expected an error for an empty header row")
	}
}

func TestParseTimeSeriesRows_NonStringHeaderCellErrors(t *testing.T) {
	rows := mustDecodeRows(t, `[["time", 123]]`)
	if _, err := parseTimeSeriesRows(rows); err == nil {
		t.Fatal("expected an error for a non-string header cell")
	}
}

func TestParseTimeSeriesRows_UnparseableTimestampKeepsColumnAlignment(t *testing.T) {
	rows := mustDecodeRows(t, `[["time", "1.1"], ["not-a-timestamp", 5]]`)
	result, err := parseTimeSeriesRows(rows)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Times) != 1 || !result.Times[0].IsZero() {
		t.Fatalf("expected a single zero-value time, got %v", result.Times)
	}
	if len(result.Values[0]) != 1 || result.Values[0][0] == nil || *result.Values[0][0] != 5 {
		t.Fatalf("expected value 5 preserved despite bad timestamp, got %v", result.Values[0])
	}
}

func mustDecodeRows(t *testing.T, jsonBody string) [][]json.RawMessage {
	t.Helper()
	var rows [][]json.RawMessage
	if err := json.Unmarshal([]byte(jsonBody), &rows); err != nil {
		t.Fatalf("decoding test rows: %v", err)
	}
	return rows
}

func TestTrimToRange(t *testing.T) {
	t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	result := TimeSeriesResult{
		ChannelIDs: []string{"a", "b"},
		Times: []time.Time{
			t0,
			t0.Add(1 * time.Hour),
			t0.Add(2 * time.Hour),
			t0.Add(3 * time.Hour),
		},
		Values: [][]*float64{
			{f64(1), f64(2), f64(3), f64(4)},
			{f64(10), f64(20), f64(30), f64(40)},
		},
	}

	trimmed := result.TrimToRange(t0.Add(1*time.Hour), t0.Add(2*time.Hour))
	if len(trimmed.Times) != 2 {
		t.Fatalf("expected 2 rows in range, got %d", len(trimmed.Times))
	}
	if !trimmed.Times[0].Equal(t0.Add(1*time.Hour)) || !trimmed.Times[1].Equal(t0.Add(2*time.Hour)) {
		t.Fatalf("unexpected trimmed times: %v", trimmed.Times)
	}
	assertFloatPtrSlice(t, "trimmed column a", trimmed.Values[0], []*float64{f64(2), f64(3)})
	assertFloatPtrSlice(t, "trimmed column b", trimmed.Values[1], []*float64{f64(20), f64(30)})
}

func TestTrimToRange_NoRowsInRange(t *testing.T) {
	t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	result := TimeSeriesResult{
		ChannelIDs: []string{"a"},
		Times:      []time.Time{t0},
		Values:     [][]*float64{{f64(1)}},
	}
	trimmed := result.TrimToRange(t0.Add(1*time.Hour), t0.Add(2*time.Hour))
	if len(trimmed.Times) != 0 {
		t.Fatalf("expected 0 rows, got %d", len(trimmed.Times))
	}
	if len(trimmed.Values[0]) != 0 {
		t.Fatalf("expected 0 values, got %d", len(trimmed.Values[0]))
	}
}
