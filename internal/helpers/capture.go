package helpers

import (
	"fmt"
	"strings"

	"github.com/zealllot/hac/internal/ha"
)

// Capturer is the subset of HA client behaviour Capture needs.
type Capturer struct {
	WS     *ha.WSClient
	Client *ha.Client
}

// Capture reads every UI helper from HA, returning domain -> Manifest. Per-domain
// failures are collected as warnings (returned), never abort the whole capture.
func (c Capturer) Capture() (map[string]Manifest, []string) {
	out := make(map[string]Manifest)
	var warns []string

	for _, domain := range CollectionDomains() {
		items, err := c.WS.ListCollectionHelpers(domain)
		if err != nil {
			warns = append(warns, fmt.Sprintf("list %s: %v", domain, err))
			continue
		}
		if m := FromCollectionItems(items); len(m) > 0 {
			out[domain] = m
		}
	}

	for _, domain := range ConfigEntryDomains() {
		if domain != "template" {
			continue // only template is supported today (see catalog.go)
		}
		m, ws := c.captureTemplates()
		warns = append(warns, ws...)
		if len(m) > 0 {
			out["template_sensor"] = m
		}
	}

	return out, warns
}

// captureTemplates reads template sensors: entry title -> name, options flow ->
// state/unit/device_class/state_class, entity registry -> object_id + icon.
func (c Capturer) captureTemplates() (Manifest, []string) {
	m := Manifest{}
	var warns []string

	entries, err := c.Client.GetConfigEntriesByDomain("template")
	if err != nil {
		return m, []string{fmt.Sprintf("list template entries: %v", err)}
	}

	regByEntry := map[string]ha.EntityRegistryEntry{}
	if reg, err := c.WS.GetEntityRegistry(); err == nil {
		for _, e := range reg {
			if e.ConfigEntryID != "" {
				regByEntry[e.ConfigEntryID] = e
			}
		}
	}

	for _, e := range entries {
		ent, ok := regByEntry[e.EntryID]
		if !ok || !strings.HasPrefix(ent.EntityID, "sensor.") {
			warns = append(warns, fmt.Sprintf("template %s: no sensor entity for entry", e.Title))
			continue
		}
		objectID := strings.TrimPrefix(ent.EntityID, "sensor.")

		opts, err := c.Client.ReadConfigEntryOptions(e.EntryID)
		if err != nil {
			warns = append(warns, fmt.Sprintf("read options %s: %v", e.Title, err))
			continue
		}
		cfg := map[string]any{"name": e.Title}
		for _, k := range []string{"state", "unit_of_measurement", "device_class", "state_class"} {
			if v, ok := opts[k]; ok && v != nil && v != "" {
				cfg[k] = v
			}
		}
		if ent.Icon != "" {
			cfg["icon"] = ent.Icon
		}
		m[objectID] = cfg
	}
	return m, warns
}
