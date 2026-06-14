package autofmt

import (
	"bytes"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestFormatAutomationKeyOrder(t *testing.T) {
	cfg := map[string]any{
		"mode":       "single",
		"actions":    []any{map[string]any{"action": "light.turn_on"}},
		"id":         "1700000000000",
		"alias":      "测试",
		"triggers":   []any{map[string]any{"platform": "state"}},
		"conditions": []any{},
	}
	out, err := FormatAutomation(cfg)
	if err != nil {
		t.Fatalf("format: %v", err)
	}
	s := string(out)
	order := []string{"alias:", "id:", "triggers:", "conditions:", "actions:", "mode:"}
	last := -1
	for _, k := range order {
		idx := strings.Index(s, "\n"+k)
		if k == "alias:" {
			idx = strings.Index(s, k)
		}
		if idx < 0 {
			t.Fatalf("key %q not found in:\n%s", k, s)
		}
		if idx < last {
			t.Errorf("key %q out of order in:\n%s", k, s)
		}
		last = idx
	}
}

func TestFormatAutomationIdempotent(t *testing.T) {
	cfg := map[string]any{
		"alias": "测试", "id": "1700000000000", "mode": "restart",
		"triggers": []any{map[string]any{"platform": "state", "entity_id": "x.y"}},
		"actions":  []any{map[string]any{"action": "light.turn_on", "data": map[string]any{"brightness_pct": 100}}},
	}
	once, err := FormatAutomation(cfg)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	var reparsed map[string]any
	if err := yaml.Unmarshal(once, &reparsed); err != nil {
		t.Fatalf("reparse: %v", err)
	}
	twice, err := FormatAutomation(reparsed)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if !bytes.Equal(once, twice) {
		t.Errorf("not idempotent:\n--- once ---\n%s\n--- twice ---\n%s", once, twice)
	}
}

func TestFormatAutomationPreservesIDAsString(t *testing.T) {
	out, err := FormatAutomation(map[string]any{"alias": "a", "id": "1700000000000"})
	if err != nil {
		t.Fatalf("format: %v", err)
	}
	if !strings.Contains(string(out), `"1700000000000"`) && !strings.Contains(string(out), `'1700000000000'`) {
		t.Errorf("id should be quoted to stay a string:\n%s", out)
	}
}
