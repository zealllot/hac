package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/zealllot/hac/internal/category"
	"github.com/zealllot/hac/internal/cliflags"
	"github.com/zealllot/hac/internal/config"
	"github.com/zealllot/hac/internal/gitops"
	"github.com/zealllot/hac/internal/ha"
	"github.com/zealllot/hac/internal/logbook"
	"github.com/zealllot/hac/internal/render"
	"github.com/zealllot/hac/internal/syncer"
	"github.com/zealllot/hac/internal/timefmt"
	"gopkg.in/yaml.v3"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	sub := os.Args[1]
	if sub == "mcp" {
		fmt.Fprintln(os.Stderr, "Error: 'mcp' has been removed; hac is a CLI-only tool. See docs/adr/0001-cli-only.md")
		os.Exit(1)
	}
	if sub == "version" {
		fmt.Println("hac version 0.1.0")
		return
	}
	if sub == "init" {
		cmdInit()
		return
	}
	if sub == "deploy" {
		runDeploy(os.Args[2:])
		return
	}
	if sub == "history" {
		runHistory(os.Args[2:])
		return
	}

	flags, rest, err := cliflags.Parse(sub, os.Args[2:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	switch sub {
	case "devices":
		runCLI(flags.Timeout, func(c *ha.Client) error { return cmdDevices(c, flags.Format) })
	case "state":
		if len(rest) < 1 {
			fmt.Fprintln(os.Stderr, "Usage: hac state <entity_id> [<entity_id> ...]")
			os.Exit(1)
		}
		runCLI(flags.Timeout, func(c *ha.Client) error { return cmdState(c, flags.Format, rest) })
	case "call":
		if len(rest) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: hac call <domain> <service> <entity_id> [data_json]")
			os.Exit(1)
		}
		var data string
		if len(rest) > 3 {
			data = rest[3]
		}
		runCLI(flags.Timeout, func(c *ha.Client) error { return cmdCall(c, rest[0], rest[1], rest[2], data) })
	case "automations":
		runCLI(flags.Timeout, func(c *ha.Client) error { return cmdAutomations(c, flags.Format) })
	case "export":
		if len(rest) < 1 {
			fmt.Fprintln(os.Stderr, "Usage: hac export <output_dir>")
			os.Exit(1)
		}
		runCLI(flags.Timeout, func(c *ha.Client) error { return cmdExport(c, rest[0]) })
	case "sync":
		cmdSync(flags.Timeout)
	case "sync-config":
		cmdSyncConfig(flags.Timeout)
	default:
		printUsage()
		os.Exit(1)
	}
}

func getClient(timeout time.Duration) *ha.Client {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	c := ha.NewClient(cfg.HAURL, cfg.HAToken)
	c.SetTimeout(timeout)
	return c
}

func runCLI(timeout time.Duration, fn func(*ha.Client) error) {
	client := getClient(timeout)
	if err := fn(client); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runDeploy(args []string) {
	var createCategory bool
	var commitMsg string
	flags, rest, err := cliflags.ParseWith("deploy", args, func(fs *flag.FlagSet) {
		fs.BoolVar(&createCategory, "create-category", false, "auto-create missing HA category from directory name")
		fs.StringVar(&commitMsg, "commit", "", "after staging, also git-commit with the given message")
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if len(rest) < 1 {
		fmt.Fprintln(os.Stderr, `Usage: hac deploy [--create-category] [--commit "<msg>"] <file_or_dir>`)
		os.Exit(1)
	}
	runCLI(flags.Timeout, func(c *ha.Client) error {
		return cmdDeploy(c, rest[0], deployOpts{
			AutoCreateCategory: createCategory,
			CommitMessage:      commitMsg,
			Format:             flags.Format,
		})
	})
}

type deployOpts struct {
	AutoCreateCategory bool
	CommitMessage      string
	Format             string
}

func runHistory(args []string) {
	var sinceStr, untilStr string
	flags, rest, err := cliflags.ParseWith("history", args, func(fs *flag.FlagSet) {
		fs.StringVar(&sinceStr, "since", "24h", "duration (24h, 2h30m) or ISO-8601 timestamp")
		fs.StringVar(&untilStr, "until", "now", "duration or ISO-8601 timestamp; default now")
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if len(rest) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: hac history [--since 24h] [--until now] <entity_id>")
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	now := time.Now()
	start, err := timefmt.Parse(sinceStr, now)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: --since: %v\n", err)
		os.Exit(1)
	}
	end, err := timefmt.Parse(untilStr, now)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: --until: %v\n", err)
		os.Exit(1)
	}

	httpClient := &http.Client{Timeout: flags.Timeout}
	events, err := logbook.Query(httpClient, cfg.HAURL, cfg.HAToken, rest, start, end)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if err := render.Logbook(os.Stdout, events, flags.Format); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func cmdDevices(client *ha.Client, format string) error {
	devices, err := client.GetDevices()
	if err != nil {
		return err
	}
	return render.Devices(os.Stdout, devices, format)
}

func cmdState(client *ha.Client, format string, args []string) error {
	// Backward-compatibility: a single literal entity_id keeps the legacy
	// single-object response shape (no JSON array wrapping).
	if len(args) == 1 && !strings.ContainsAny(args[0], "*?") {
		state, err := client.GetState(args[0])
		if err != nil {
			return err
		}
		return render.State(os.Stdout, state, format)
	}

	// Multi-entity or wildcard: fetch all states once, then build the result
	// slice preserving caller order (with "not_found" placeholders).
	all, err := client.GetStates()
	if err != nil {
		return err
	}
	byID := make(map[string]ha.EntityState, len(all))
	for _, s := range all {
		byID[s.EntityID] = s
	}

	var out []ha.EntityState
	for _, a := range args {
		if strings.ContainsAny(a, "*?") {
			for _, s := range all {
				if matched, _ := filepath.Match(a, s.EntityID); matched {
					out = append(out, s)
				}
			}
			continue
		}
		if s, ok := byID[a]; ok {
			out = append(out, s)
		} else {
			out = append(out, ha.EntityState{EntityID: a, State: "not_found"})
		}
	}

	return render.States(os.Stdout, out, format)
}

func cmdCall(client *ha.Client, domain, service, entityID, dataJSON string) error {
	serviceData := map[string]any{
		"entity_id": entityID,
	}

	if dataJSON != "" {
		var extra map[string]any
		if err := json.Unmarshal([]byte(dataJSON), &extra); err != nil {
			return fmt.Errorf("invalid JSON data: %w", err)
		}
		for k, v := range extra {
			serviceData[k] = v
		}
	}

	if err := client.CallService(domain, service, serviceData); err != nil {
		return err
	}

	fmt.Printf("✓ Called %s.%s on %s\n", domain, service, entityID)
	return nil
}

func cmdAutomations(client *ha.Client, format string) error {
	automations, err := client.GetAutomations()
	if err != nil {
		return err
	}
	return render.Automations(os.Stdout, automations, format)
}

func cmdInit() {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Home Assistant URL (e.g., http://192.168.1.100:8123): ")
	haURL, _ := reader.ReadString('\n')
	haURL = strings.TrimSpace(haURL)

	fmt.Print("Long-lived access token: ")
	haToken, _ := reader.ReadString('\n')
	haToken = strings.TrimSpace(haToken)

	if haURL == "" || haToken == "" {
		fmt.Fprintln(os.Stderr, "Error: URL and token are required")
		os.Exit(1)
	}

	// Test connection
	fmt.Print("Testing connection... ")
	client := ha.NewClient(haURL, haToken)
	haCfg, err := client.GetConfig()
	if err != nil {
		fmt.Println("✗")
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✓ Connected to %s (HA %s)\n", haCfg.LocationName, haCfg.Version)

	// Ask for config repo path
	fmt.Print("Config repo path (e.g., ~/ha-config): ")
	configRepo, _ := reader.ReadString('\n')
	configRepo = strings.TrimSpace(configRepo)
	if strings.HasPrefix(configRepo, "~") {
		homeDir, _ := os.UserHomeDir()
		configRepo = filepath.Join(homeDir, configRepo[1:])
	}
	if configRepo != "" {
		configRepo, _ = filepath.Abs(configRepo)
	}

	if err := config.Save(&config.Config{
		HAURL:      haURL,
		HAToken:    haToken,
		ConfigRepo: configRepo,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving config: %v\n", err)
		os.Exit(1)
	}
	homeDir, _ := os.UserHomeDir()
	fmt.Printf("✓ Saved config to %s\n", filepath.Join(homeDir, ".hac.yaml"))
}

func cmdExport(client *ha.Client, outputDir string) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	automations, err := client.GetAutomations()
	if err != nil {
		return fmt.Errorf("get automations: %w", err)
	}

	exported := 0
	for _, a := range automations {
		state, err := client.GetState(a.EntityID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to get %s: %v\n", a.EntityID, err)
			continue
		}

		id := strings.TrimPrefix(a.EntityID, "automation.")

		automation := map[string]any{
			"id": id,
		}

		if alias, ok := state.Attributes["friendly_name"].(string); ok {
			automation["alias"] = alias
		}
		if mode, ok := state.Attributes["mode"].(string); ok {
			automation["mode"] = mode
		}

		for k, v := range state.Attributes {
			switch k {
			case "friendly_name", "mode", "id", "last_triggered":
				continue
			default:
				automation[k] = v
			}
		}

		data, err := yaml.Marshal(automation)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to marshal %s: %v\n", id, err)
			continue
		}

		filename := filepath.Join(outputDir, id+".yaml")
		if err := os.WriteFile(filename, data, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to write %s: %v\n", filename, err)
			continue
		}

		fmt.Printf("✓ Exported %s\n", filename)
		exported++
	}

	fmt.Printf("\nExported %d automations to %s\n", exported, outputDir)
	return nil
}

func cmdDeploy(client *ha.Client, path string, opts deployOpts) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat path: %w", err)
	}

	var files []string
	if info.IsDir() {
		entries, err := os.ReadDir(path)
		if err != nil {
			return fmt.Errorf("read directory: %w", err)
		}
		for _, e := range entries {
			if !e.IsDir() && (strings.HasSuffix(e.Name(), ".yaml") || strings.HasSuffix(e.Name(), ".yml")) {
				files = append(files, filepath.Join(path, e.Name()))
			}
		}
	} else {
		files = []string{path}
	}

	ws, err := client.NewWSClient()
	if err != nil {
		return fmt.Errorf("connect WebSocket: %w", err)
	}
	defer ws.Close()

	results := make([]render.DeployResult, 0, len(files))
	failed := 0
	for _, file := range files {
		r := deployOne(client, ws, file, opts)
		results = append(results, r)
		if r.Error != "" {
			failed++
		}
	}

	if rerr := render.Deploy(os.Stdout, results, opts.Format); rerr != nil {
		fmt.Fprintf(os.Stderr, "render: %v\n", rerr)
	}

	if failed > 0 {
		return fmt.Errorf("%d of %d file(s) failed", failed, len(files))
	}
	return nil
}

// deployOne handles a single YAML file end-to-end. Pre-flight category check
// happens BEFORE the HA push, so a missing category never leaves the
// automation in a half-deployed state on HA. Git staging happens AFTER push +
// assign so a failed deploy doesn't pollute the working tree.
func deployOne(client *ha.Client, ws *ha.WSClient, file string, opts deployOpts) render.DeployResult {
	r := render.DeployResult{File: file}

	data, err := os.ReadFile(file)
	if err != nil {
		r.Error = fmt.Sprintf("read: %v", err)
		return r
	}
	var automation map[string]any
	if err := yaml.Unmarshal(data, &automation); err != nil {
		r.Error = fmt.Sprintf("parse YAML: %v", err)
		return r
	}

	// Path-based category (single source of truth; see ADR-0003).
	cat := category.Resolve(file)
	r.Category = cat
	var categoryID string
	if cat != "" {
		categoryID, err = category.EnsureExists(ws, cat, opts.AutoCreateCategory)
		if err != nil {
			r.Error = err.Error()
			return r
		}
	}

	if err := client.CreateAutomation(automation); err != nil {
		r.Error = fmt.Sprintf("push HA: %v", err)
		return r
	}

	alias, _ := automation["alias"].(string)
	if alias == "" {
		alias = filepath.Base(file)
	}
	r.Alias = alias
	if id, ok := automation["id"].(string); ok {
		r.AutomationID = id
	}

	if categoryID != "" && r.AutomationID != "" {
		entityID := findEntityIDByAutomationID(client, r.AutomationID, alias)
		r.EntityID = entityID
		if err := category.Assign(ws, entityID, categoryID); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to assign category for %s: %v\n", alias, err)
		}
	}

	// Git staging — non-fatal: HA is already updated, so just warn on failure.
	if err := gitops.Add(file); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: HA updated but git add failed for %s: %v\n", file, err)
	} else {
		r.GitAdded = true
	}

	// Commit only if --commit was passed.
	if opts.CommitMessage != "" && r.GitAdded {
		if err := gitops.Commit(filepath.Dir(file), opts.CommitMessage); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: git commit failed: %v\n", err)
		} else {
			r.GitCommitted = true
		}
	}

	return r
}

// findEntityIDByAutomationID looks up the HA-assigned entity_id (which may
// be auto-numbered when a new automation is pushed) by matching the `id`
// field in the YAML against HA's automation registry.
// Falls back to a guess derived from the alias if no match is found.
func findEntityIDByAutomationID(client *ha.Client, automationID, alias string) string {
	guess := "automation." + strings.ReplaceAll(strings.ToLower(alias), " ", "_")
	automations, err := client.GetAutomations()
	if err != nil {
		return guess
	}
	for _, a := range automations {
		if aid, ok := a.Attributes["id"].(string); ok && aid == automationID {
			return a.EntityID
		}
	}
	return guess
}

func cmdSync(timeout time.Duration) {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if cfg.ConfigRepo == "" {
		fmt.Fprintln(os.Stderr, "Error: config_repo not set. Run 'hac init' to configure.")
		os.Exit(1)
	}

	client := ha.NewClient(cfg.HAURL, cfg.HAToken)
	client.SetTimeout(timeout)
	ws, err := client.NewWSClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: connect WebSocket: %v\n", err)
		os.Exit(1)
	}
	defer ws.Close()

	if err := os.MkdirAll(filepath.Join(cfg.ConfigRepo, "automations"), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating automations dir: %v\n", err)
		os.Exit(1)
	}

	s := &syncer.Syncer{HA: client, WS: ws, ConfigRepo: cfg.ConfigRepo}
	report, err := s.Sync()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Synced %d automations (%d updates, %d orphans deleted, %d local-only)\n",
		len(report.Created), len(report.Updated), len(report.DeletedOrphans), len(report.WarnLocalOnly))
	for _, p := range report.DeletedOrphans {
		fmt.Printf("  ✗ deleted orphan: %s\n", p)
	}
	for _, p := range report.WarnLocalOnly {
		fmt.Printf("  ! local-only (kept): %s\n", p)
	}

	// Git add + commit, preserving the historical "Sync automations from HA" message.
	cmd := exec.Command("git", "add", "automations/")
	cmd.Dir = cfg.ConfigRepo
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: git add failed: %v\n", err)
		return
	}
	cmd = exec.Command("git", "commit", "-m", "Sync automations from Home Assistant")
	cmd.Dir = cfg.ConfigRepo
	output, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(output), "nothing to commit") {
			fmt.Println("✓ No changes to commit")
		} else {
			fmt.Fprintf(os.Stderr, "Warning: git commit failed: %v\n", err)
		}
		return
	}
	fmt.Println("✓ Committed changes to git")
}

func cmdSyncConfig(timeout time.Duration) {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	client := ha.NewClient(cfg.HAURL, cfg.HAToken)
	client.SetTimeout(timeout)
	states, err := client.GetStates()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting states: %v\n", err)
		os.Exit(1)
	}

	// Filter input_number entities
	configMap := make(map[string]any)
	count := 0
	for _, state := range states {
		if strings.HasPrefix(state.EntityID, "input_number.") {
			if editable, ok := state.Attributes["editable"].(bool); ok && editable {
				// Use current state value as initial if original initial is nil
				var initial any
				if state.Attributes["initial"] != nil {
					initial = state.Attributes["initial"]
				} else if stateVal := state.State; stateVal != "" && stateVal != "unknown" && stateVal != "unavailable" {
					// Parse state value as float
					var val float64
					if _, err := fmt.Sscanf(stateVal, "%f", &val); err == nil {
						initial = val
					}
				}

				entry := map[string]any{
					"name":    state.Attributes["friendly_name"],
					"min":     state.Attributes["min"],
					"max":     state.Attributes["max"],
					"step":    state.Attributes["step"],
					"initial": initial,
				}
				if unit, ok := state.Attributes["unit_of_measurement"].(string); ok && unit != "" {
					entry["unit_of_measurement"] = unit
				}
				if icon, ok := state.Attributes["icon"].(string); ok && icon != "" {
					entry["icon"] = icon
				}
				key := strings.TrimPrefix(state.EntityID, "input_number.")
				configMap[key] = entry
				count++
			}
		}
	}

	if count == 0 {
		fmt.Println("No editable input_number entities found")
		return
	}

	// Write to input_number.yaml
	filePath := filepath.Join(cfg.ConfigRepo, "input_number.yaml")
	data, err := yaml.Marshal(configMap)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling yaml: %v\n", err)
		os.Exit(1)
	}

	header := "# 全局变量配置 - 由 hac sync-config 自动生成\n\n"
	if err := os.WriteFile(filePath, []byte(header+string(data)), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Synced %d input_number entities to %s\n", count, filePath)

	// Git add and commit
	cmd := exec.Command("git", "add", filePath)
	cmd.Dir = cfg.ConfigRepo
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: git add failed: %v\n", err)
		return
	}

	cmd = exec.Command("git", "commit", "-m", fmt.Sprintf("Sync input_number config (%d items)", count))
	cmd.Dir = cfg.ConfigRepo
	output, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(output), "nothing to commit") {
			fmt.Println("✓ No changes to commit")
		} else {
			fmt.Fprintf(os.Stderr, "Warning: git commit failed: %v\n", err)
		}
		return
	}

	fmt.Println("✓ Committed changes to git")
}

func printUsage() {
	fmt.Println(`hac - Home Assistant CLI & MCP Server

Usage:
  hac init                                   Configure Windsurf MCP integration
  hac mcp                                    Start MCP server (for Windsurf)
  hac devices                                List all devices
  hac state <entity_id>                      Get device state
  hac call <domain> <service> <entity_id> [data]   Call a service
  hac automations                            List all automations
  hac export <output_dir>                    Export automations to YAML files
  hac deploy <file_or_dir>                   Deploy YAML automations to HA
  hac sync                                   Sync HA automations to config repo and commit
  hac version                                Show version

Examples:
  hac init
  hac devices
  hac state light.living_room
  hac call light turn_on light.living_room
  hac call light turn_on light.living_room '{"brightness_pct":50}'
  hac automations
  hac export ./automations
  hac deploy ./automations/living_room.yaml
  hac deploy ./automations/
  hac sync

Environment variables:
  HA_URL        Home Assistant URL (e.g., http://192.168.1.100:8123)
  HA_TOKEN      Long-lived access token`)
}
