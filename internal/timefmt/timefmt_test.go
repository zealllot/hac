package timefmt_test

import (
	"testing"
	"time"

	"github.com/zealllot/hac/internal/timefmt"
)

func TestParse(t *testing.T) {
	ref := time.Date(2026, 5, 27, 10, 0, 0, 0, time.UTC)

	cases := []struct {
		name   string
		input  string
		want   time.Time
		errSub string // empty == expect success; non-empty = expected substring of err
	}{
		{
			name:  "duration 24h subtracts from ref",
			input: "24h",
			want:  ref.Add(-24 * time.Hour),
		},
		{
			name:  "duration 2h30m subtracts from ref",
			input: "2h30m",
			want:  ref.Add(-(2*time.Hour + 30*time.Minute)),
		},
		{
			name:  "duration 90m subtracts from ref",
			input: "90m",
			want:  ref.Add(-90 * time.Minute),
		},
		{
			name:  "literal 'now' returns ref",
			input: "now",
			want:  ref,
		},
		{
			name:  "ISO-8601 UTC Z-suffix absolute",
			input: "2026-05-26T22:00:00Z",
			want:  time.Date(2026, 5, 26, 22, 0, 0, 0, time.UTC),
		},
		{
			name:  "ISO-8601 with +08:00 offset absolute",
			input: "2026-05-27T06:00:00+08:00",
			want:  time.Date(2026, 5, 26, 22, 0, 0, 0, time.UTC), // same instant
		},
		{
			name:   "garbage errors",
			input:  "yesterday",
			errSub: `parse time "yesterday"`,
		},
		{
			name:   "empty errors",
			input:  "",
			errSub: "empty",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := timefmt.Parse(tc.input, ref)
			if tc.errSub != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.errSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !got.Equal(tc.want) {
				t.Errorf("Parse(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}
