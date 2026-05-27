// Package search filters HA device data by free-text query or exact area name.
// All functions are pure; HTTP fetching happens in the caller.
package search

import (
	"strings"

	"github.com/zealllot/hac/internal/ha"
)

// Match is one search hit.
type Match struct {
	EntityID     string `json:"entity_id"`
	State        string `json:"state"`
	FriendlyName string `json:"friendly_name"`
	Area         string `json:"area"`
}

// Run performs case-insensitive substring match against entity_id,
// friendly_name, and area (OR semantics). An empty query returns all devices.
func Run(devices map[string]ha.DeviceCapability, query string) []Match {
	q := strings.ToLower(query)
	out := make([]Match, 0, len(devices))
	for _, d := range devices {
		m := Match{
			EntityID:     d.EntityID,
			State:        d.State,
			FriendlyName: d.Name,
			Area:         d.Area,
		}
		if q == "" {
			out = append(out, m)
			continue
		}
		if strings.Contains(strings.ToLower(d.EntityID), q) ||
			strings.Contains(strings.ToLower(d.Name), q) ||
			strings.Contains(strings.ToLower(d.Area), q) {
			out = append(out, m)
		}
	}
	return out
}

// ByArea returns devices whose Area exactly equals the given area name.
// Use Run for substring matching.
func ByArea(devices map[string]ha.DeviceCapability, area string) []Match {
	out := make([]Match, 0)
	for _, d := range devices {
		if d.Area == area {
			out = append(out, Match{
				EntityID:     d.EntityID,
				State:        d.State,
				FriendlyName: d.Name,
				Area:         d.Area,
			})
		}
	}
	return out
}
