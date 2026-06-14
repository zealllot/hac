package helpers

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ReadManifest loads one helper-type file. A missing file yields an empty
// manifest (not an error), so callers can treat absent files as "no helpers".
func ReadManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Manifest{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	m := Manifest{}
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return m, nil
}

// WriteManifest writes one helper-type file, creating parent dirs. yaml.v3
// sorts map keys, so output is deterministic across runs.
func WriteManifest(path string, m Manifest) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// FromCollectionItems converts <domain>/list output into a Manifest, using each
// item's "id" as the object_id key and the remaining fields as its config.
func FromCollectionItems(items []map[string]any) Manifest {
	m := Manifest{}
	for _, item := range items {
		id, _ := item["id"].(string)
		if id == "" {
			continue
		}
		cfg := make(map[string]any, len(item))
		for k, v := range item {
			if k == "id" {
				continue
			}
			cfg[k] = v
		}
		m[id] = cfg
	}
	return m
}
