package ir

type ActionIR struct {
	Action  string         `json:"action"`
	Service string         `json:"service,omitempty"`
	Target  string         `json:"target,omitempty"`
	Data    map[string]any `json:"data,omitempty"`
}

type AutomationIR struct {
	Name        string       `json:"name"`
	Trigger     Trigger      `json:"trigger"`
	Conditions  []Condition  `json:"conditions,omitempty"`
	Actions     []ActionIR   `json:"actions"`
	Constraints *Constraints `json:"constraints,omitempty"`
}

type Trigger struct {
	Type   string `json:"type"`
	Entity string `json:"entity,omitempty"`
	To     string `json:"to,omitempty"`
	From   string `json:"from,omitempty"`
	At     string `json:"at,omitempty"`
}

type Condition struct {
	Type   string `json:"type"`
	Entity string `json:"entity,omitempty"`
	State  string `json:"state,omitempty"`
	After  string `json:"after,omitempty"`
	Before string `json:"before,omitempty"`
}

type Constraints struct {
	Cooldown string `json:"cooldown,omitempty"`
	Mode     string `json:"mode,omitempty"`
}
