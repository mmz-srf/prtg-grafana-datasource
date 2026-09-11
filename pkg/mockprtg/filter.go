package mockprtg

import "regexp"

// filterClauseRe extracts every `field = "value"` clause from a PRTG APIv2
// filter expression.
var filterClauseRe = regexp.MustCompile(`([A-Za-z0-9_.]+)\s*=\s*"([^"]*)"`)

// ParseSimpleFilter extracts every `field = "value"` clause from filter,
// regardless of `and`/`or`/parens. This mirrors what pkg/prtg's Filter
// builder ever emits for the fields this mock actually needs to honor
// (parentid, ancestors.id, id) -- it is not a general PRTG filter-expression
// parser, the same deliberate simplification pkg/plugin's own
// fakeserver_test.go test double makes.
func ParseSimpleFilter(filter string) map[string]string {
	out := map[string]string{}
	for _, m := range filterClauseRe.FindAllStringSubmatch(filter, -1) {
		out[m[1]] = m[2]
	}
	return out
}
