package ir

import (
	"fmt"
	"strings"
)

type HAAutomation struct {
	ID          string           `json:"id,omitempty"`
	Alias       string           `json:"alias"`
	Description string           `json:"description,omitempty"`
	Mode        string           `json:"mode,omitempty"`
	Trigger     []map[string]any `json:"trigger"`
	Condition   []map[string]any `json:"condition,omitempty"`
	Action      []map[string]any `json:"action"`
	Labels      []string         `json:"labels,omitempty"`
}

func CompileAutomation(ir *AutomationIR) (*HAAutomation, error) {
	ha := &HAAutomation{
		Alias: ir.Name,
	}

	if ir.Constraints != nil && ir.Constraints.Mode != "" {
		ha.Mode = ir.Constraints.Mode
	} else {
		ha.Mode = "single"
	}

	trigger, err := compileTrigger(&ir.Trigger)
	if err != nil {
		return nil, fmt.Errorf("compile trigger: %w", err)
	}
	ha.Trigger = []map[string]any{trigger}

	for _, cond := range ir.Conditions {
		haCond, err := compileCondition(&cond)
		if err != nil {
			return nil, fmt.Errorf("compile condition: %w", err)
		}
		ha.Condition = append(ha.Condition, haCond)
	}

	for _, action := range ir.Actions {
		haAction, err := compileAction(&action)
		if err != nil {
			return nil, fmt.Errorf("compile action: %w", err)
		}
		ha.Action = append(ha.Action, haAction)
	}

	if len(ir.Labels) > 0 {
		ha.Labels = ir.Labels
	}

	return ha, nil
}

func compileTrigger(t *Trigger) (map[string]any, error) {
	result := make(map[string]any)

	switch t.Type {
	case "state":
		result["platform"] = "state"
		result["entity_id"] = t.Entity
		if t.To != "" {
			result["to"] = t.To
		}
		if t.From != "" {
			result["from"] = t.From
		}

	case "time":
		result["platform"] = "time"
		result["at"] = t.At

	case "event":
		result["platform"] = "event"
		result["event_type"] = t.Entity

	default:
		return nil, fmt.Errorf("unsupported trigger type: %s", t.Type)
	}

	return result, nil
}

func compileCondition(c *Condition) (map[string]any, error) {
	result := make(map[string]any)

	switch c.Type {
	case "state":
		result["condition"] = "state"
		result["entity_id"] = c.Entity
		result["state"] = c.State

	case "time":
		result["condition"] = "time"
		if c.After != "" {
			result["after"] = c.After
		}
		if c.Before != "" {
			result["before"] = c.Before
		}

	case "numeric_state":
		result["condition"] = "numeric_state"
		result["entity_id"] = c.Entity
		if c.Above != nil {
			result["above"] = *c.Above
		}
		if c.Below != nil {
			result["below"] = *c.Below
		}

	default:
		return nil, fmt.Errorf("unsupported condition type: %s", c.Type)
	}

	return result, nil
}

func compileAction(a *ActionIR) (map[string]any, error) {
	result := make(map[string]any)

	switch a.Action {
	case "call_service":
		parts := strings.SplitN(a.Service, ".", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid service format: %s", a.Service)
		}

		result["service"] = a.Service
		result["target"] = map[string]any{
			"entity_id": a.Target,
		}
		if len(a.Data) > 0 {
			result["data"] = a.Data
		}

	default:
		return nil, fmt.Errorf("unsupported action type: %s", a.Action)
	}

	return result, nil
}
