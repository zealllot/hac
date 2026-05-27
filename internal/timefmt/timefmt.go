// Package timefmt parses CLI time expressions: relative durations ("24h",
// "2h30m"), the literal "now", or absolute ISO-8601 timestamps.
package timefmt

import (
	"errors"
	"fmt"
	"time"
)

// Parse interprets s as either a duration (subtracted from ref) or an
// absolute ISO-8601 timestamp. The literal "now" returns ref unchanged.
func Parse(s string, ref time.Time) (time.Time, error) {
	if s == "" {
		return time.Time{}, errors.New("empty time expression")
	}
	if s == "now" {
		return ref, nil
	}

	// Try as Go duration first ("24h", "2h30m", "90m"). If it parses, treat
	// as ref - duration (CLI semantics: "--since 24h" means 24h ago).
	if d, err := time.ParseDuration(s); err == nil {
		return ref.Add(-d), nil
	}

	// Try absolute ISO-8601. RFC3339 handles both "...Z" and "...+08:00".
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}

	return time.Time{}, fmt.Errorf(`parse time %q: not a duration or ISO-8601 timestamp`, s)
}
