package category

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zealllot/hac/internal/ha"
)

// WSClient is the subset of the HA WebSocket client used by this package.
// Defined here so the package can be unit-tested with a stub.
type WSClient interface {
	ListCategories(scope string) ([]ha.Category, error)
	CreateCategory(scope, name, icon string) (*ha.Category, error)
	AssignCategory(scope, entityID, categoryID string) error
}

// NotFoundError is returned by EnsureExists when the named category does not
// exist on HA and autoCreate is false.
type NotFoundError struct {
	Want     string
	Existing []string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf(
		"category %q not found on HA. existing: [%s]; pass --create-category to create it",
		e.Want, strings.Join(e.Existing, ", "),
	)
}

// EnsureExists resolves a category by display name. If the category is missing
// and autoCreate is true, it is created on HA. Returns the category UUID.
// When autoCreate is false and the category is missing, returns *NotFoundError
// (no mutation on HA).
func EnsureExists(ws WSClient, name string, autoCreate bool) (string, error) {
	cats, err := ws.ListCategories("automation")
	if err != nil {
		return "", fmt.Errorf("list categories: %w", err)
	}
	for _, c := range cats {
		if c.Name == name {
			return c.CategoryID, nil
		}
	}
	if !autoCreate {
		existing := make([]string, len(cats))
		for i, c := range cats {
			existing[i] = c.Name
		}
		return "", &NotFoundError{Want: name, Existing: existing}
	}
	created, err := ws.CreateCategory("automation", name, "")
	if err != nil {
		return "", fmt.Errorf("create category %q: %w", name, err)
	}
	return created.CategoryID, nil
}

// Assign attaches the deployed automation to the given category by UUID.
func Assign(ws WSClient, entityID, categoryID string) error {
	if err := ws.AssignCategory("automation", entityID, categoryID); err != nil {
		return fmt.Errorf("assign category: %w", err)
	}
	return nil
}

// Resolve extracts the HA category name from an automation file path,
// using the convention that the immediate subdirectory under `automations/`
// names the category.
//
//	automations/光亮灯灭/foo.yaml     -> "光亮灯灭"
//	automations/foo.yaml             -> ""    (file at automations root)
//	scripts/foo.yaml                 -> ""    (outside automations tree)
//	automations/x/y/z.yaml           -> "y"   (deepest segment between automations and file)
//
// Treats "./automations/..." and absolute paths identically.
func Resolve(filePath string) string {
	clean := filepath.ToSlash(filepath.Clean(filePath))
	parts := strings.Split(clean, "/")

	// Find the LAST "automations" segment (handles both relative and absolute paths).
	autoIdx := -1
	for i, p := range parts {
		if p == "automations" {
			autoIdx = i
		}
	}
	if autoIdx < 0 {
		return ""
	}
	// We want the segment immediately before the file (parts[len-1]).
	// If file is directly under automations/ (autoIdx == len-2), no category.
	if autoIdx >= len(parts)-2 {
		return ""
	}
	return parts[len(parts)-2]
}
