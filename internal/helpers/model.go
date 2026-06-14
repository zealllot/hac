package helpers

// Manifest is one helper-type file: object_id -> config map (config never
// contains an "id" key; the object_id is the map key).
type Manifest map[string]map[string]any
