// Package syncer reconciles a local automation YAML repository against
// the HA automation registry. HA is the source of truth: each automation's
// directory under automations/ is determined by its HA category. Local files
// whose path doesn't match HA's category are treated as orphans and deleted.
package syncer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zealllot/hac/internal/ha"
	"gopkg.in/yaml.v3"
)

// HAClient is the REST-side subset used by Sync.
type HAClient interface {
	GetAutomations() ([]ha.EntityState, error)
	GetAutomationConfig(id string) (map[string]any, error)
}

// WSClient is the WebSocket-side subset used by Sync.
type WSClient interface {
	ListCategories(scope string) ([]ha.Category, error)
	GetEntityRegistry() ([]ha.EntityRegistryEntry, error)
}

// Syncer reconciles a local repo against HA.
type Syncer struct {
	HA         HAClient
	WS         WSClient
	ConfigRepo string // absolute path to the ha-config root
}

// Report summarizes what Sync changed.
type Report struct {
	Created        []string `json:"created"`
	Updated        []string `json:"updated"`
	DeletedOrphans []string `json:"deleted_orphans"`
	WarnLocalOnly  []string `json:"warn_local_only"`
}

// Sync writes one YAML per HA automation at automations/<category>/<slug>.yaml,
// deletes any local file whose id matches an HA automation but lives at a
// different path, and warns about local-only files.
func (s *Syncer) Sync() (Report, error) {
	rep := Report{}

	automations, err := s.HA.GetAutomations()
	if err != nil {
		return rep, fmt.Errorf("get automations: %w", err)
	}

	cats, err := s.WS.ListCategories("automation")
	if err != nil {
		return rep, fmt.Errorf("list categories: %w", err)
	}
	catNameByID := make(map[string]string, len(cats))
	for _, c := range cats {
		catNameByID[c.CategoryID] = c.Name
	}

	entities, err := s.WS.GetEntityRegistry()
	if err != nil {
		return rep, fmt.Errorf("get entity registry: %w", err)
	}
	categoryNameByEntityID := make(map[string]string)
	for _, e := range entities {
		if catID, ok := e.Categories["automation"]; ok {
			if name, ok := catNameByID[catID]; ok {
				categoryNameByEntityID[e.EntityID] = name
			}
		}
	}

	automationsDir := filepath.Join(s.ConfigRepo, "automations")

	// Index existing local files by id BEFORE writes, so we can detect orphans
	// after we know the canonical path for each HA automation.
	localByID, err := scanLocalAutomations(automationsDir)
	if err != nil {
		return rep, fmt.Errorf("scan local automations: %w", err)
	}

	haIDs := make(map[string]bool, len(automations))

	for _, a := range automations {
		id, _ := a.Attributes["id"].(string)
		if id == "" {
			continue
		}
		haIDs[id] = true

		config, err := s.HA.GetAutomationConfig(id)
		if err != nil {
			continue
		}
		alias, _ := config["alias"].(string)
		if alias == "" {
			alias = strings.TrimPrefix(a.EntityID, "automation.")
		}

		categoryName := categoryNameByEntityID[a.EntityID]
		var targetPath string
		if categoryName != "" {
			targetPath = filepath.Join(automationsDir, categoryName, slug(alias)+".yaml")
		} else {
			targetPath = filepath.Join(automationsDir, slug(alias)+".yaml")
		}

		data, err := yaml.Marshal(config)
		if err != nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			continue
		}
		if err := os.WriteFile(targetPath, data, 0o644); err != nil {
			continue
		}
		rep.Created = append(rep.Created, targetPath)

		// Delete orphan: a local file with this id at a different path.
		for _, existing := range localByID[id] {
			if existing != targetPath {
				if err := os.Remove(existing); err == nil {
					rep.DeletedOrphans = append(rep.DeletedOrphans, existing)
				}
			}
		}
	}

	// Local files whose id is not on HA are flagged (likely WIP), never deleted.
	for id, paths := range localByID {
		if haIDs[id] {
			continue
		}
		rep.WarnLocalOnly = append(rep.WarnLocalOnly, paths...)
	}

	return rep, nil
}

// scanLocalAutomations walks automations/**/*.yaml and returns id -> list of
// paths (a single id may appear in multiple paths when prior syncs left orphans).
func scanLocalAutomations(dir string) (map[string][]string, error) {
	out := make(map[string][]string)
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil // empty repo; not an error
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil // skip unreadable
		}
		var doc struct {
			ID string `yaml:"id"`
		}
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return nil // skip unparsable
		}
		if doc.ID == "" {
			return nil
		}
		out[doc.ID] = append(out[doc.ID], path)
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return out, nil
}

// slug normalises an HA alias so it is safe to use as a single filename
// segment. Keeps Chinese / Unicode letters; rewrites filesystem-significant
// separators only.
func slug(alias string) string {
	r := strings.NewReplacer("/", "_", "\\", "_", ":", "_")
	return r.Replace(alias)
}
