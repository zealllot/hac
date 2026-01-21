package ir

import (
	"fmt"
	"strings"

	"github.com/zealllot/hac/internal/ha"
)

type ValidationError struct {
	Field      string   `json:"field"`
	Message    string   `json:"message"`
	Suggestion []string `json:"suggestion,omitempty"`
}

type ValidationResult struct {
	Valid  bool              `json:"valid"`
	Errors []ValidationError `json:"errors,omitempty"`
}

type Validator struct {
	devices  map[string]ha.DeviceCapability
	services map[string][]string
}

func NewValidator(devices map[string]ha.DeviceCapability, services []ha.ServiceDomain) *Validator {
	serviceMap := make(map[string][]string)
	for _, svc := range services {
		var names []string
		for name := range svc.Services {
			names = append(names, name)
		}
		serviceMap[svc.Domain] = names
	}

	return &Validator{
		devices:  devices,
		services: serviceMap,
	}
}

func (v *Validator) ValidateAction(ir *ActionIR) ValidationResult {
	var errors []ValidationError

	if ir.Action == "" {
		errors = append(errors, ValidationError{
			Field:   "action",
			Message: "action is required",
		})
		return ValidationResult{Valid: false, Errors: errors}
	}

	switch ir.Action {
	case "call_service":
		errors = append(errors, v.validateCallService(ir)...)
	default:
		errors = append(errors, ValidationError{
			Field:      "action",
			Message:    fmt.Sprintf("unknown action: %s", ir.Action),
			Suggestion: []string{"call_service"},
		})
	}

	return ValidationResult{
		Valid:  len(errors) == 0,
		Errors: errors,
	}
}

func (v *Validator) validateCallService(ir *ActionIR) []ValidationError {
	var errors []ValidationError

	if ir.Service == "" {
		errors = append(errors, ValidationError{
			Field:   "service",
			Message: "service is required for call_service action",
		})
		return errors
	}

	parts := strings.SplitN(ir.Service, ".", 2)
	if len(parts) != 2 {
		errors = append(errors, ValidationError{
			Field:   "service",
			Message: "service must be in format domain.service (e.g., light.turn_on)",
		})
		return errors
	}

	domain := parts[0]
	service := parts[1]

	domainServices, ok := v.services[domain]
	if !ok {
		var availableDomains []string
		for d := range v.services {
			availableDomains = append(availableDomains, d)
		}
		errors = append(errors, ValidationError{
			Field:      "service",
			Message:    fmt.Sprintf("unknown domain: %s", domain),
			Suggestion: availableDomains,
		})
		return errors
	}

	serviceFound := false
	for _, s := range domainServices {
		if s == service {
			serviceFound = true
			break
		}
	}
	if !serviceFound {
		errors = append(errors, ValidationError{
			Field:      "service",
			Message:    fmt.Sprintf("unknown service: %s.%s", domain, service),
			Suggestion: domainServices,
		})
	}

	if ir.Target == "" {
		errors = append(errors, ValidationError{
			Field:   "target",
			Message: "target (entity_id) is required",
		})
	} else {
		if _, ok := v.devices[ir.Target]; !ok {
			var suggestions []string
			for entityID, dev := range v.devices {
				if dev.Domain == domain {
					suggestions = append(suggestions, entityID)
				}
			}
			errors = append(errors, ValidationError{
				Field:      "target",
				Message:    fmt.Sprintf("entity not found: %s", ir.Target),
				Suggestion: suggestions,
			})
		}
	}

	return errors
}

func (v *Validator) ValidateAutomation(ir *AutomationIR) ValidationResult {
	var errors []ValidationError

	if ir.Name == "" {
		errors = append(errors, ValidationError{
			Field:   "name",
			Message: "automation name is required",
		})
	}

	errors = append(errors, v.validateTrigger(&ir.Trigger)...)

	for i, cond := range ir.Conditions {
		condErrors := v.validateCondition(&cond)
		for j := range condErrors {
			condErrors[j].Field = fmt.Sprintf("conditions[%d].%s", i, condErrors[j].Field)
		}
		errors = append(errors, condErrors...)
	}

	if len(ir.Actions) == 0 {
		errors = append(errors, ValidationError{
			Field:   "actions",
			Message: "at least one action is required",
		})
	}

	for i, action := range ir.Actions {
		actionResult := v.ValidateAction(&action)
		for _, err := range actionResult.Errors {
			err.Field = fmt.Sprintf("actions[%d].%s", i, err.Field)
			errors = append(errors, err)
		}
	}

	return ValidationResult{
		Valid:  len(errors) == 0,
		Errors: errors,
	}
}

func (v *Validator) validateTrigger(t *Trigger) []ValidationError {
	var errors []ValidationError

	if t.Type == "" {
		errors = append(errors, ValidationError{
			Field:      "trigger.type",
			Message:    "trigger type is required",
			Suggestion: []string{"state", "time", "event"},
		})
		return errors
	}

	switch t.Type {
	case "state":
		if t.Entity == "" {
			errors = append(errors, ValidationError{
				Field:   "trigger.entity",
				Message: "entity is required for state trigger",
			})
		} else if _, ok := v.devices[t.Entity]; !ok {
			var suggestions []string
			for entityID := range v.devices {
				suggestions = append(suggestions, entityID)
			}
			errors = append(errors, ValidationError{
				Field:      "trigger.entity",
				Message:    fmt.Sprintf("entity not found: %s", t.Entity),
				Suggestion: suggestions[:min(10, len(suggestions))],
			})
		}
	case "time":
		if t.At == "" {
			errors = append(errors, ValidationError{
				Field:   "trigger.at",
				Message: "at is required for time trigger (e.g., '07:00:00')",
			})
		}
	}

	return errors
}

func (v *Validator) validateCondition(c *Condition) []ValidationError {
	var errors []ValidationError

	if c.Type == "" {
		errors = append(errors, ValidationError{
			Field:      "type",
			Message:    "condition type is required",
			Suggestion: []string{"state", "time"},
		})
		return errors
	}

	switch c.Type {
	case "state":
		if c.Entity == "" {
			errors = append(errors, ValidationError{
				Field:   "entity",
				Message: "entity is required for state condition",
			})
		} else if _, ok := v.devices[c.Entity]; !ok {
			errors = append(errors, ValidationError{
				Field:   "entity",
				Message: fmt.Sprintf("entity not found: %s", c.Entity),
			})
		}
	case "time":
		if c.After == "" && c.Before == "" {
			errors = append(errors, ValidationError{
				Field:   "after/before",
				Message: "at least one of after or before is required for time condition",
			})
		}
	}

	return errors
}
