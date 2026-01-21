package ha

type EntityState struct {
	EntityID    string         `json:"entity_id"`
	State       string         `json:"state"`
	Attributes  map[string]any `json:"attributes"`
	LastChanged string         `json:"last_changed"`
	LastUpdated string         `json:"last_updated"`
}

type ServiceDomain struct {
	Domain   string             `json:"domain"`
	Services map[string]Service `json:"services"`
}

type Service struct {
	Name        string                  `json:"name"`
	Description string                  `json:"description"`
	Fields      map[string]ServiceField `json:"fields"`
}

type ServiceField struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Example     any    `json:"example"`
}

type Config struct {
	LocationName string  `json:"location_name"`
	Latitude     float64 `json:"latitude"`
	Longitude    float64 `json:"longitude"`
	Elevation    int     `json:"elevation"`
	UnitSystem   struct {
		Length      string `json:"length"`
		Mass        string `json:"mass"`
		Temperature string `json:"temperature"`
		Volume      string `json:"volume"`
	} `json:"unit_system"`
	TimeZone   string   `json:"time_zone"`
	Components []string `json:"components"`
	Version    string   `json:"version"`
}

type DeviceCapability struct {
	EntityID   string   `json:"entity_id"`
	Domain     string   `json:"domain"`
	Name       string   `json:"name"`
	State      string   `json:"state"`
	Supports   []string `json:"supports"`
	Attributes []string `json:"attributes"`
}
