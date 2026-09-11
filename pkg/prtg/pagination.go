package prtg

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// defaultPageSize matches PRTG APIv2's documented default offset/limit page
// size for list endpoints.
const defaultPageSize = 100

// maxPageSize matches PRTG APIv2's documented maximum limit for list
// endpoints (0 also means "max" server-side, but we always send an explicit
// positive limit so a short page reliably signals end-of-data).
const maxPageSize = 3000

// Page is a single decoded page of list results, plus the pagination
// metadata PRTG reports via response headers (list endpoints return bare
// JSON arrays with no body-level pagination envelope).
type Page[T any] struct {
	Items []T
	// TotalCount is parsed from the X-Total-Count header, or -1 if the
	// header was absent/unparseable.
	TotalCount int
	// ResultCount is parsed from the X-Result-Count header, falling back to
	// len(Items) if the header was absent/unparseable.
	ResultCount int
}

func parseCountHeader(h http.Header, key string) int {
	v := h.Get(key)
	if v == "" {
		return -1
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return -1
	}
	return n
}

// FetchPage fetches a single page of list results from a PRTG APIv2
// metadata endpoint (e.g. "/experimental/sensors"), applying an optional
// filter/include expression on top of an explicit offset/limit.
func FetchPage[T any](ctx context.Context, c *Client, path string, filter string, include string, offset, limit int) (Page[T], error) {
	query := url.Values{}
	if filter != "" {
		query.Set("filter", filter)
	}
	if include != "" {
		query.Set("include", include)
	}
	query.Set("offset", strconv.Itoa(offset))
	query.Set("limit", strconv.Itoa(limit))

	var items []T
	headers, err := c.do(ctx, http.MethodGet, path, query, &items)
	if err != nil {
		return Page[T]{}, err
	}

	total := parseCountHeader(headers, "X-Total-Count")
	result := parseCountHeader(headers, "X-Result-Count")
	if result < 0 {
		result = len(items)
	}
	return Page[T]{Items: items, TotalCount: total, ResultCount: result}, nil
}

// FetchAllOptions bounds an unbounded FetchAll walk over a paginated list
// endpoint.
type FetchAllOptions struct {
	Filter  string
	Include string
	// PageSize overrides defaultPageSize (100) if > 0; always capped to
	// maxPageSize (3000), PRTG's documented maximum.
	PageSize int
	// MaxItems caps the total number of items walked; 0 means unlimited.
	// When the cap is hit, FetchAllResult.Truncated is set so callers can
	// surface that the result may be incomplete instead of silently
	// dropping data.
	MaxItems int
}

// FetchAllResult is FetchAll's return value.
type FetchAllResult[T any] struct {
	Items []T
	// Truncated is true when MaxItems cut the walk short of the server's
	// reported total (or, if the server didn't report a total, short of
	// what a full walk might otherwise have returned).
	Truncated bool
}

// FetchAll walks every page of a PRTG APIv2 list endpoint (offset/limit),
// accumulating items until either the server signals no more data (a page
// shorter than the requested page size) or opts.MaxItems is reached.
func FetchAll[T any](ctx context.Context, c *Client, path string, opts FetchAllOptions) (FetchAllResult[T], error) {
	pageSize := opts.PageSize
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}

	var all []T
	offset := 0
	for {
		page, err := FetchPage[T](ctx, c, path, opts.Filter, opts.Include, offset, pageSize)
		if err != nil {
			return FetchAllResult[T]{}, err
		}
		all = append(all, page.Items...)

		if opts.MaxItems > 0 && len(all) >= opts.MaxItems {
			beforeCapLen := len(all)
			var truncated bool
			switch {
			case page.TotalCount >= 0:
				// We know the server's real total: truncated iff it
				// exceeds what we're about to cap the result to.
				truncated = page.TotalCount > opts.MaxItems
			case beforeCapLen > opts.MaxItems:
				// We overshot the cap within a single page fetch, so there
				// was more data than the cap regardless of what follows.
				truncated = true
			case len(page.Items) == pageSize:
				// Landed exactly on the cap with a full-sized page: more
				// might follow and we can't tell, so warn conservatively.
				truncated = true
			default:
				// Landed exactly on the cap with a short/empty final page:
				// that was genuinely the end of the data.
				truncated = false
			}
			all = all[:opts.MaxItems]
			return FetchAllResult[T]{Items: all, Truncated: truncated}, nil
		}

		if len(page.Items) == 0 || len(page.Items) < pageSize {
			// Short (or empty) page: no more data, regardless of what
			// X-Total-Count reported.
			break
		}
		offset += len(page.Items)
		if page.TotalCount >= 0 && offset >= page.TotalCount {
			break
		}
	}
	return FetchAllResult[T]{Items: all, Truncated: false}, nil
}
