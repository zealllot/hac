package ir

import "encoding/json"

type ActionIR struct {
	Action  string         `json:"action"`
	Service string         `json:"service,omitempty"`
	Target  string         `json:"target,omitempty"`
	Data    map[string]any `json:"data,omitempty"`
}

type AutomationIR struct {
	Name        string       `json:"name"`
	Triggers    []Trigger    `json:"-"` // 支持单个或多个触发器
	Conditions  []Condition  `json:"conditions,omitempty"`
	Actions     []ActionIR   `json:"actions"`
	Constraints *Constraints `json:"constraints,omitempty"`
	Labels      []string     `json:"labels,omitempty"`
}

// UnmarshalJSON 自定义解析，支持 trigger 为单个对象或数组
func (a *AutomationIR) UnmarshalJSON(data []byte) error {
	type Alias AutomationIR
	aux := &struct {
		Trigger json.RawMessage `json:"trigger"`
		*Alias
	}{
		Alias: (*Alias)(a),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	if len(aux.Trigger) == 0 {
		return nil
	}

	// 尝试解析为数组
	var triggers []Trigger
	if err := json.Unmarshal(aux.Trigger, &triggers); err == nil {
		a.Triggers = triggers
		return nil
	}

	// 尝试解析为单个对象
	var trigger Trigger
	if err := json.Unmarshal(aux.Trigger, &trigger); err != nil {
		return err
	}
	a.Triggers = []Trigger{trigger}
	return nil
}

type Trigger struct {
	Type   string     `json:"type"`
	Entity string     `json:"entity,omitempty"`
	To     string     `json:"to,omitempty"`
	From   string     `json:"from,omitempty"`
	At     string     `json:"at,omitempty"`
	For    *ForConfig `json:"for,omitempty"`
}

type ForConfig struct {
	Hours   int `json:"hours,omitempty"`
	Minutes int `json:"minutes,omitempty"`
	Seconds int `json:"seconds,omitempty"`
}

type Condition struct {
	Type   string   `json:"type"`
	Entity string   `json:"entity,omitempty"`
	State  string   `json:"state,omitempty"`
	After  string   `json:"after,omitempty"`
	Before string   `json:"before,omitempty"`
	Above  *float64 `json:"above,omitempty"`
	Below  *float64 `json:"below,omitempty"`
}

type Constraints struct {
	Cooldown string `json:"cooldown,omitempty"`
	Mode     string `json:"mode,omitempty"`
}
