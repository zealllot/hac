package category_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/zealllot/hac/internal/category"
	"github.com/zealllot/hac/internal/ha"
)

type fakeWS struct {
	cats           []ha.Category
	listErr        error
	createReturnID string
	createErr      error
	assignErr      error
	listCalls      int
	createCalls    int
	assignCalls    int
	lastAssignArgs struct{ scope, entityID, catID string }
}

func (f *fakeWS) ListCategories(scope string) ([]ha.Category, error) {
	f.listCalls++
	return f.cats, f.listErr
}
func (f *fakeWS) CreateCategory(scope, name, icon string) (*ha.Category, error) {
	f.createCalls++
	if f.createErr != nil {
		return nil, f.createErr
	}
	return &ha.Category{CategoryID: f.createReturnID, Name: name, Icon: icon}, nil
}
func (f *fakeWS) AssignCategory(scope, entityID, catID string) error {
	f.assignCalls++
	f.lastAssignArgs = struct{ scope, entityID, catID string }{scope, entityID, catID}
	return f.assignErr
}

func TestEnsureExists_hitReturnsID(t *testing.T) {
	ws := &fakeWS{
		cats: []ha.Category{
			{CategoryID: "abc-123", Name: "光亮灯灭"},
			{CategoryID: "def-456", Name: "人来灯亮"},
		},
	}
	got, err := category.EnsureExists(ws, "光亮灯灭", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "abc-123" {
		t.Errorf("ID = %q, want abc-123", got)
	}
	if ws.createCalls != 0 {
		t.Errorf("Create called %d times when category existed", ws.createCalls)
	}
}

func TestEnsureExists_missWithoutAutoCreateReturnsNotFound(t *testing.T) {
	ws := &fakeWS{
		cats: []ha.Category{
			{CategoryID: "abc", Name: "光亮灯灭"},
			{CategoryID: "def", Name: "人来灯亮"},
		},
	}
	_, err := category.EnsureExists(ws, "不存在分类", false)
	if err == nil {
		t.Fatalf("expected NotFoundError, got nil")
	}
	var nfErr *category.NotFoundError
	if !errors.As(err, &nfErr) {
		t.Fatalf("expected *NotFoundError, got %T: %v", err, err)
	}
	if nfErr.Want != "不存在分类" {
		t.Errorf("Want = %q, want 不存在分类", nfErr.Want)
	}
	// Error message should contain the missing name AND existing names (for diagnostics).
	msg := err.Error()
	for _, frag := range []string{"不存在分类", "光亮灯灭", "人来灯亮", "create-category"} {
		if !strings.Contains(msg, frag) {
			t.Errorf("error missing %q\nfull: %s", frag, msg)
		}
	}
	if ws.createCalls != 0 {
		t.Errorf("Create called %d times when autoCreate=false", ws.createCalls)
	}
}

func TestEnsureExists_missWithAutoCreateCreatesAndReturnsID(t *testing.T) {
	ws := &fakeWS{
		cats:           []ha.Category{{CategoryID: "abc", Name: "光亮灯灭"}},
		createReturnID: "new-uuid",
	}
	got, err := category.EnsureExists(ws, "新分类", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "new-uuid" {
		t.Errorf("ID = %q, want new-uuid", got)
	}
	if ws.createCalls != 1 {
		t.Errorf("Create called %d times, want 1", ws.createCalls)
	}
}

func TestResolve(t *testing.T) {
	cases := []struct {
		name string
		path string
		want string
	}{
		{"sibling of automations root", "automations/光亮灯灭/客厅_光亮_关灯.yaml", "光亮灯灭"},
		{"file at automations root", "automations/foo.yaml", ""},
		{"nested deeper than category dir", "automations/光亮灯灭/sub/foo.yaml", "sub"},
		{"path with leading dot-slash", "./automations/人来灯亮/foo.yaml", "人来灯亮"},
		{"absolute path", "/Users/zealot/ha-config/automations/睡眠模式/foo.yaml", "睡眠模式"},
		{"file outside automations dir", "scripts/foo.yaml", ""},
		{"file in random dir", "/tmp/foo.yaml", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := category.Resolve(tc.path)
			if got != tc.want {
				t.Errorf("Resolve(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}
