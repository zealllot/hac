package helpers

import "testing"

func TestCollectionDomains(t *testing.T) {
	got := CollectionDomains()
	want := map[string]bool{
		"input_boolean": true, "input_number": true, "input_text": true,
		"input_select": true, "input_button": true, "input_datetime": true,
		"counter": true, "timer": true, "schedule": true,
	}
	if len(got) != len(want) {
		t.Fatalf("got %d domains, want %d", len(got), len(want))
	}
	for _, d := range got {
		if !want[d] {
			t.Errorf("unexpected domain %q", d)
		}
	}
}

func TestConfigEntryDomains(t *testing.T) {
	got := ConfigEntryDomains()
	if len(got) != 1 || got[0] != "template" {
		t.Fatalf("got %v, want [template]", got)
	}
}
