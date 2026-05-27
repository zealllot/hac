package render

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/zealllot/hac/internal/ha"
	"github.com/zealllot/hac/internal/logbook"
	"github.com/zealllot/hac/internal/search"
)

// Devices writes a device list as JSON (default) or aligned table.
func Devices(w io.Writer, devs map[string]ha.DeviceCapability, format string) error {
	type row struct {
		EntityID string `json:"entity_id"`
		State    string `json:"state"`
		Name     string `json:"name"`
		Domain   string `json:"domain,omitempty"`
		Area     string `json:"area,omitempty"`
	}
	rows := make([]row, 0, len(devs))
	for id, d := range devs {
		state := d.State
		if state == "" {
			state = "-"
		}
		name := d.Name
		if name == "" {
			name = id
		}
		rows = append(rows, row{EntityID: id, State: state, Name: name, Domain: d.Domain, Area: d.Area})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].EntityID < rows[j].EntityID })

	switch format {
	case "json":
		return writeJSON(w, rows)
	case "table":
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "entity_id\tstate\tname")
		for _, r := range rows {
			fmt.Fprintf(tw, "%s\t%s\t%s\n", r.EntityID, r.State, r.Name)
		}
		return tw.Flush()
	default:
		return fmt.Errorf("unknown format %q", format)
	}
}

// State writes a single entity state as JSON or key-value table.
func State(w io.Writer, s *ha.EntityState, format string) error {
	switch format {
	case "json":
		return writeJSON(w, s)
	case "table":
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintf(tw, "entity_id\t%s\n", s.EntityID)
		fmt.Fprintf(tw, "state\t%s\n", s.State)
		keys := make([]string, 0, len(s.Attributes))
		for k := range s.Attributes {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(tw, "%s\t%v\n", k, s.Attributes[k])
		}
		return tw.Flush()
	default:
		return fmt.Errorf("unknown format %q", format)
	}
}

// States renders a list of entity states as a JSON array or a stacked
// key-value table. Used by `hac state` when multiple entities or a wildcard
// are passed.
func States(w io.Writer, states []ha.EntityState, format string) error {
	switch format {
	case "json":
		return writeJSON(w, states)
	case "table":
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		for i, s := range states {
			if i > 0 {
				fmt.Fprintln(tw, "---")
			}
			fmt.Fprintf(tw, "entity_id\t%s\n", s.EntityID)
			fmt.Fprintf(tw, "state\t%s\n", s.State)
			keys := make([]string, 0, len(s.Attributes))
			for k := range s.Attributes {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				fmt.Fprintf(tw, "%s\t%v\n", k, s.Attributes[k])
			}
		}
		return tw.Flush()
	default:
		return fmt.Errorf("unknown format %q", format)
	}
}

// DeployResult is the per-file outcome emitted by `hac deploy`.
type DeployResult struct {
	File         string `json:"file"`
	Alias        string `json:"alias,omitempty"`
	Category     string `json:"category,omitempty"`
	AutomationID string `json:"automation_id,omitempty"`
	EntityID     string `json:"entity_id,omitempty"`
	GitAdded     bool   `json:"git_added"`
	GitCommitted bool   `json:"git_committed"`
	Error        string `json:"error,omitempty"`
}

// Deploy writes the per-file deploy results as a JSON array or aligned table.
func Deploy(w io.Writer, results []DeployResult, format string) error {
	switch format {
	case "json":
		return writeJSON(w, results)
	case "table":
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "file\talias\tcategory\tgit_added\tgit_committed\tstatus")
		for _, r := range results {
			status := "OK"
			if r.Error != "" {
				status = r.Error
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%v\t%v\t%s\n", r.File, r.Alias, r.Category, r.GitAdded, r.GitCommitted, status)
		}
		return tw.Flush()
	default:
		return fmt.Errorf("unknown format %q", format)
	}
}

// Logbook writes a logbook event list as JSON or aligned table.
// Table format formats `When` in time.Local for human readability.
func Logbook(w io.Writer, events []logbook.Event, format string) error {
	switch format {
	case "json":
		return writeJSON(w, events)
	case "table":
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "when\tstate\tentity_id\tname\tby")
		for _, e := range events {
			by := e.ContextName
			if by == "" {
				by = "—"
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
				e.When.In(time.Local).Format("2006-01-02 15:04:05"),
				e.State, e.EntityID, e.Name, by)
		}
		return tw.Flush()
	default:
		return fmt.Errorf("unknown format %q", format)
	}
}

// Matches writes search/area results as JSON array or aligned 4-column table.
func Matches(w io.Writer, matches []search.Match, format string) error {
	sort.Slice(matches, func(i, j int) bool { return matches[i].EntityID < matches[j].EntityID })
	switch format {
	case "json":
		return writeJSON(w, matches)
	case "table":
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "entity_id\tstate\tarea\tfriendly_name")
		for _, m := range matches {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", m.EntityID, m.State, m.Area, m.FriendlyName)
		}
		return tw.Flush()
	default:
		return fmt.Errorf("unknown format %q", format)
	}
}

// Automations writes the automation list as JSON or aligned table.
func Automations(w io.Writer, autos []ha.EntityState, format string) error {
	type row struct {
		ID    string `json:"id"`
		State string `json:"state"`
		Name  string `json:"name"`
	}
	rows := make([]row, 0, len(autos))
	for _, a := range autos {
		id := a.EntityID
		if len(id) > len("automation.") && id[:len("automation.")] == "automation." {
			id = id[len("automation."):]
		}
		name, _ := a.Attributes["friendly_name"].(string)
		rows = append(rows, row{ID: id, State: a.State, Name: name})
	}

	switch format {
	case "json":
		return writeJSON(w, rows)
	case "table":
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "id\tstate\tname")
		for _, r := range rows {
			fmt.Fprintf(tw, "%s\t%s\t%s\n", r.ID, r.State, r.Name)
		}
		return tw.Flush()
	default:
		return fmt.Errorf("unknown format %q", format)
	}
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
