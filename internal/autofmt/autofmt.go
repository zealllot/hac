// Package autofmt provides the single canonical serialization (format "F") for
// Home Assistant automation YAML. hac fmt, hac sync, and hac deploy all route
// through FormatAutomation so files never diverge in formatting. F orders the
// top-level keys the way automations are authored (see CLAUDE.md) and is
// idempotent.
package autofmt

import (
	"bytes"
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"
)

// automationKeyOrder is the canonical top-level key order. Keys not listed are
// appended afterwards in alphabetical order.
var automationKeyOrder = []string{
	"alias", "id", "description", "triggers", "trigger",
	"conditions", "condition", "actions", "action", "mode", "max", "variables",
}

// FormatAutomation serializes an automation config to canonical YAML (format F).
func FormatAutomation(config map[string]any) ([]byte, error) {
	node, err := orderedMapNode(config)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(node); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// orderedMapNode builds a YAML mapping node whose top-level keys follow
// automationKeyOrder (then alphabetical for the rest). Values are encoded with
// yaml's defaults (nested maps end up alphabetical, which is fine and stable).
func orderedMapNode(m map[string]any) (*yaml.Node, error) {
	rank := make(map[string]int, len(automationKeyOrder))
	for i, k := range automationKeyOrder {
		rank[k] = i
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		ri, oki := rank[keys[i]]
		rj, okj := rank[keys[j]]
		if oki && okj {
			return ri < rj
		}
		if oki != okj {
			return oki
		}
		return keys[i] < keys[j]
	})
	out := &yaml.Node{Kind: yaml.MappingNode}
	for _, k := range keys {
		kn := &yaml.Node{Kind: yaml.ScalarNode, Value: k}
		vn := &yaml.Node{}
		if err := vn.Encode(m[k]); err != nil {
			return nil, fmt.Errorf("encode key %q: %w", k, err)
		}
		out.Content = append(out.Content, kn, vn)
	}
	return out, nil
}

// FormatFile reads, canonicalizes, and rewrites a file in place. Returns whether
// the file content changed. A file already in F is left byte-identical.
func FormatFile(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	formatted, err := formatBytes(data, path)
	if err != nil {
		return false, err
	}
	if bytes.Equal(data, formatted) {
		return false, nil
	}
	if err := os.WriteFile(path, formatted, 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// IsFormatted reports whether a file is already in canonical form F.
func IsFormatted(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	formatted, err := formatBytes(data, path)
	if err != nil {
		return false, err
	}
	return bytes.Equal(data, formatted), nil
}

func formatBytes(data []byte, path string) ([]byte, error) {
	var config map[string]any
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return FormatAutomation(config)
}
