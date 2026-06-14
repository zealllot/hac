package helpers

import (
	"fmt"

	"github.com/zealllot/hac/internal/ha"
)

// ApplyReport summarises an apply run.
type ApplyReport struct {
	Created []string
	Skipped []string
	Failed  []string // "entity_id: reason"
}

// Applier pushes manifests back to HA.
type Applier struct {
	WS     *ha.WSClient
	Client *ha.Client
}

// Apply creates every helper in `byDomain` that does not already exist on HA.
// byDomain keys are the manifest file stems: the 9 collection domains plus
// "template_sensor".
func (a Applier) Apply(byDomain map[string]Manifest) ApplyReport {
	var rep ApplyReport
	for fileDomain, m := range byDomain {
		for objectID, cfg := range m {
			entityID := entityDomain(fileDomain) + "." + objectID
			if _, err := a.Client.GetState(entityID); err == nil {
				rep.Skipped = append(rep.Skipped, entityID)
				continue
			}
			if err := a.create(fileDomain, objectID, entityID, cfg); err != nil {
				rep.Failed = append(rep.Failed, fmt.Sprintf("%s: %v", entityID, err))
				continue
			}
			rep.Created = append(rep.Created, entityID)
		}
	}
	return rep
}

// entityDomain maps a manifest file stem to the entity domain. Collection files
// are named after their domain; template_sensor lands in the sensor domain.
func entityDomain(fileDomain string) string {
	if fileDomain == "template_sensor" {
		return "sensor"
	}
	return fileDomain
}

func (a Applier) create(fileDomain, objectID, entityID string, cfg map[string]any) error {
	if fileDomain == "template_sensor" {
		name, _ := cfg["name"].(string)
		entryID, err := a.Client.CreateTemplateSensor(name, map[string]any{
			"state":               cfg["state"],
			"unit_of_measurement": cfg["unit_of_measurement"],
			"device_class":        cfg["device_class"],
			"state_class":         cfg["state_class"],
		})
		if err != nil {
			return err
		}
		created, err := a.WS.ResolveEntityByConfigEntry(entryID)
		if err != nil {
			return err
		}
		if created != entityID {
			if err := a.WS.RenameEntityID(created, entityID); err != nil {
				return err
			}
		}
		if icon, _ := cfg["icon"].(string); icon != "" {
			_ = a.WS.SetEntityIcon(entityID, icon)
		}
		return nil
	}

	// Collection helper: create with config (drop nulls), then rename to object_id.
	clean := make(map[string]any, len(cfg))
	for k, v := range cfg {
		if v != nil {
			clean[k] = v
		}
	}
	created, err := a.WS.CreateCollectionHelper(fileDomain, clean)
	if err != nil {
		return err
	}
	if created != entityID {
		if err := a.WS.RenameEntityID(created, entityID); err != nil {
			return err
		}
	}
	return nil
}
