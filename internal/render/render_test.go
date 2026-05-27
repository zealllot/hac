package render_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/zealllot/hac/internal/ha"
	"github.com/zealllot/hac/internal/logbook"
	"github.com/zealllot/hac/internal/render"
)

func TestLogbook_json(t *testing.T) {
	events := []logbook.Event{
		{
			When:        time.Date(2026, 5, 26, 13, 39, 3, 0, time.UTC),
			State:       "off",
			EntityID:    "light.x",
			Name:        "灯 X",
			ContextName: "客厅_无人_关灯",
		},
	}
	var buf bytes.Buffer
	if err := render.Logbook(&buf, events, "json"); err != nil {
		t.Fatalf("Logbook: %v", err)
	}
	var got []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output not JSON array: %v\n%s", err, buf.String())
	}
	if len(got) != 1 || got[0]["state"] != "off" || got[0]["entity_id"] != "light.x" {
		t.Errorf("wrong JSON: %+v", got)
	}
}

func TestLogbook_table(t *testing.T) {
	events := []logbook.Event{
		{
			When:        time.Date(2026, 5, 26, 13, 39, 3, 0, time.UTC),
			State:       "off",
			EntityID:    "light.x",
			Name:        "灯 X",
			ContextName: "客厅_无人_关灯",
		},
	}
	var buf bytes.Buffer
	if err := render.Logbook(&buf, events, "table"); err != nil {
		t.Fatalf("Logbook: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"light.x", "off", "灯 X", "客厅_无人_关灯", "when"} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing %q\nactual:\n%s", want, out)
		}
	}
}

func sampleDevices() map[string]ha.DeviceCapability {
	return map[string]ha.DeviceCapability{
		"light.a": {EntityID: "light.a", Domain: "light", Name: "灯 A", Area: "客厅", State: "on"},
		"light.b": {EntityID: "light.b", Domain: "light", Name: "灯 B", Area: "卧室", State: "off"},
	}
}

func TestDevices_json(t *testing.T) {
	var buf bytes.Buffer
	if err := render.Devices(&buf, sampleDevices(), "json"); err != nil {
		t.Fatalf("Devices: %v", err)
	}
	var got []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not a JSON array: %v\noutput: %s", err, buf.String())
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2", len(got))
	}
	// Spot-check at least one entry has the expected keys.
	for _, e := range got {
		if _, ok := e["entity_id"]; !ok {
			t.Errorf("entry missing entity_id: %+v", e)
		}
		if _, ok := e["state"]; !ok {
			t.Errorf("entry missing state: %+v", e)
		}
	}
}

func TestState_json(t *testing.T) {
	s := &ha.EntityState{
		EntityID: "light.x",
		State:    "on",
		Attributes: map[string]any{
			"brightness":    255,
			"friendly_name": "灯 X",
		},
	}
	var buf bytes.Buffer
	if err := render.State(&buf, s, "json"); err != nil {
		t.Fatalf("State: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not JSON object: %v", err)
	}
	if got["entity_id"] != "light.x" || got["state"] != "on" {
		t.Errorf("missing fields in JSON: %+v", got)
	}
}

func TestStates_jsonArray(t *testing.T) {
	states := []ha.EntityState{
		{EntityID: "light.a", State: "on"},
		{EntityID: "light.b", State: "not_found"},
	}
	var buf bytes.Buffer
	if err := render.States(&buf, states, "json"); err != nil {
		t.Fatalf("States: %v", err)
	}
	var got []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output not JSON array: %v\n%s", err, buf.String())
	}
	if len(got) != 2 || got[1]["state"] != "not_found" {
		t.Errorf("wrong JSON: %+v", got)
	}
}

func TestState_table(t *testing.T) {
	s := &ha.EntityState{
		EntityID:   "light.x",
		State:      "on",
		Attributes: map[string]any{"friendly_name": "灯 X", "brightness": 255},
	}
	var buf bytes.Buffer
	if err := render.State(&buf, s, "table"); err != nil {
		t.Fatalf("State: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"entity_id", "light.x", "state", "on", "friendly_name", "灯 X", "brightness", "255"} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing %q\nactual:\n%s", want, out)
		}
	}
}

func TestDeploy_json(t *testing.T) {
	results := []render.DeployResult{
		{
			File: "automations/光亮灯灭/x.yaml", Alias: "客厅 光亮 关灯", Category: "光亮灯灭",
			AutomationID: "123", EntityID: "automation.ke_ting_guang_liang_guan_deng",
			GitAdded: true, GitCommitted: false,
		},
		{
			File: "automations/未知/y.yaml", Error: "category \"未知\" not found",
		},
	}
	var buf bytes.Buffer
	if err := render.Deploy(&buf, results, "json"); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	var got []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output not JSON array: %v\n%s", err, buf.String())
	}
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2", len(got))
	}
	if got[0]["git_added"] != true || got[0]["git_committed"] != false {
		t.Errorf("first row missing git_added/committed: %+v", got[0])
	}
	if got[1]["error"] == nil {
		t.Errorf("second row missing error: %+v", got[1])
	}
}

func TestDeploy_table(t *testing.T) {
	results := []render.DeployResult{
		{File: "x.yaml", Alias: "foo", GitAdded: true, GitCommitted: true},
		{File: "y.yaml", Error: "boom"},
	}
	var buf bytes.Buffer
	if err := render.Deploy(&buf, results, "table"); err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"x.yaml", "y.yaml", "foo", "boom"} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing %q\nactual:\n%s", want, out)
		}
	}
}

func TestAutomations_table(t *testing.T) {
	autos := []ha.EntityState{
		{EntityID: "automation.客厅_有人_开灯", State: "on", Attributes: map[string]any{"friendly_name": "客厅 有人 开灯"}},
		{EntityID: "automation.卧室_无人_关灯", State: "off", Attributes: map[string]any{"friendly_name": "卧室 无人 关灯"}},
	}
	var buf bytes.Buffer
	if err := render.Automations(&buf, autos, "table"); err != nil {
		t.Fatalf("Automations: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"客厅_有人_开灯", "客厅 有人 开灯", "卧室_无人_关灯", "on", "off"} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing %q\nactual:\n%s", want, out)
		}
	}
}

func TestDevices_table(t *testing.T) {
	var buf bytes.Buffer
	if err := render.Devices(&buf, sampleDevices(), "table"); err != nil {
		t.Fatalf("Devices: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"light.a", "light.b", "灯 A", "灯 B", "on", "off"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q\nactual:\n%s", want, out)
		}
	}
	// First non-empty line is header row.
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 3 {
		t.Fatalf("table should have header + 2 rows, got %d lines:\n%s", len(lines), out)
	}
	header := lines[0]
	for _, col := range []string{"entity_id", "state", "name"} {
		if !strings.Contains(strings.ToLower(header), col) {
			t.Errorf("header missing column %q: %s", col, header)
		}
	}
}
