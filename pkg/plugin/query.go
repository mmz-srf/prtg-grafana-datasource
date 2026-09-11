package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/srgssr/prtg-datasource/pkg/prtg"
)

// queryModel is the Go-side mirror of the unified frontend<->backend query
// contract (PrtgQuery in src/types.ts). *Name fields (groupName, deviceName,
// ...) are frontend-only display labels for a picker that hasn't finished
// loading yet -- they're intentionally absent here and ignored by
// json.Unmarshal if present.
type queryModel struct {
	QueryType string `json:"queryType"`
	UseRegex  bool   `json:"useRegex,omitempty"`

	GroupID   string `json:"groupId,omitempty"`
	DeviceID  string `json:"deviceId,omitempty"`
	SensorID  string `json:"sensorId,omitempty"`
	ChannelID string `json:"channelId,omitempty"`

	SensorPattern    string `json:"sensorPattern,omitempty"`
	ChannelMatchMode string `json:"channelMatchMode,omitempty"`
	ChannelPattern   string `json:"channelPattern,omitempty"`
}

const (
	queryTypeCurrent    = "current"
	queryTypeTimeseries = "timeseries"
)

// errNotFound tags a plugin-level "we looked and it's not there" error
// (e.g. a query's channelId doesn't exist on its sensor) so errToDataResponse
// can map it to backend.StatusNotFound instead of a generic 500, the same
// way it maps a parsed *prtg.APIError.
var errNotFound = errors.New("prtg: not found")

// maxSensorsPerQuery/maxChannelsPerSensor bound a single regex query's
// fan-out, to keep one dashboard panel from being able to trigger an
// unbounded number of upstream PRTG requests.
const (
	maxSensorsPerQuery   = 200
	maxChannelsPerSensor = 200
)

// queryTarget is one resolved (sensor, channel) pair a query fans out to --
// one Grafana data.Frame per target.
type queryTarget struct {
	SensorID string
	Channel  prtg.ChannelInfo
}

// QueryData handles multiple queries and returns multiple responses, one
// per incoming query (keyed by RefID, as required by the
// backend.QueryDataHandler contract).
func (d *Datasource) QueryData(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	response := backend.NewQueryDataResponse()
	for _, q := range req.Queries {
		response.Responses[q.RefID] = d.handleQuery(ctx, q)
	}
	return response, nil
}

func (d *Datasource) handleQuery(ctx context.Context, query backend.DataQuery) backend.DataResponse {
	var qm queryModel
	if err := json.Unmarshal(query.JSON, &qm); err != nil {
		return backend.ErrDataResponse(backend.StatusBadRequest, fmt.Sprintf("invalid query JSON: %s", err.Error()))
	}

	targets, err := d.resolveTargets(ctx, qm)
	if err != nil {
		return errToDataResponse(err)
	}

	var response backend.DataResponse
	for _, t := range targets {
		var frame *data.Frame
		var ferr error

		if qm.QueryType == queryTypeTimeseries {
			frame, ferr = d.buildTimeSeriesFrame(ctx, query, t)
		} else {
			frame, ferr = buildCurrentValueFrame(t)
		}
		if ferr != nil {
			return errToDataResponse(ferr)
		}
		response.Frames = append(response.Frames, frame)
	}
	return response
}

// resolveTargets fans a query out into one target per (sensor, channel)
// pair it addresses: a single pair for a fully-specified hierarchy query, or
// N pairs for a regex query (or a hierarchy query with channelId=="*").
func (d *Datasource) resolveTargets(ctx context.Context, qm queryModel) ([]queryTarget, error) {
	if qm.UseRegex {
		return d.resolveRegexTargets(ctx, qm)
	}
	return d.resolveHierarchyTargets(ctx, qm)
}

func (d *Datasource) resolveHierarchyTargets(ctx context.Context, qm queryModel) ([]queryTarget, error) {
	if qm.SensorID == "" {
		// Incomplete query (e.g. mid-edit). The frontend's filterQuery
		// gates this from ever being sent, but defensively return an empty,
		// non-error result rather than fail the panel.
		return nil, nil
	}

	if qm.ChannelID == "" || qm.ChannelID == "*" {
		channels, err := prtg.FetchAll[prtg.ChannelInfo](ctx, d.client, "/experimental/channels", prtg.FetchAllOptions{
			Filter:   prtg.ParentID(qm.SensorID).String(),
			MaxItems: maxChannelsPerSensor,
		})
		if err != nil {
			return nil, err
		}
		targets := make([]queryTarget, 0, len(channels.Items))
		for _, ch := range channels.Items {
			targets = append(targets, queryTarget{SensorID: qm.SensorID, Channel: ch})
		}
		return targets, nil
	}

	page, err := prtg.FetchPage[prtg.ChannelInfo](ctx, d.client, "/experimental/channels",
		prtg.And(prtg.ParentID(qm.SensorID), prtg.Eq("id", qm.ChannelID)).String(), "", 0, 1)
	if err != nil {
		return nil, err
	}
	if len(page.Items) == 0 {
		return nil, fmt.Errorf("%w: channel %q not found on sensor %q", errNotFound, qm.ChannelID, qm.SensorID)
	}
	return []queryTarget{{SensorID: qm.SensorID, Channel: page.Items[0]}}, nil
}

func (d *Datasource) resolveRegexTargets(ctx context.Context, qm queryModel) ([]queryTarget, error) {
	if qm.SensorPattern == "" {
		return nil, nil
	}

	sensorRe, err := regexp.Compile(qm.SensorPattern)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid sensor pattern: %v", prtg.ErrInvalidPattern, err)
	}

	channelMatches, err := prtg.CompileChannelMatcher(qm.ChannelMatchMode, qm.ChannelPattern)
	if err != nil {
		return nil, err
	}

	var scope prtg.Filter
	switch {
	case qm.DeviceID != "":
		scope = prtg.ParentID(qm.DeviceID)
	case qm.GroupID != "":
		scope = prtg.AncestorID(qm.GroupID)
	}

	sensors, err := prtg.FetchAll[prtg.SensorInfo](ctx, d.client, "/experimental/sensors", prtg.FetchAllOptions{
		Filter:   scope.String(),
		MaxItems: maxSensorsPerQuery,
	})
	if err != nil {
		return nil, err
	}

	var targets []queryTarget
	for _, s := range sensors.Items {
		if !sensorRe.MatchString(s.Name) {
			continue
		}

		channels, err := prtg.FetchAll[prtg.ChannelInfo](ctx, d.client, "/experimental/channels", prtg.FetchAllOptions{
			Filter:   prtg.ParentID(s.ID).String(),
			MaxItems: maxChannelsPerSensor,
		})
		if err != nil {
			return nil, err
		}

		for _, ch := range channels.Items {
			if !channelMatches(ch.Name) {
				continue
			}
			targets = append(targets, queryTarget{SensorID: s.ID, Channel: ch})
		}
	}
	return targets, nil
}

// labelsFromPath turns a ChannelInfo.Path breadcrumb (root -> ... ->
// channel) into Grafana series labels keyed by object type (e.g. "probe",
// "group", "device", "sensor", "channel"), per the plan's guidance that
// Path is "perfect for series labels".
func labelsFromPath(path []prtg.ReferencedObject) data.Labels {
	labels := data.Labels{}
	for _, ref := range path {
		simpleType := ref.SimpleType()
		if simpleType == "" || ref.Name == "" {
			continue
		}
		labels[simpleType] = ref.Name
	}
	return labels
}

// parseMeasurementTime parses a ChannelInfo.LastMeasurement.Timestamp
// (expected ISO-8601/RFC3339); falls back to time.Now() if it doesn't
// parse, so a "current value" frame still gets a sane, recent timestamp
// rather than the zero time.
func parseMeasurementTime(ts string) time.Time {
	if t, err := time.Parse(time.RFC3339, ts); err == nil {
		return t
	}
	return time.Now()
}

// buildCurrentValueFrame builds the "current value" frame for one
// (sensor, channel) target, using ChannelInfo.LastMeasurement directly --
// no separate call needed, per plan §1.
func buildCurrentValueFrame(t queryTarget) (*data.Frame, error) {
	ch := t.Channel
	labels := labelsFromPath(ch.Path)

	frame := data.NewFrame(ch.Name,
		data.NewField("time", nil, []time.Time{parseMeasurementTime(ch.LastMeasurement.Timestamp)}),
		data.NewField("value", labels, []*float64{ch.LastMeasurement.Value}),
	)
	return frame, nil
}

// buildTimeSeriesFrame builds the "historic time series" frame for one
// (sensor, channel) target: it picks the smallest fixed PRTG window
// covering the query's requested range, fetches it, trims it back down to
// that range, and attaches a Notice explaining the fixed-window resolution
// mismatch (warning severity if the range had to be clamped to the longest
// window, info otherwise).
func (d *Datasource) buildTimeSeriesFrame(ctx context.Context, query backend.DataQuery, t queryTarget) (*data.Frame, error) {
	from, to := query.TimeRange.From, query.TimeRange.To
	window, clamped := prtg.SelectWindow(from, to)

	channelKey := t.SensorID + "." + t.Channel.ID
	result, err := d.client.FetchTimeSeries(ctx, t.SensorID, window, []string{channelKey})
	if err != nil {
		return nil, err
	}

	trimmed := result.TrimToRange(from, to)
	values := make([]*float64, len(trimmed.Times))
	if len(trimmed.Values) > 0 {
		values = trimmed.Values[0]
	}

	labels := labelsFromPath(t.Channel.Path)
	frame := data.NewFrame(t.Channel.Name,
		data.NewField("time", nil, trimmed.Times),
		data.NewField("value", labels, values),
	)

	severity := data.NoticeSeverityInfo
	text := fmt.Sprintf(
		"PRTG API v2 only supports fixed historic-data windows (live=4h, short=2d, medium=60d, long=365d); used the %q window and trimmed to the requested range.",
		window,
	)
	if clamped {
		severity = data.NoticeSeverityWarning
		text += " The requested range exceeds 365 days, PRTG's longest available window, so results are clamped to it."
	}
	frame.AppendNotices(data.Notice{Severity: severity, Text: text})

	return frame, nil
}

// errToDataResponse maps an error from the prtg package to a
// backend.DataResponse, preferring a parsed *prtg.APIError's code/message
// over generic "HTTP nnn" text.
func errToDataResponse(err error) backend.DataResponse {
	if errors.Is(err, prtg.ErrInvalidPattern) {
		return backend.ErrDataResponse(backend.StatusBadRequest, err.Error())
	}
	if errors.Is(err, errNotFound) {
		return backend.ErrDataResponse(backend.StatusNotFound, err.Error())
	}

	apiErr, ok := prtg.AsAPIError(err)
	if !ok {
		return backend.ErrDataResponse(backend.StatusInternal, err.Error())
	}

	// Anything else (e.g. 503 SERVICE_UNAVAILABLE/LICENSE_INACTIVE) falls
	// through to StatusInternal, still carrying PRTG's own code/message
	// rather than generic "HTTP nnn" text (set below).
	status := backend.StatusInternal
	switch {
	case apiErr.IsUnauthorized():
		status = backend.StatusUnauthorized
	case apiErr.IsForbidden():
		status = backend.StatusForbidden
	case apiErr.IsNotFound():
		status = backend.StatusNotFound
	case apiErr.IsBadRequest():
		status = backend.StatusBadRequest
	case apiErr.IsTimeout():
		status = backend.StatusTimeout
	}

	message := apiErr.Message
	if apiErr.Code != "" {
		message = fmt.Sprintf("%s: %s", apiErr.Code, apiErr.Message)
	}
	return backend.ErrDataResponse(status, message)
}
