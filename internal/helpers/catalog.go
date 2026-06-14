// Package helpers captures Home Assistant UI helpers into the config repo and
// applies them back. It spans two HA storage mechanisms: storage-collection
// helpers (input_*, counter, timer, schedule) and config-entry helpers
// (template sensors).
package helpers

// CollectionDomains lists the storage-collection helper domains, each managed
// through uniform <domain>/list and <domain>/create WS commands.
func CollectionDomains() []string {
	return []string{
		"input_boolean", "input_number", "input_text", "input_select",
		"input_button", "input_datetime", "counter", "timer", "schedule",
	}
}

// ConfigEntryDomains lists the config-entry helper domains hac can round-trip.
// Only template today; extend as more config-flow drivers are added.
func ConfigEntryDomains() []string {
	return []string{"template"}
}
