package prtg

import (
	"encoding/json"
	"testing"
)

// TestReferencedObject_SimpleType locks in that SimpleType() correctly
// normalizes PRTG's real APIv2 enum values (the "REFERENCED_" prefix, as
// confirmed against the live OpenAPI spec) as well as an already-short form
// (as used by this package's own test fixtures), so both converge to the
// same lowercase noun used for series labels and hierarchy lookups.
func TestReferencedObject_SimpleType(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "real PRTG enum: device", in: "REFERENCED_DEVICE", want: "device"},
		{name: "real PRTG enum: group", in: "REFERENCED_GROUP", want: "group"},
		{name: "real PRTG enum: sensor", in: "REFERENCED_SENSOR", want: "sensor"},
		{name: "real PRTG enum: channel", in: "REFERENCED_CHANNEL", want: "channel"},
		{name: "real PRTG enum: probe", in: "REFERENCED_PROBE", want: "probe"},
		{name: "real PRTG enum: root", in: "REFERENCED_ROOT", want: "root"},
		{name: "already-short form (e.g. test fixtures)", in: "device", want: "device"},
		{name: "empty", in: "", want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ReferencedObject{Type: tc.in}.SimpleType()
			if got != tc.want {
				t.Errorf("SimpleType(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestMeasurement_UnmarshalJSON_DisplayValue locks in that display_value
// decodes whether PRTG sends it as an already-formatted string (the common
// case) or a bare JSON number -- observed in production for a real "Disk
// Free: C:\" sensor's channels, where a plain string-typed field failed
// json.Unmarshal outright ("cannot unmarshal number into Go struct field
// Measurement.last_measurement.display_value of type string").
func TestMeasurement_UnmarshalJSON_DisplayValue(t *testing.T) {
	tests := []struct {
		name string
		json string
		want string
	}{
		{name: "string (common case)", json: `{"value":42.5,"display_value":"42.5 %"}`, want: "42.5 %"},
		{name: "bare number (observed in production)", json: `{"value":74,"display_value":74}`, want: "74"},
		{name: "bare number, fractional", json: `{"value":1.5,"display_value":1.5}`, want: "1.5"},
		{name: "null", json: `{"value":null,"display_value":null}`, want: ""},
		{name: "missing", json: `{"value":null}`, want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var m Measurement
			if err := json.Unmarshal([]byte(tc.json), &m); err != nil {
				t.Fatalf("Unmarshal(%s) returned error: %v", tc.json, err)
			}
			if m.DisplayValue != tc.want {
				t.Errorf("DisplayValue = %q, want %q", m.DisplayValue, tc.want)
			}
		})
	}
}

// TestMeasurement_UnmarshalJSON_OtherFields locks in that the rest of
// Measurement's fields still decode correctly through the custom
// UnmarshalJSON (i.e. the display_value special-casing doesn't break
// anything else).
func TestMeasurement_UnmarshalJSON_OtherFields(t *testing.T) {
	var m Measurement
	raw := `{"value":42.5,"display_value":"42.5 %","timestamp":"2024-01-01T12:00:00Z","average":40.1,"minimum":10,"maximum":90}`
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("Unmarshal returned error: %v", err)
	}
	if m.Value == nil || *m.Value != 42.5 {
		t.Errorf("Value = %v, want 42.5", m.Value)
	}
	if m.Timestamp != "2024-01-01T12:00:00Z" {
		t.Errorf("Timestamp = %q, want %q", m.Timestamp, "2024-01-01T12:00:00Z")
	}
	if m.Average == nil || *m.Average != 40.1 {
		t.Errorf("Average = %v, want 40.1", m.Average)
	}
	if m.Minimum == nil || *m.Minimum != 10 {
		t.Errorf("Minimum = %v, want 10", m.Minimum)
	}
	if m.Maximum == nil || *m.Maximum != 90 {
		t.Errorf("Maximum = %v, want 90", m.Maximum)
	}
}
