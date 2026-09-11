package prtg

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

type paginationItem struct {
	ID string `json:"id"`
}

func makeItems(n int) []paginationItem {
	items := make([]paginationItem, n)
	for i := 0; i < n; i++ {
		items[i] = paginationItem{ID: strconv.Itoa(i)}
	}
	return items
}

// pagedServer serves offset/limit pages over a fixed slice of items,
// recording every (offset, limit) it was asked for. If includeTotal is
// true, it also sets X-Total-Count/X-Result-Count response headers.
type pagedServer struct {
	items          []paginationItem
	includeTotal   bool
	requestOffsets []int
	requestLimits  []int
}

func (s *pagedServer) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		s.requestOffsets = append(s.requestOffsets, offset)
		s.requestLimits = append(s.requestLimits, limit)

		end := offset + limit
		if end > len(s.items) {
			end = len(s.items)
		}
		var page []paginationItem
		if offset < len(s.items) {
			page = s.items[offset:end]
		}

		if s.includeTotal {
			w.Header().Set("X-Total-Count", strconv.Itoa(len(s.items)))
			w.Header().Set("X-Result-Count", strconv.Itoa(len(page)))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(page)
	}
}

func newPagedTestClient(t *testing.T, s *pagedServer) *Client {
	t.Helper()
	ts := httptest.NewServer(s.handler())
	t.Cleanup(ts.Close)
	base, err := ParseServerURL(ts.URL)
	if err != nil {
		t.Fatalf("ParseServerURL: %v", err)
	}
	c, err := NewClient(base, ts.Client(), &fakeAuthenticator{tokens: []string{"tok"}})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

func TestFetchPage_ParsesHeaders(t *testing.T) {
	s := &pagedServer{items: makeItems(25), includeTotal: true}
	c := newPagedTestClient(t, s)

	page, err := FetchPage[paginationItem](context.Background(), c, "/experimental/sensors", "", "", 0, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.Items) != 10 {
		t.Fatalf("expected 10 items, got %d", len(page.Items))
	}
	if page.TotalCount != 25 {
		t.Fatalf("expected TotalCount 25, got %d", page.TotalCount)
	}
	if page.ResultCount != 10 {
		t.Fatalf("expected ResultCount 10, got %d", page.ResultCount)
	}
}

func TestFetchPage_MissingHeadersFallBack(t *testing.T) {
	s := &pagedServer{items: makeItems(3), includeTotal: false}
	c := newPagedTestClient(t, s)

	page, err := FetchPage[paginationItem](context.Background(), c, "/experimental/sensors", "", "", 0, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.TotalCount != -1 {
		t.Fatalf("expected TotalCount -1 (header absent), got %d", page.TotalCount)
	}
	if page.ResultCount != len(page.Items) {
		t.Fatalf("expected ResultCount to fall back to len(Items), got %d vs %d", page.ResultCount, len(page.Items))
	}
}

func TestFetchAll_WalksMultiplePages(t *testing.T) {
	s := &pagedServer{items: makeItems(25), includeTotal: true}
	c := newPagedTestClient(t, s)

	result, err := FetchAll[paginationItem](context.Background(), c, "/experimental/sensors", FetchAllOptions{PageSize: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 25 {
		t.Fatalf("expected 25 items, got %d", len(result.Items))
	}
	if result.Truncated {
		t.Fatal("expected Truncated=false")
	}
	wantOffsets := []int{0, 10, 20}
	if fmt.Sprint(s.requestOffsets) != fmt.Sprint(wantOffsets) {
		t.Fatalf("expected offsets %v, got %v", wantOffsets, s.requestOffsets)
	}
}

func TestFetchAll_StopsOnShortPageWithoutTotalHeader(t *testing.T) {
	s := &pagedServer{items: makeItems(15), includeTotal: false}
	c := newPagedTestClient(t, s)

	result, err := FetchAll[paginationItem](context.Background(), c, "/experimental/sensors", FetchAllOptions{PageSize: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 15 {
		t.Fatalf("expected 15 items, got %d", len(result.Items))
	}
	if result.Truncated {
		t.Fatal("expected Truncated=false")
	}
	if len(s.requestOffsets) != 2 {
		t.Fatalf("expected exactly 2 requests, got %d (%v)", len(s.requestOffsets), s.requestOffsets)
	}
}

func TestFetchAll_EmptyResult(t *testing.T) {
	s := &pagedServer{items: makeItems(0), includeTotal: true}
	c := newPagedTestClient(t, s)

	result, err := FetchAll[paginationItem](context.Background(), c, "/experimental/sensors", FetchAllOptions{PageSize: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(result.Items))
	}
	if result.Truncated {
		t.Fatal("expected Truncated=false")
	}
}

func TestFetchAll_MaxItemsTruncation(t *testing.T) {
	tests := []struct {
		name          string
		itemCount     int
		includeTotal  bool
		pageSize      int
		maxItems      int
		wantLen       int
		wantTruncated bool
	}{
		{
			name:          "known total exceeds cap",
			itemCount:     100,
			includeTotal:  true,
			pageSize:      10,
			maxItems:      25,
			wantLen:       25,
			wantTruncated: true,
		},
		{
			name:          "known total exactly equals cap",
			itemCount:     20,
			includeTotal:  true,
			pageSize:      10,
			maxItems:      20,
			wantLen:       20,
			wantTruncated: false,
		},
		{
			name:          "unknown total, full page lands exactly on cap",
			itemCount:     30,
			includeTotal:  false,
			pageSize:      10,
			maxItems:      10,
			wantLen:       10,
			wantTruncated: true,
		},
		{
			name:          "unknown total, short final page lands exactly on cap",
			itemCount:     5,
			includeTotal:  false,
			pageSize:      10,
			maxItems:      5,
			wantLen:       5,
			wantTruncated: false,
		},
		{
			name:          "unknown total, cap overshot within one page",
			itemCount:     30,
			includeTotal:  false,
			pageSize:      10,
			maxItems:      7,
			wantLen:       7,
			wantTruncated: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &pagedServer{items: makeItems(tt.itemCount), includeTotal: tt.includeTotal}
			c := newPagedTestClient(t, s)

			result, err := FetchAll[paginationItem](context.Background(), c, "/experimental/sensors", FetchAllOptions{
				PageSize: tt.pageSize,
				MaxItems: tt.maxItems,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(result.Items) != tt.wantLen {
				t.Fatalf("expected %d items, got %d", tt.wantLen, len(result.Items))
			}
			if result.Truncated != tt.wantTruncated {
				t.Fatalf("expected Truncated=%v, got %v", tt.wantTruncated, result.Truncated)
			}
		})
	}
}

func TestFetchAll_PageSizeClampedToMax(t *testing.T) {
	s := &pagedServer{items: makeItems(1), includeTotal: true}
	c := newPagedTestClient(t, s)

	if _, err := FetchAll[paginationItem](context.Background(), c, "/experimental/sensors", FetchAllOptions{PageSize: 999999}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s.requestLimits) != 1 || s.requestLimits[0] != maxPageSize {
		t.Fatalf("expected limit clamped to %d, got %v", maxPageSize, s.requestLimits)
	}
}

func TestFetchAll_DefaultPageSize(t *testing.T) {
	s := &pagedServer{items: makeItems(1), includeTotal: true}
	c := newPagedTestClient(t, s)

	if _, err := FetchAll[paginationItem](context.Background(), c, "/experimental/sensors", FetchAllOptions{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s.requestLimits) != 1 || s.requestLimits[0] != defaultPageSize {
		t.Fatalf("expected default page size %d, got %v", defaultPageSize, s.requestLimits)
	}
}
