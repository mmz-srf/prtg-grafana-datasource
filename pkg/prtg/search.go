package prtg

import (
	"context"
	"errors"
	"fmt"
	"regexp"
)

// Channel match modes, mirroring the frontend's PrtgChannelMatchMode.
const (
	ChannelMatchModeExact = "exact"
	ChannelMatchModeRegex = "regex"
)

// maxSensorScan bounds how many sensors SearchSensors will fetch and
// name-match against, protecting both PRTG (a huge/unscoped search) and the
// plugin (an unbounded live-typing preview) from a runaway query.
const maxSensorScan = 500

// maxSensorsWithChannels bounds how many *name-matched* sensors
// SearchSensors will go on to do the more expensive per-sensor channel
// fetch+filter for. This is the cost-control cap the plan calls out
// specifically for the live MatchPreview typing experience behind
// CallResource's "sensors/search" route; QueryData's own regex fan-out
// (pkg/plugin/query.go) does not reuse this cap, since a saved dashboard
// query should reliably return the same set of frames every run rather than
// an arbitrary first-N cutoff.
const maxSensorsWithChannels = 50

// ErrInvalidPattern wraps a regexp.Compile failure for either the sensor or
// channel pattern, so callers (e.g. the CallResource route) can distinguish
// a bad-input 400 from a genuine backend/upstream error.
var ErrInvalidPattern = errors.New("prtg: invalid pattern")

// CompileChannelMatcher returns a predicate for whether a channel name
// matches pattern under mode. An empty pattern always matches (no
// channel-level filter -- every channel of a name-matched sensor counts).
// mode == ChannelMatchModeRegex compiles pattern as a Go (RE2) regular
// expression; any other value (including "", the documented default) does
// an exact string match.
func CompileChannelMatcher(mode, pattern string) (func(name string) bool, error) {
	if pattern == "" {
		return func(string) bool { return true }, nil
	}
	if mode == ChannelMatchModeRegex {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid channel pattern: %v", ErrInvalidPattern, err)
		}
		return re.MatchString, nil
	}
	return func(name string) bool { return name == pattern }, nil
}

// ChannelMatch is one channel that matched a SearchSensors channel
// criterion.
type ChannelMatch struct {
	ID   string
	Name string
}

// SensorSearchMatch is one sensor that matched a SearchSensors sensor-name
// pattern, together with the subset of its channels that also matched the
// channel criteria.
type SensorSearchMatch struct {
	ID              string
	Name            string
	DeviceID        string
	DeviceName      string
	MatchedChannels []ChannelMatch
}

// SearchSensorsResult is SearchSensors' return value, mirroring the
// unified frontend<->backend contract's sensors/search response shape.
type SearchSensorsResult struct {
	Sensors       []SensorSearchMatch
	TotalSensors  int
	TotalChannels int
	// Truncated is true if the result may be incomplete: either the sensor
	// scan itself hit maxSensorScan, or more than maxSensorsWithChannels
	// sensors matched the name pattern (so only the first
	// maxSensorsWithChannels got the channel-fetch treatment).
	Truncated bool
}

// SearchSensorsParams are SearchSensors' inputs, mirroring the
// GET sensors/search resource route's query parameters.
type SearchSensorsParams struct {
	// Pattern is a Go regexp matched against sensor names.
	Pattern string
	// GroupID, if set (and DeviceID is not), scopes the search to sensors
	// descending from this group/probe at any depth.
	GroupID string
	// DeviceID, if set, scopes the search to this device's direct sensors,
	// taking precedence over GroupID.
	DeviceID string
	// ChannelMatchMode is "exact" (default) or "regex".
	ChannelMatchMode string
	// ChannelPattern is an exact channel name, or a regex per
	// ChannelMatchMode; empty means "match every channel".
	ChannelPattern string
}

// SearchSensors implements the "glue step" the unified contract calls out
// explicitly: it matches sensors by name (params.Pattern, scoped by
// GroupID/DeviceID if given), then -- for each matched sensor, up to
// maxSensorsWithChannels of them -- fetches that sensor's channels and
// filters them by ChannelMatchMode/ChannelPattern, returning only sensors
// that end up with at least one matched channel.
func SearchSensors(ctx context.Context, c *Client, params SearchSensorsParams) (SearchSensorsResult, error) {
	nameRe, err := regexp.Compile(params.Pattern)
	if err != nil {
		return SearchSensorsResult{}, fmt.Errorf("%w: invalid sensor pattern: %v", ErrInvalidPattern, err)
	}

	channelMatches, err := CompileChannelMatcher(params.ChannelMatchMode, params.ChannelPattern)
	if err != nil {
		return SearchSensorsResult{}, err
	}

	var scope Filter
	switch {
	case params.DeviceID != "":
		scope = ParentID(params.DeviceID)
	case params.GroupID != "":
		scope = AncestorID(params.GroupID)
	}

	fetched, err := FetchAll[SensorInfo](ctx, c, "/experimental/sensors", FetchAllOptions{
		Filter:   scope.String(),
		PageSize: 100,
		MaxItems: maxSensorScan,
	})
	if err != nil {
		return SearchSensorsResult{}, err
	}

	var nameMatched []SensorInfo
	for _, s := range fetched.Items {
		if nameRe.MatchString(s.Name) {
			nameMatched = append(nameMatched, s)
		}
	}

	truncated := fetched.Truncated
	scanCount := len(nameMatched)
	if scanCount > maxSensorsWithChannels {
		truncated = true
		scanCount = maxSensorsWithChannels
	}

	result := SearchSensorsResult{}
	for _, s := range nameMatched[:scanCount] {
		channels, err := FetchAll[ChannelInfo](ctx, c, "/experimental/channels", FetchAllOptions{
			Filter: ParentID(s.ID).String(),
		})
		if err != nil {
			return SearchSensorsResult{}, err
		}

		var matched []ChannelMatch
		for _, ch := range channels.Items {
			if !channelMatches(ch.Name) {
				continue
			}
			matched = append(matched, ChannelMatch{ID: ch.ID, Name: ch.Name})
		}
		if len(matched) == 0 {
			continue
		}

		deviceID, deviceName := deviceFromPath(s.Path)
		result.Sensors = append(result.Sensors, SensorSearchMatch{
			ID:              s.ID,
			Name:            s.Name,
			DeviceID:        deviceID,
			DeviceName:      deviceName,
			MatchedChannels: matched,
		})
		result.TotalChannels += len(matched)
	}
	result.TotalSensors = len(result.Sensors)
	result.Truncated = truncated
	return result, nil
}

// deviceFromPath extracts the owning device's id/name from a sensor's Path
// breadcrumb (see ReferencedObject), avoiding an extra /devices call.
func deviceFromPath(path []ReferencedObject) (id, name string) {
	for _, ref := range path {
		if ref.SimpleType() == "device" {
			return ref.ID, ref.Name
		}
	}
	return "", ""
}
