package prtg

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// ReferencedObject is a lightweight reference to another PRTG object, as
// used in `path` (and `channels`, for SensorInfo) reference-array fields
// across PRTG APIv2 responses. A `path` array runs from the root object down
// to the object itself, so it doubles as a ready-made breadcrumb for series
// labels (see plugin.labelsFromPath).
//
// Type is PRTG's raw ReferencedObjectType enum value (e.g. "REFERENCED_DEVICE",
// "REFERENCED_GROUP", "REFERENCED_SENSOR", "REFERENCED_CHANNEL", "REFERENCED_PROBE",
// "REFERENCED_ROOT", ...) -- use SimpleType() rather than comparing Type directly.
type ReferencedObject struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	Href string `json:"href,omitempty"`
}

// SimpleType normalizes Type into a short, lowercase noun ("device", "group",
// "sensor", "channel", "probe", "root", ...) by stripping PRTG's
// "REFERENCED_" enum prefix and lowercasing. Safe to call on a value that's
// already in that short form (e.g. from a test fixture or a future API
// version without the prefix) -- TrimPrefix is a no-op when the prefix isn't
// present, so it still normalizes to the same result.
func (r ReferencedObject) SimpleType() string {
	return strings.ToLower(strings.TrimPrefix(r.Type, "REFERENCED_"))
}

// GroupInfo mirrors PRTG APIv2's GroupInfo schema, returned by
// GET /experimental/groups. It is also structurally reused for
// GET /experimental/probes: probes and groups share the same confirmed
// field set, and the plugin's "groups" resource route treats both as a
// single, indistinguishable "group tier" (see plan §5 open item (a)).
type GroupInfo struct {
	ID               string             `json:"id"`
	Name             string             `json:"name"`
	Status           string             `json:"status"`
	Path             []ReferencedObject `json:"path"`
	Favorite         bool               `json:"favorite"`
	Priority         int                `json:"priority"`
	ScanningInterval string             `json:"scanning_interval"`
}

// DeviceInfo mirrors PRTG APIv2's DeviceInfo schema, returned by
// GET /experimental/devices.
type DeviceInfo struct {
	ID               string             `json:"id"`
	Name             string             `json:"name"`
	Status           string             `json:"status"`
	Path             []ReferencedObject `json:"path"`
	Favorite         bool               `json:"favorite"`
	Priority         int                `json:"priority"`
	ScanningInterval string             `json:"scanning_interval"`
}

// SensorInfo mirrors PRTG APIv2's SensorInfo schema, returned by
// GET /experimental/sensors.
type SensorInfo struct {
	ID               string             `json:"id"`
	Name             string             `json:"name"`
	Status           string             `json:"status"`
	Path             []ReferencedObject `json:"path"`
	Favorite         bool               `json:"favorite"`
	Priority         int                `json:"priority"`
	ScanningInterval string             `json:"scanning_interval"`
	// Channels may already embed the sensor's channel references, which can
	// save a separate /channels call per sensor when just browsing (not
	// currently relied upon by this package: query/search flows fetch full
	// ChannelInfo instead, since they need unit/last_measurement/path too).
	Channels    []ReferencedObject `json:"channels"`
	Message     string             `json:"message"`
	SensorKind  string             `json:"sensor_kind"`
	StatusSince string             `json:"status_since"`
}

// Unit describes a channel's display unit, as returned in
// ChannelInfo.Unit. Custom is a user-defined unit *label* (e.g. "widgets"),
// not a boolean flag -- confirmed against PRTG's ChannelInfo.yaml schema.
type Unit struct {
	DisplayUnit   string `json:"display_unit"`
	Type          string `json:"type"`
	DecimalDigits int    `json:"decimal_digits"`
	Custom        string `json:"custom,omitempty"`
}

// Measurement is a single value sample, used for
// ChannelInfo.LastMeasurement. Numeric fields are pointers so a null (e.g. a
// sensor that hasn't reported data yet) decodes cleanly instead of failing
// json.Unmarshal.
type Measurement struct {
	Value        *float64 `json:"value"`
	DisplayValue string   `json:"display_value"`
	Timestamp    string   `json:"timestamp"`
	Average      *float64 `json:"average"`
	Minimum      *float64 `json:"minimum"`
	Maximum      *float64 `json:"maximum"`
}

// UnmarshalJSON decodes a Measurement, tolerating PRTG APIv2 sending
// display_value as either an already-formatted string (the common case,
// e.g. "42.5 %") or a bare JSON number -- observed in production for some
// sensor/channel kinds (e.g. a "Disk Free: C:\" sensor's channels). A plain
// `string`-typed field would fail json.Unmarshal outright for the latter.
func (m *Measurement) UnmarshalJSON(data []byte) error {
	var alias struct {
		Value        *float64        `json:"value"`
		DisplayValue json.RawMessage `json:"display_value"`
		Timestamp    string          `json:"timestamp"`
		Average      *float64        `json:"average"`
		Minimum      *float64        `json:"minimum"`
		Maximum      *float64        `json:"maximum"`
	}
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}

	displayValue, err := decodeFlexibleString(alias.DisplayValue)
	if err != nil {
		return fmt.Errorf("prtg: decoding display_value: %w", err)
	}

	m.Value = alias.Value
	m.DisplayValue = displayValue
	m.Timestamp = alias.Timestamp
	m.Average = alias.Average
	m.Minimum = alias.Minimum
	m.Maximum = alias.Maximum
	return nil
}

// decodeFlexibleString decodes a JSON string, number, or null into a Go
// string, for fields PRTG APIv2 doesn't consistently send as one type.
func decodeFlexibleString(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, nil
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return strconv.FormatFloat(f, 'f', -1, 64), nil
	}
	return "", fmt.Errorf("unsupported JSON type %s", raw)
}

// Limits carries a channel's configured warning/error thresholds, as
// returned in ChannelInfo.Limits. Field names/tags are confirmed against
// PRTG's ChannelInfo.yaml schema (model.Limits).
type Limits struct {
	LimitMaxError   *float64 `json:"limit_max_error,omitempty"`
	LimitMinError   *float64 `json:"limit_min_error,omitempty"`
	LimitMaxWarning *float64 `json:"limit_max_warning,omitempty"`
	LimitMinWarning *float64 `json:"limit_min_warning,omitempty"`
	LimitErrorMsg   string   `json:"limit_error_msg,omitempty"`
	LimitWarningMsg string   `json:"limit_warning_msg,omitempty"`
}

// ChannelInfo mirrors PRTG APIv2's ChannelInfo schema, returned by
// GET /experimental/channels. LastMeasurement.DisplayValue + Unit.DisplayUnit
// is exactly the "current value" query mode -- no separate call needed.
type ChannelInfo struct {
	ID              string             `json:"id"`
	Name            string             `json:"name"`
	InternalName    string             `json:"internal_name"`
	Unit            Unit               `json:"unit"`
	LastMeasurement Measurement        `json:"last_measurement"`
	Path            []ReferencedObject `json:"path"`
	Limits          Limits             `json:"limits"`
	ManualColor     string             `json:"manual_color,omitempty"`
}
