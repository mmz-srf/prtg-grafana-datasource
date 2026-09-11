package prtg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Window is one of PRTG APIv2's fixed historic-data time windows. There is
// no arbitrary from/to and no avg/aggregation parameter for
// GET /experimental/timeseries/{sensorId}/{type} -- resolution and range are
// entirely dictated by PRTG. The deprecated, being-removed
// GET /experimental/timeseries/{id} (no {type}, arbitrary from/to) is
// intentionally not used anywhere in this package.
type Window string

const (
	WindowLive   Window = "live"   // last 4 hours
	WindowShort  Window = "short"  // last 2 days
	WindowMedium Window = "medium" // last 60 days
	WindowLong   Window = "long"   // last 365 days
)

// windowDurations gives each Window's fixed span, per PRTG APIv2 docs.
var windowDurations = map[Window]time.Duration{
	WindowLive:   4 * time.Hour,
	WindowShort:  2 * 24 * time.Hour,
	WindowMedium: 60 * 24 * time.Hour,
	WindowLong:   365 * 24 * time.Hour,
}

// SelectWindow picks the smallest fixed PRTG APIv2 window (live -> short ->
// medium -> long) whose span covers [from, to]. If the requested range
// exceeds even the longest window (365 days), it clamps to WindowLong and
// reports clamped=true so the caller can attach a warning-severity notice
// (as opposed to an informational one for a normal, unclamped window pick).
func SelectWindow(from, to time.Time) (window Window, clamped bool) {
	span := to.Sub(from)
	switch {
	case span <= windowDurations[WindowLive]:
		return WindowLive, false
	case span <= windowDurations[WindowShort]:
		return WindowShort, false
	case span <= windowDurations[WindowMedium]:
		return WindowMedium, false
	case span <= windowDurations[WindowLong]:
		return WindowLong, false
	default:
		return WindowLong, true
	}
}

// TimeSeriesResult is the parsed, defensively-decoded result of a
// GET /experimental/timeseries/{sensorId}/{window} call.
type TimeSeriesResult struct {
	// ChannelIDs are the "<sensorId>.<channelId>" column headers, in column
	// order, taken from row 0 of the response (excluding the leading "time"
	// column).
	ChannelIDs []string
	// Times holds the parsed timestamp of every data row, in row order. A
	// row whose timestamp cell didn't parse as RFC3339 gets the zero
	// time.Time rather than being dropped, to keep this slice's indices
	// aligned with Values' inner slices.
	Times []time.Time
	// Values holds one []*float64 per channel column (len(Values) ==
	// len(ChannelIDs)); Values[col][row] is nil when the corresponding cell
	// didn't parse as a number (null, a non-numeric string, or a missing
	// trailing cell in a short row) and is treated as a gap.
	//
	// This defensive stance is deliberate, not a placeholder: PRTG's own
	// OpenAPI spec documents a timeseries row gap using non-JSON pseudocode
	// (`["...Z", // gap start]`) and a formal schema
	// (`anyOf[Timestamp, ChannelID, Integer, Float]`) loose enough not to
	// pin down a single concrete gap shape. See plan §1.
	Values [][]*float64
}

// FetchTimeSeries fetches one fixed window of historic data for a sensor.
// channelIDs, if non-empty, is passed as the `channels` query parameter
// (each entry already in "<sensorId>.<channelId>" form) to limit the
// response to specific channels; if empty, PRTG returns all of the sensor's
// channels.
func (c *Client) FetchTimeSeries(ctx context.Context, sensorID string, window Window, channelIDs []string) (TimeSeriesResult, error) {
	if sensorID == "" {
		return TimeSeriesResult{}, errors.New("prtg: sensorID is required")
	}

	query := url.Values{}
	if len(channelIDs) > 0 {
		query.Set("channels", strings.Join(channelIDs, ","))
	}

	path := fmt.Sprintf("/experimental/timeseries/%s/%s", url.PathEscape(sensorID), url.PathEscape(string(window)))

	var rows [][]json.RawMessage
	if _, err := c.do(ctx, http.MethodGet, path, query, &rows); err != nil {
		return TimeSeriesResult{}, err
	}
	return parseTimeSeriesRows(rows)
}

func parseTimeSeriesRows(rows [][]json.RawMessage) (TimeSeriesResult, error) {
	if len(rows) == 0 {
		return TimeSeriesResult{}, nil
	}

	header := rows[0]
	if len(header) == 0 {
		return TimeSeriesResult{}, errors.New("prtg: timeseries response has an empty header row")
	}

	channelIDs := make([]string, 0, len(header)-1)
	for _, cell := range header[1:] {
		var s string
		if err := json.Unmarshal(cell, &s); err != nil {
			return TimeSeriesResult{}, fmt.Errorf("prtg: timeseries header cell is not a string: %w", err)
		}
		channelIDs = append(channelIDs, s)
	}

	result := TimeSeriesResult{
		ChannelIDs: channelIDs,
		Times:      make([]time.Time, 0, len(rows)-1),
		Values:     make([][]*float64, len(channelIDs)),
	}
	for i := range result.Values {
		result.Values[i] = make([]*float64, 0, len(rows)-1)
	}

	for _, row := range rows[1:] {
		if len(row) == 0 {
			continue // defensively skip a totally empty row
		}

		var t time.Time
		var ts string
		if err := json.Unmarshal(row[0], &ts); err == nil {
			if parsed, err := time.Parse(time.RFC3339, ts); err == nil {
				t = parsed
			}
		}
		result.Times = append(result.Times, t)

		for col := 0; col < len(channelIDs); col++ {
			var v *float64
			if col+1 < len(row) {
				v = parseNumericCell(row[col+1])
			}
			result.Values[col] = append(result.Values[col], v)
		}
	}
	return result, nil
}

// parseNumericCell attempts to decode raw as a float64, returning nil (a
// gap) for anything that doesn't parse -- null, a non-numeric string, a
// bool, etc.
//
// Unmarshaling is deliberately done into a *float64 (rather than a plain
// float64): encoding/json treats a JSON null unmarshaled into a non-pointer
// target as a documented no-op (no error, value left unchanged at its
// zero value) -- which would otherwise turn a genuine gap into a
// misleading 0. A pointer target gets the standard, correct null handling:
// the pointer itself is set to nil.
func parseNumericCell(raw json.RawMessage) *float64 {
	var v *float64
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil
	}
	return v
}

// TrimToRange filters r's rows down to [from, to] inclusive, since PRTG
// always returns the entire fixed window regardless of the narrower range
// Grafana actually asked for.
func (r TimeSeriesResult) TrimToRange(from, to time.Time) TimeSeriesResult {
	trimmed := TimeSeriesResult{
		ChannelIDs: r.ChannelIDs,
		Times:      make([]time.Time, 0, len(r.Times)),
		Values:     make([][]*float64, len(r.Values)),
	}
	for i := range trimmed.Values {
		trimmed.Values[i] = make([]*float64, 0, len(r.Times))
	}

	for i, t := range r.Times {
		if t.Before(from) || t.After(to) {
			continue
		}
		trimmed.Times = append(trimmed.Times, t)
		for col := range r.Values {
			trimmed.Values[col] = append(trimmed.Values[col], r.Values[col][i])
		}
	}
	return trimmed
}
