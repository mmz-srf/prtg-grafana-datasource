package prtg

import (
	"fmt"
	"strings"
)

// Filter builds a PRTG APIv2 `filter=` query expression using the confirmed
// operators from Paessler's OpenAPI spec: comparisons (=, !=, <, <=, >, >=,
// in, contains, matches) and logical composition (and, or, not), plus
// hierarchy-scoped field prefixes (children/descendants/ancestors/parents),
// e.g. the confirmed example:
//
//	type = sensor and tags contains "ping" and parentid = "2004"
//
// The zero Filter (Filter{}) represents "no filter" and is skipped by And/Or
// and rendered as the empty string by String(), so it's safe to compose
// optional filter fragments unconditionally.
type Filter struct {
	expr string
}

// String renders the filter expression, or "" for the zero Filter.
func (f Filter) String() string { return f.expr }

// IsZero reports whether f carries no expression.
func (f Filter) IsZero() bool { return f.expr == "" }

// quoteFilterValue double-quotes a filter value, escaping embedded quotes
// and backslashes defensively (PRTG object ids/names are not expected to
// contain them in practice, but this keeps a stray one from producing a
// malformed filter string).
func quoteFilterValue(v string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range v {
		if r == '"' || r == '\\' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('"')
	return b.String()
}

// Eq builds a `field = "value"` filter.
func Eq(field, value string) Filter {
	return Filter{expr: fmt.Sprintf("%s = %s", field, quoteFilterValue(value))}
}

// NotEq builds a `field != "value"` filter.
func NotEq(field, value string) Filter {
	return Filter{expr: fmt.Sprintf("%s != %s", field, quoteFilterValue(value))}
}

// Contains builds a `field contains "value"` filter.
func Contains(field, value string) Filter {
	return Filter{expr: fmt.Sprintf("%s contains %s", field, quoteFilterValue(value))}
}

// Matches builds a `field matches "pattern"` filter, using PRTG's own
// (server-side) pattern matching -- not Go's regexp package. Kept for
// completeness/parity with the confirmed operator list; this package's
// regex-matching features (search.go, plugin's regex query fan-out) instead
// fetch candidates and match with Go's regexp locally, since PRTG's
// "matches" pattern flavor isn't documented precisely enough to rely on for
// user-supplied Go-style regular expressions.
func Matches(field, pattern string) Filter {
	return Filter{expr: fmt.Sprintf("%s matches %s", field, quoteFilterValue(pattern))}
}

// TypeEq builds a `type = "<objectType>"` filter, e.g. TypeEq("sensor").
func TypeEq(objectType string) Filter {
	return Eq("type", objectType)
}

// ParentID builds a `parentid = "<id>"` filter, matching an object's direct
// parent (e.g. a sensor's owning device, or a device's owning group/probe).
func ParentID(id string) Filter {
	return Eq("parentid", id)
}

// AncestorID builds an `ancestors.id = "<id>"` filter, matching any object
// that descends from the given ancestor at any depth (e.g. any sensor
// anywhere under a group, regardless of intervening sub-groups) -- using the
// "ancestors" hierarchy prefix confirmed to exist in PRTG APIv2's filter
// grammar. The exact prefix syntax isn't spelled out with a worked example
// in Paessler's docs beyond confirming the prefix exists, so this is a
// best-effort construction; verify against a live PRTG server (see the
// implementation report's uncertainty list).
func AncestorID(id string) Filter {
	return Eq("ancestors.id", id)
}

func nonZeroExprs(filters []Filter) []string {
	parts := make([]string, 0, len(filters))
	for _, f := range filters {
		if !f.IsZero() {
			parts = append(parts, f.expr)
		}
	}
	return parts
}

// And combines filters with logical "and", skipping any zero Filter. A
// multi-part result is parenthesized so it composes safely as an operand of
// a further And/Or/Not.
func And(filters ...Filter) Filter {
	parts := nonZeroExprs(filters)
	switch len(parts) {
	case 0:
		return Filter{}
	case 1:
		return Filter{expr: parts[0]}
	default:
		return Filter{expr: "(" + strings.Join(parts, " and ") + ")"}
	}
}

// Or combines filters with logical "or", skipping any zero Filter. A
// multi-part result is parenthesized so it composes safely as an operand of
// a further And/Or/Not.
func Or(filters ...Filter) Filter {
	parts := nonZeroExprs(filters)
	switch len(parts) {
	case 0:
		return Filter{}
	case 1:
		return Filter{expr: parts[0]}
	default:
		return Filter{expr: "(" + strings.Join(parts, " or ") + ")"}
	}
}

// Not negates f. Not(Filter{}) is the zero Filter (nothing to negate).
func Not(f Filter) Filter {
	if f.IsZero() {
		return Filter{}
	}
	return Filter{expr: "not " + f.expr}
}
