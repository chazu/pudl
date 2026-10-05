package lister

import "testing"

func TestLimitedPageSize(t *testing.T) {
	cases := []struct {
		name                   string
		limit, offset, perPage int
		want                   int
	}{
		{"no limit", 0, 0, 20, 20},
		{"limit below page", 3, 0, 20, 3},
		{"limit above page", 50, 0, 20, 20},
		{"last partial page", 3, 2, 2, 1},
		{"page past limit", 3, 4, 2, 0},
		{"page ends at limit", 4, 2, 2, 2},
	}
	for _, tc := range cases {
		if got := limitedPageSize(tc.limit, tc.offset, tc.perPage); got != tc.want {
			t.Errorf("%s: limitedPageSize(%d, %d, %d) = %d, want %d", tc.name, tc.limit, tc.offset, tc.perPage, got, tc.want)
		}
	}
}
