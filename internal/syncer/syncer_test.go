package syncer_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zealllot/hac/internal/ha"
	"github.com/zealllot/hac/internal/syncer"
)

// fakeClient implements both HAClient and WSClient interfaces with canned data.
type fakeClient struct {
	automations []ha.EntityState
	configs     map[string]map[string]any // automationID → config map
	cats        []ha.Category
	entities    []ha.EntityRegistryEntry
}

func (f *fakeClient) GetAutomations() ([]ha.EntityState, error) {
	return f.automations, nil
}
func (f *fakeClient) GetAutomationConfig(id string) (map[string]any, error) {
	if c, ok := f.configs[id]; ok {
		return c, nil
	}
	return nil, os.ErrNotExist
}
func (f *fakeClient) ListCategories(scope string) ([]ha.Category, error) {
	return f.cats, nil
}
func (f *fakeClient) GetEntityRegistry() ([]ha.EntityRegistryEntry, error) {
	return f.entities, nil
}

func TestSync_deletesOrphanWhenIdMovedCategory(t *testing.T) {
	repo := t.TempDir()
	// Seed: same id exists at TWO local paths — orphan from old "其他/" plus a
	// canonical file under "光亮灯灭/". HA says it belongs to 光亮灯灭.
	mkfile := func(rel string) {
		full := filepath.Join(repo, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("id: '111'\nalias: 客厅_光亮_关灯\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mkfile("automations/其他/客厅_光亮_关灯.yaml")
	mkfile("automations/光亮灯灭/客厅_光亮_关灯.yaml")

	fc := &fakeClient{
		automations: []ha.EntityState{
			{EntityID: "automation.ke_ting_guang_liang_guan_deng", Attributes: map[string]any{"id": "111"}},
		},
		configs: map[string]map[string]any{
			"111": {"id": "111", "alias": "客厅_光亮_关灯"},
		},
		cats: []ha.Category{{CategoryID: "c1", Name: "光亮灯灭"}},
		entities: []ha.EntityRegistryEntry{
			{EntityID: "automation.ke_ting_guang_liang_guan_deng", Categories: map[string]string{"automation": "c1"}},
		},
	}

	s := &syncer.Syncer{HA: fc, WS: fc, ConfigRepo: repo}
	report, err := s.Sync()
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	orphan := filepath.Join(repo, "automations", "其他", "客厅_光亮_关灯.yaml")
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Errorf("orphan still exists at %s", orphan)
	}
	canonical := filepath.Join(repo, "automations", "光亮灯灭", "客厅_光亮_关灯.yaml")
	if _, err := os.Stat(canonical); err != nil {
		t.Errorf("canonical missing at %s: %v", canonical, err)
	}

	foundOrphan := false
	for _, p := range report.DeletedOrphans {
		if p == orphan {
			foundOrphan = true
			break
		}
	}
	if !foundOrphan {
		t.Errorf("report.DeletedOrphans missing %s; got %v", orphan, report.DeletedOrphans)
	}
}

func TestSync_warnsAboutLocalOnlyFile(t *testing.T) {
	repo := t.TempDir()
	rel := "automations/光暗灯亮/local_wip.yaml"
	full := filepath.Join(repo, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("id: 'wip-only'\nalias: local_wip\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	fc := &fakeClient{
		automations: nil, // HA has nothing
		cats:        nil,
		entities:    nil,
	}
	s := &syncer.Syncer{HA: fc, WS: fc, ConfigRepo: repo}
	report, err := s.Sync()
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	if _, err := os.Stat(full); err != nil {
		t.Errorf("local-only file was deleted at %s; should be kept", full)
	}
	found := false
	for _, p := range report.WarnLocalOnly {
		if p == full {
			found = true
		}
	}
	if !found {
		t.Errorf("WarnLocalOnly missing %s; got %v", full, report.WarnLocalOnly)
	}
}

func TestSync_writesFilesAtExpectedPaths(t *testing.T) {
	repo := t.TempDir()
	fc := &fakeClient{
		automations: []ha.EntityState{
			{EntityID: "automation.ke_ting_guang_liang_guan_deng", Attributes: map[string]any{"id": "1773036980350328000"}},
		},
		configs: map[string]map[string]any{
			"1773036980350328000": {
				"id":      "1773036980350328000",
				"alias":   "客厅_光亮_关灯",
				"trigger": []any{},
			},
		},
		cats: []ha.Category{
			{CategoryID: "cat-abc", Name: "光亮灯灭"},
		},
		entities: []ha.EntityRegistryEntry{
			{EntityID: "automation.ke_ting_guang_liang_guan_deng", Categories: map[string]string{"automation": "cat-abc"}},
		},
	}

	s := &syncer.Syncer{HA: fc, WS: fc, ConfigRepo: repo}
	report, err := s.Sync()
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}

	want := filepath.Join(repo, "automations", "光亮灯灭", "客厅_光亮_关灯.yaml")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected file at %s: %v", want, err)
	}
	if len(report.Created) != 1 || report.Created[0] != want {
		t.Errorf("report.Created = %v, want [%s]", report.Created, want)
	}
}
