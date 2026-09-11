package prtg

import "testing"

func TestFilter_Basics(t *testing.T) {
	tests := []struct {
		name string
		f    Filter
		want string
	}{
		{name: "eq", f: Eq("id", "2004"), want: `id = "2004"`},
		{name: "not eq", f: NotEq("status", "up"), want: `status != "up"`},
		{name: "contains", f: Contains("tags", "ping"), want: `tags contains "ping"`},
		{name: "matches", f: Matches("name", "^CPU.*"), want: `name matches "^CPU.*"`},
		{name: "type eq", f: TypeEq("sensor"), want: `type = "sensor"`},
		{name: "parent id", f: ParentID("2004"), want: `parentid = "2004"`},
		{name: "ancestor id", f: AncestorID("1000"), want: `ancestors.id = "1000"`},
		{name: "zero", f: Filter{}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.f.String(); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestFilter_IsZero(t *testing.T) {
	if !(Filter{}).IsZero() {
		t.Fatal("expected zero Filter to be IsZero()")
	}
	if Eq("id", "1").IsZero() {
		t.Fatal("expected non-zero Filter to not be IsZero()")
	}
}

func TestFilter_QuotingEscapesQuotesAndBackslashes(t *testing.T) {
	f := Eq("name", `weird "value" \with\ backslashes`)
	want := `name = "weird \"value\" \\with\\ backslashes"`
	if got := f.String(); got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestAnd(t *testing.T) {
	tests := []struct {
		name    string
		filters []Filter
		want    string
	}{
		{name: "no filters", filters: nil, want: ""},
		{name: "all zero", filters: []Filter{{}, {}}, want: ""},
		{name: "single", filters: []Filter{Eq("id", "1")}, want: `id = "1"`},
		{name: "single with zero mixed in", filters: []Filter{{}, Eq("id", "1"), {}}, want: `id = "1"`},
		{
			name:    "confirmed example",
			filters: []Filter{TypeEq("sensor"), Contains("tags", "ping"), ParentID("2004")},
			want:    `(type = "sensor" and tags contains "ping" and parentid = "2004")`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := And(tt.filters...).String(); got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestOr(t *testing.T) {
	got := Or(Eq("status", "up"), Eq("status", "warning")).String()
	want := `(status = "up" or status = "warning")`
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestNot(t *testing.T) {
	if got := Not(Filter{}).String(); got != "" {
		t.Fatalf("expected empty string for Not(zero), got %q", got)
	}
	if got := Not(Eq("status", "up")).String(); got != `not status = "up"` {
		t.Fatalf("unexpected Not() result: %q", got)
	}
}

func TestAndOrCompose(t *testing.T) {
	// And/Or nested inside each other should stay well-formed (each
	// multi-part sub-expression parenthesized).
	inner := Or(Eq("status", "warning"), Eq("status", "down"))
	got := And(TypeEq("sensor"), inner).String()
	want := `(type = "sensor" and (status = "warning" or status = "down"))`
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
