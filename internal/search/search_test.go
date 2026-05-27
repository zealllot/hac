package search_test

import (
	"reflect"
	"sort"
	"testing"

	"github.com/zealllot/hac/internal/ha"
	"github.com/zealllot/hac/internal/search"
)

func sampleDevices() map[string]ha.DeviceCapability {
	return map[string]ha.DeviceCapability{
		"light.keting_shedeng_dengzu": {EntityID: "light.keting_shedeng_dengzu", Domain: "light", Name: "客厅射灯灯组", Area: "客厅", State: "off"},
		"light.zhuwo_dengzu":          {EntityID: "light.zhuwo_dengzu", Domain: "light", Name: "主卧灯组", Area: "主卧", State: "on"},
		"binary_sensor.ke_ting_occ":   {EntityID: "binary_sensor.ke_ting_occ", Domain: "binary_sensor", Name: "客厅人体", Area: "客厅", State: "off"},
		"switch.washing_machine":      {EntityID: "switch.washing_machine", Domain: "switch", Name: "Washing Machine", Area: "厨房", State: "off"},
	}
}

func ids(ms []search.Match) []string {
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.EntityID)
	}
	sort.Strings(out)
	return out
}

func TestRun(t *testing.T) {
	cases := []struct {
		name    string
		query   string
		wantIDs []string
	}{
		{
			name:    "by entity_id substring",
			query:   "keting",
			wantIDs: []string{"light.keting_shedeng_dengzu"},
		},
		{
			name:    "by friendly_name substring (中文)",
			query:   "客厅",
			wantIDs: []string{"binary_sensor.ke_ting_occ", "light.keting_shedeng_dengzu"},
		},
		{
			name:    "by area",
			query:   "主卧",
			wantIDs: []string{"light.zhuwo_dengzu"},
		},
		{
			name:    "case insensitive on ASCII",
			query:   "WASHING",
			wantIDs: []string{"switch.washing_machine"},
		},
		{
			name:    "empty query returns all",
			query:   "",
			wantIDs: []string{"binary_sensor.ke_ting_occ", "light.keting_shedeng_dengzu", "light.zhuwo_dengzu", "switch.washing_machine"},
		},
		{
			name:    "no match returns empty",
			query:   "nonexistent",
			wantIDs: []string{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ids(search.Run(sampleDevices(), tc.query))
			if len(got) == 0 && len(tc.wantIDs) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.wantIDs) {
				t.Errorf("Run(%q) = %v, want %v", tc.query, got, tc.wantIDs)
			}
		})
	}
}

func TestByArea_exactMatchOnly(t *testing.T) {
	got := ids(search.ByArea(sampleDevices(), "客厅"))
	want := []string{"binary_sensor.ke_ting_occ", "light.keting_shedeng_dengzu"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ByArea = %v, want %v", got, want)
	}

	// Partial match should NOT hit.
	got = ids(search.ByArea(sampleDevices(), "客"))
	if len(got) != 0 {
		t.Errorf("ByArea should require exact match; got %v", got)
	}
}
