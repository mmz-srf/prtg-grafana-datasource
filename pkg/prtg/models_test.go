package prtg

import "testing"

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
