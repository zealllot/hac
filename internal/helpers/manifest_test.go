package helpers

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestManifestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	m := Manifest{
		"quan_ju_liang_du": {"name": "全局亮度", "min": 1, "max": 100, "step": 5},
		"ke_ting_shou_dong": {"name": "客厅手动", "icon": "mdi:gesture-tap"},
	}
	path := filepath.Join(dir, "input_number.yaml")
	if err := WriteManifest(path, m); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := ReadManifest(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got["ke_ting_shou_dong"]["icon"] != "mdi:gesture-tap" {
		t.Errorf("icon round-trip failed: %v", got["ke_ting_shou_dong"])
	}
	if len(got) != 2 {
		t.Errorf("got %d entries, want 2", len(got))
	}
}

func TestReadManifestMissingFileIsEmpty(t *testing.T) {
	got, err := ReadManifest(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want empty manifest, got %v", got)
	}
}

func TestFromCollectionItems(t *testing.T) {
	items := []map[string]any{
		{"id": "a", "name": "A", "min": 0},
		{"id": "b", "name": "B"},
	}
	m := FromCollectionItems(items)
	want := Manifest{"a": {"name": "A", "min": 0}, "b": {"name": "B"}}
	if !reflect.DeepEqual(m, want) {
		t.Errorf("got %v, want %v", m, want)
	}
	if _, hasID := m["a"]["id"]; hasID {
		t.Error("id should be stripped from config")
	}
}

var _ = os.Stdout
