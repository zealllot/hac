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

	automationsDir := filepath.Join(cfg.ConfigRepo, "automations")
	if err := os.MkdirAll(automationsDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating automations dir: %v\n", err)
		os.Exit(1)
	}

	automations, err := client.GetAutomations()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting automations: %v\n", err)
		os.Exit(1)
	}

	// Track synced files by group for README generation
	groupFiles := make(map[string][]string)
	synced := 0

	for _, a := range automations {
		// Get automation config from HA API
		id, _ := a.Attributes["id"].(string)
		if id == "" {
			continue
		}

		config, err := client.GetAutomationConfig(id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to get config for %s: %v\n", a.EntityID, err)
			continue
		}

		alias, _ := config["alias"].(string)
		if alias == "" {
			alias = strings.TrimPrefix(a.EntityID, "automation.")
		}

		// Determine group based on alias
		group := getAutomationGroup(alias)

		// Create group directory
		groupDir := filepath.Join(automationsDir, group)
		if err := os.MkdirAll(groupDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to create group dir %s: %v\n", group, err)
			continue
		}

		data, err := yaml.Marshal(config)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to marshal %s: %v\n", alias, err)
			continue
		}

		filename := filepath.Join(groupDir, alias+".yaml")
		if err := os.WriteFile(filename, data, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to write %s: %v\n", filename, err)
			continue
		}

		groupFiles[group] = append(groupFiles[group], alias)
		synced++
	}

	// Generate README for each group
	for group := range groupFiles {
		groupDir := filepath.Join(automationsDir, group)
		if err := generateGroupREADME(groupDir, group); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to generate README for %s: %v\n", group, err)
		}
	}

	fmt.Printf("✓ Synced %d automations to %d groups\n", synced, len(groupFiles))
	for group, files := range groupFiles {
		fmt.Printf("  - %s: %d 个\n", group, len(files))
	}

	// Git add and commit
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

// getAutomationGroup determines the group/category for an automation based on its alias
func getAutomationGroup(alias string) string {
	// Define group patterns - order matters, more specific patterns first
	patterns := map[string][]string{
		"人来灯亮":    {"_有人_开灯", "_有人移动_开灯"},
		"人走灯灭":    {"_无人_关灯", "_无人5分钟_关灯"},
		"热水器":     {"热水器"},
		"马桶换气":    {"_坐马桶_开换气", "_无人_关换气"},
		"睡眠模式":    {"睡眠模式"},
		"光暗灯亮":    {"_光暗_开灯"},
		"衣柜灯":     {"衣柜开门", "衣柜关门", "衣柜超时"},
		"洗澡模式":    {"洗澡模式", "浴霸"},
		"全屋模式":    {"全屋_"},
		"iPad自动化": {"iPad"},
	}

	for group, suffixes := range patterns {
		for _, suffix := range suffixes {
			if strings.Contains(alias, suffix) {
				return group
			}
		}
	}

	return "其他"
}

// generateGroupREADME generates a README.md file for a group directory
func generateGroupREADME(groupDir, groupName string) error {
	entries, err := os.ReadDir(groupDir)
	if err != nil {
		return err
	}

	// Mode explanations
	modeNames := map[string]string{
		"single":   "单次执行",
		"restart":  "重新开始",
		"queued":   "排队执行",
		"parallel": "并行执行",
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# %s\n\n", groupName))

	var count int
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		count++
	}
	sb.WriteString(fmt.Sprintf("本目录包含 %d 个自动化配置。\n\n", count))

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}

		filePath := filepath.Join(groupDir, e.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		var config map[string]any
		if err := yaml.Unmarshal(data, &config); err != nil {
			continue
		}

		alias, _ := config["alias"].(string)
		mode, _ := config["mode"].(string)
		if mode == "" {
			mode = "single"
		}
		modeName := modeNames[mode]
		if modeName == "" {
			modeName = mode
		}

		sb.WriteString(fmt.Sprintf("## %s\n\n", alias))
		sb.WriteString(fmt.Sprintf("- **文件**: `%s`\n", e.Name()))
		sb.WriteString(fmt.Sprintf("- **模式**: %s\n", modeName))

		// Extract detailed trigger info
		triggerDetail := extractTriggerDetail(config)
		sb.WriteString(fmt.Sprintf("- **触发条件**: %s\n", triggerDetail))

		// Extract detailed action info
		actionDetail := extractActionDetail(config)
		sb.WriteString(fmt.Sprintf("- **执行动作**:\n%s", actionDetail))

		sb.WriteString("\n")
	}

	readmePath := filepath.Join(groupDir, "README.md")
	return os.WriteFile(readmePath, []byte(sb.String()), 0644)
}

// extractTriggerDetail extracts detailed trigger information
func extractTriggerDetail(config map[string]any) string {
	triggers, ok := config["triggers"].([]any)
	if !ok {
		triggers, ok = config["trigger"].([]any)
	}
	if !ok || len(triggers) == 0 {
		return "未配置触发器"
	}

	var parts []string
	for _, t := range triggers {
		trigger, ok := t.(map[string]any)
		if !ok {
			continue
		}

		platform, _ := trigger["platform"].(string)
		// Also check "trigger" key for platform (HA uses both)
		if platform == "" {
			platform, _ = trigger["trigger"].(string)
		}
		// Handle entity_id as string or array
		entityID, _ := trigger["entity_id"].(string)
		if entityID == "" {
			if entityIDs, ok := trigger["entity_id"].([]any); ok && len(entityIDs) > 0 {
				entityID, _ = entityIDs[0].(string)
			}
		}
		to, _ := trigger["to"].(string)
		// Check for attribute-based triggers (like virtual events)
		attribute, _ := trigger["attribute"].(string)

		switch platform {
		case "state":
			entityName := extractEntityNameDetail(entityID)
			// For virtual event triggers
			if attribute != "" && strings.Contains(entityID, "virtual_event") {
				parts = append(parts, fmt.Sprintf("当收到语音指令「%s」时", to))
			} else if to == "on" {
				parts = append(parts, fmt.Sprintf("当 %s 检测到有人时", entityName))
			} else if to == "off" {
				forDuration := ""
				if forMap, ok := trigger["for"].(map[string]any); ok {
					if mins, ok := forMap["minutes"].(int); ok {
						forDuration = fmt.Sprintf(" %d分钟", mins)
					}
				}
				parts = append(parts, fmt.Sprintf("当 %s 无人%s后", entityName, forDuration))
			} else if to == "1.0" || to == "1" {
				parts = append(parts, fmt.Sprintf("当 %s 开启时", entityName))
			} else if to == "0.0" || to == "0" {
				parts = append(parts, fmt.Sprintf("当 %s 关闭时", entityName))
			} else if to != "" {
				parts = append(parts, fmt.Sprintf("当 %s 变为 %s 时", entityName, to))
			} else {
				parts = append(parts, fmt.Sprintf("当 %s 状态变化时", entityName))
			}
		case "time":
			at, _ := trigger["at"].(string)
			parts = append(parts, fmt.Sprintf("每天 %s", at))
		case "numeric_state":
			entityName := extractEntityNameDetail(entityID)
			below, _ := trigger["below"].(string)
			above, _ := trigger["above"].(string)
			if below != "" {
				parts = append(parts, fmt.Sprintf("当 %s 低于 %s 时", entityName, below))
			} else if above != "" {
				parts = append(parts, fmt.Sprintf("当 %s 高于 %s 时", entityName, above))
			}
		}
	}

	if len(parts) == 0 {
		return "未配置触发器"
	}
	return strings.Join(parts, "；")
}

// extractActionDetail extracts detailed action information
func extractActionDetail(config map[string]any) string {
	actions, ok := config["actions"].([]any)
	if !ok {
		actions, ok = config["action"].([]any)
	}
	if !ok || len(actions) == 0 {
		return "  - 无动作\n"
	}

	var sb strings.Builder
	for _, a := range actions {
		action, ok := a.(map[string]any)
		if !ok {
			continue
		}

		// Skip delay actions
		if delay, hasDelay := action["delay"].(map[string]any); hasDelay {
			if secs, ok := delay["seconds"].(int); ok {
				sb.WriteString(fmt.Sprintf("  - 等待 %d 秒\n", secs))
			}
			continue
		}

		service, _ := action["action"].(string)
		if service == "" {
			service, _ = action["service"].(string)
		}
		if service == "" {
			continue
		}

		target, _ := action["target"].(map[string]any)
		data, _ := action["data"].(map[string]any)

		var entityID string
		if target != nil {
			entityID, _ = target["entity_id"].(string)
		}

		switch service {
		case "light.turn_on":
			entityName := extractEntityNameDetail(entityID)
			sb.WriteString(fmt.Sprintf("  - 打开 %s\n", entityName))
		case "light.turn_off":
			entityName := extractEntityNameDetail(entityID)
			sb.WriteString(fmt.Sprintf("  - 关闭 %s\n", entityName))
		case "cover.open_cover":
			entityName := extractEntityNameDetail(entityID)
			sb.WriteString(fmt.Sprintf("  - 打开 %s\n", entityName))
		case "cover.close_cover":
			entityName := extractEntityNameDetail(entityID)
			sb.WriteString(fmt.Sprintf("  - 关闭 %s\n", entityName))
		case "automation.turn_on":
			entityName := extractEntityNameDetail(entityID)
			sb.WriteString(fmt.Sprintf("  - 启用自动化: %s\n", entityName))
		case "automation.turn_off":
			entityName := extractEntityNameDetail(entityID)
			sb.WriteString(fmt.Sprintf("  - 禁用自动化: %s\n", entityName))
		case "input_number.set_value":
			entityName := extractEntityNameDetail(entityID)
			if data != nil {
				if val, ok := data["value"].(float64); ok {
					sb.WriteString(fmt.Sprintf("  - 设置 %s 为 %.0f\n", entityName, val))
				} else if val, ok := data["value"].(int); ok {
					sb.WriteString(fmt.Sprintf("  - 设置 %s 为 %d\n", entityName, val))
				} else {
					sb.WriteString(fmt.Sprintf("  - 设置 %s\n", entityName))
				}
			} else {
				sb.WriteString(fmt.Sprintf("  - 设置 %s\n", entityName))
			}
		case "media_player.volume_set":
			entityName := extractEntityNameDetail(entityID)
			if data != nil {
				if vol, ok := data["volume_level"].(float64); ok {
					sb.WriteString(fmt.Sprintf("  - 设置 %s 音量为 %.0f%%\n", entityName, vol*100))
				} else {
					sb.WriteString(fmt.Sprintf("  - 设置 %s 音量\n", entityName))
				}
			}
		case "text.set_value":
			if data != nil {
				if val, ok := data["value"].(string); ok {
					// Truncate long text
					if len(val) > 50 {
						val = val[:50] + "..."
					}
					// Remove template syntax for display
					if strings.Contains(val, "{{") {
						sb.WriteString("  - 语音播报（随机内容）\n")
					} else {
						sb.WriteString(fmt.Sprintf("  - 语音播报: \"%s\"\n", val))
					}
				}
			}
		default:
			sb.WriteString(fmt.Sprintf("  - %s\n", service))
		}
	}

	if sb.Len() == 0 {
		return "  - 无动作\n"
	}
	return sb.String()
}

// extractEntityNameDetail extracts a detailed friendly name from entity_id
func extractEntityNameDetail(entityID string) string {
	if entityID == "" {
		return "未知"
	}

	parts := strings.SplitN(entityID, ".", 2)
	if len(parts) != 2 {
		return entityID
	}

	domain := parts[0]
	name := parts[1]

	// Common entity name mappings
	nameMap := map[string]string{
		// 模式开关
		"hui_ke_mo_shi":                             "会客模式",
		"guan_ying_mo_shi":                          "观影模式",
		"quan_wu_yin_liang":                         "全屋音量",
		"global_brightness":                         "全局亮度",
		"global_color_temp":                         "全局色温",
		"zhu_wo_shui_mian_mo_shi":                   "主卧睡眠模式",
		"er_tong_fang_shui_mian_mo_shi":             "儿童房睡眠模式",
		"fu_mu_fang_shui_mian_mo_shi":               "父母房睡眠模式",
		"lao_ren_fang_shui_mian_mo_shi":             "老人房睡眠模式",
		"quan_wu_deng_guang_zi_dong_hua_zhuang_tai": "全屋灯光自动化状态",
		// 自动化名称
		"can_ting_wu_ren_guan_deng":                 "餐厅无人关灯",
		"ke_ting_wu_ren_guan_deng":                  "客厅无人关灯",
		"ke_wei_men_kou_guo_dao_wu_ren_guan_deng":   "客卫门口过道无人关灯",
		"ke_ting_yang_tai_guo_dao_wu_ren_guan_deng": "客厅阳台过道无人关灯",
		"xi_yi_fang_wu_ren_guan_deng":               "洗衣房无人关灯",
		"zhu_wo_men_kou_guo_dao_wu_ren_guan_deng":   "主卧门口过道无人关灯",
	}

	if friendly, ok := nameMap[name]; ok {
		return friendly
	}

	// For automations, extract the friendly name
	if domain == "automation" {
		name = strings.ReplaceAll(name, "_", " ")
		return name
	}

	// For binary sensors
	if domain == "binary_sensor" {
		return "人体传感器"
	}

	// For covers - simplify the name
	if domain == "cover" {
		return "窗帘"
	}

	// For lights - simplify the name
	if domain == "light" {
		return "灯"
	}

	// For media players
	if domain == "media_player" {
		return "音箱"
	}

	// Default
	name = strings.ReplaceAll(name, "_", " ")
	return name
}

// extractTriggerInfo extracts human-readable trigger information
func extractTriggerInfo(config map[string]any) string {
	triggers, ok := config["triggers"].([]any)
	if !ok {
		triggers, ok = config["trigger"].([]any)
	}
	if !ok || len(triggers) == 0 {
		return "未知"
	}

	var parts []string
	for _, t := range triggers {
		trigger, ok := t.(map[string]any)
		if !ok {
			continue
		}

		platform, _ := trigger["platform"].(string)
		entityID, _ := trigger["entity_id"].(string)
		to, _ := trigger["to"].(string)

		switch platform {
		case "state":
			// Extract entity name for better readability
			entityName := extractEntityName(entityID)
			if to == "on" {
				parts = append(parts, fmt.Sprintf("%s 检测到", entityName))
			} else if to == "off" {
				forDuration := ""
				if forMap, ok := trigger["for"].(map[string]any); ok {
					if mins, ok := forMap["minutes"].(int); ok {
						forDuration = fmt.Sprintf(" %d分钟后", mins)
					}
				}
				parts = append(parts, fmt.Sprintf("%s 无人%s", entityName, forDuration))
			} else if to == "1.0" || to == "1" {
				parts = append(parts, fmt.Sprintf("%s 开启", entityName))
			} else if to == "0.0" || to == "0" {
				parts = append(parts, fmt.Sprintf("%s 关闭", entityName))
			} else if to != "" {
				parts = append(parts, fmt.Sprintf("%s → %s", entityName, to))
			} else {
				parts = append(parts, fmt.Sprintf("%s 状态变化", entityName))
			}
		case "time":
			at, _ := trigger["at"].(string)
			parts = append(parts, fmt.Sprintf("时间 %s", at))
		case "numeric_state":
			entityName := extractEntityName(entityID)
			below, _ := trigger["below"].(string)
			above, _ := trigger["above"].(string)
			if below != "" {
				parts = append(parts, fmt.Sprintf("%s < %s", entityName, below))
			} else if above != "" {
				parts = append(parts, fmt.Sprintf("%s > %s", entityName, above))
			}
		default:
			if platform != "" {
				parts = append(parts, platform)
			}
		}
	}

	if len(parts) == 0 {
		return "未知"
	}

	// Deduplicate and count
	counts := make(map[string]int)
	for _, p := range parts {
		counts[p]++
	}

	var result []string
	for p, count := range counts {
		if count > 1 {
			result = append(result, fmt.Sprintf("%s×%d", p, count))
		} else {
			result = append(result, p)
		}
	}

	return strings.Join(result, ", ")
}

// extractActionInfo extracts human-readable action information
func extractActionInfo(config map[string]any) string {
	actions, ok := config["actions"].([]any)
	if !ok {
		actions, ok = config["action"].([]any)
	}
	if !ok || len(actions) == 0 {
		return "未知"
	}

	// Service to Chinese name mapping
	serviceNames := map[string]string{
		"light.turn_on":           "开灯",
		"light.turn_off":          "关灯",
		"switch.turn_on":          "开启开关",
		"switch.turn_off":         "关闭开关",
		"cover.open_cover":        "打开窗帘",
		"cover.close_cover":       "关闭窗帘",
		"automation.turn_on":      "启用自动化",
		"automation.turn_off":     "禁用自动化",
		"input_number.set_value":  "设置数值",
		"input_boolean.turn_on":   "开启",
		"input_boolean.turn_off":  "关闭",
		"media_player.volume_set": "设置音量",
		"media_player.media_stop": "停止播放",
		"media_player.media_play": "播放",
		"text.set_value":          "语音播报",
		"fan.turn_on":             "开启风扇",
		"fan.turn_off":            "关闭风扇",
		"climate.turn_on":         "开启空调",
		"climate.turn_off":        "关闭空调",
		"scene.turn_on":           "激活场景",
	}

	// Count actions by type
	actionCounts := make(map[string]int)
	for _, a := range actions {
		action, ok := a.(map[string]any)
		if !ok {
			continue
		}

		// Skip delay actions
		if _, hasDelay := action["delay"]; hasDelay {
			continue
		}

		service, _ := action["action"].(string)
		if service == "" {
			service, _ = action["service"].(string)
		}
		if service == "" {
			continue
		}

		// Get friendly name
		friendlyName := service
		if name, ok := serviceNames[service]; ok {
			friendlyName = name
		}

		// Skip template expressions
		if strings.Contains(friendlyName, "{{") {
			continue
		}

		actionCounts[friendlyName]++
	}

	// Build result
	var parts []string
	for name, count := range actionCounts {
		if count > 1 {
			parts = append(parts, fmt.Sprintf("%s×%d", name, count))
		} else {
			parts = append(parts, name)
		}
	}

	if len(parts) == 0 {
		return "未知"
	}
	return strings.Join(parts, ", ")
}

// extractEntityName extracts a friendly name from entity_id
func extractEntityName(entityID string) string {
	parts := strings.SplitN(entityID, ".", 2)
	if len(parts) != 2 {
		return entityID
	}

	domain := parts[0]
	name := parts[1]

	// Common entity name mappings
	nameMap := map[string]string{
		"hui_ke_mo_shi":     "会客模式",
		"guan_ying_mo_shi":  "观影模式",
		"quan_wu_yin_liang": "全屋音量",
		"global_brightness": "全局亮度",
		"global_color_temp": "全局色温",
		"shui_mian_mo_shi":  "睡眠模式",
		"xi_zao_mo_shi":     "洗澡模式",
	}

	if friendly, ok := nameMap[name]; ok {
		return friendly
	}

	// For binary sensors (motion/occupancy), just return "人体传感器"
	if domain == "binary_sensor" {
		return "人体传感器"
	}

	// For input_number/input_boolean, extract the name part
	if domain == "input_number" || domain == "input_boolean" {
		name = strings.ReplaceAll(name, "_", " ")
		return name
	}

	return "传感器"
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
