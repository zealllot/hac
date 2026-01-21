package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/zealllot/hac/internal/ha"
	"github.com/zealllot/hac/internal/mcp"
	"gopkg.in/yaml.v3"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "mcp":
		runMCP()
	case "version":
		fmt.Println("hac version 0.1.0")
	case "devices":
		runCLI(cmdDevices)
	case "state":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: hac state <entity_id>")
			os.Exit(1)
		}
		runCLI(func(c *ha.Client) error { return cmdState(c, os.Args[2]) })
	case "call":
		if len(os.Args) < 5 {
			fmt.Fprintln(os.Stderr, "Usage: hac call <domain> <service> <entity_id> [data_json]")
			os.Exit(1)
		}
		var data string
		if len(os.Args) > 5 {
			data = os.Args[5]
		}
		runCLI(func(c *ha.Client) error { return cmdCall(c, os.Args[2], os.Args[3], os.Args[4], data) })
	case "automations":
		runCLI(cmdAutomations)
	case "init":
		cmdInit()
	case "export":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: hac export <output_dir>")
			os.Exit(1)
		}
		runCLI(func(c *ha.Client) error { return cmdExport(c, os.Args[2]) })
	case "deploy":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: hac deploy <file_or_dir>")
			os.Exit(1)
		}
		runCLI(func(c *ha.Client) error { return cmdDeploy(c, os.Args[2]) })
	case "sync":
		cmdSync()
	default:
		printUsage()
		os.Exit(1)
	}
}

func getClient() *ha.Client {
	haURL := os.Getenv("HA_URL")
	haToken := os.Getenv("HA_TOKEN")

	if haURL == "" || haToken == "" {
		fmt.Fprintln(os.Stderr, "Error: HA_URL and HA_TOKEN environment variables are required")
		os.Exit(1)
	}

	return ha.NewClient(haURL, haToken)
}

func runCLI(fn func(*ha.Client) error) {
	client := getClient()
	if err := fn(client); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runMCP() {
	client := getClient()
	server := mcp.NewServer(client)

	if err := server.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func cmdDevices(client *ha.Client) error {
	devices, err := client.GetDevices()
	if err != nil {
		return err
	}

	for entityID, dev := range devices {
		state := dev.State
		if state == "" {
			state = "-"
		}
		name := dev.Name
		if name == "" {
			name = entityID
		}
		fmt.Printf("%-40s %-20s %s\n", entityID, state, name)
	}
	return nil
}

func cmdState(client *ha.Client, entityID string) error {
	state, err := client.GetState(entityID)
	if err != nil {
		return err
	}

	data, _ := json.MarshalIndent(state, "", "  ")
	fmt.Println(string(data))
	return nil
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

func cmdAutomations(client *ha.Client) error {
	automations, err := client.GetAutomations()
	if err != nil {
		return err
	}

	for _, a := range automations {
		name := ""
		if n, ok := a.Attributes["friendly_name"].(string); ok {
			name = n
		}
		id := strings.TrimPrefix(a.EntityID, "automation.")
		fmt.Printf("%-30s %-10s %s\n", id, a.State, name)
	}
	return nil
}

type HacConfig struct {
	HAURL      string `yaml:"ha_url"`
	HAToken    string `yaml:"ha_token"`
	ConfigRepo string `yaml:"config_repo"`
}

func getHacConfigPath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".hac.yaml")
}

func loadHacConfig() (*HacConfig, error) {
	data, err := os.ReadFile(getHacConfigPath())
	if err != nil {
		return nil, err
	}
	var cfg HacConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func saveHacConfig(cfg *HacConfig) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(getHacConfigPath(), data, 0600)
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
	config, err := client.GetConfig()
	if err != nil {
		fmt.Println("✗")
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✓ Connected to %s (HA %s)\n", config.LocationName, config.Version)

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

	// Save hac config
	hacCfg := &HacConfig{
		HAURL:      haURL,
		HAToken:    haToken,
		ConfigRepo: configRepo,
	}
	if err := saveHacConfig(hacCfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error saving hac config: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✓ Saved config to %s\n", getHacConfigPath())

	// Get hac binary path
	exePath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting executable path: %v\n", err)
		os.Exit(1)
	}
	exePath, _ = filepath.Abs(exePath)

	// Prepare MCP config
	homeDir, _ := os.UserHomeDir()
	configDir := filepath.Join(homeDir, ".codeium", "windsurf")
	configPath := filepath.Join(configDir, "mcp_config.json")

	// Create directory if not exists
	if err := os.MkdirAll(configDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating config directory: %v\n", err)
		os.Exit(1)
	}

	// Read existing config or create new
	var mcpConfig map[string]any
	if data, err := os.ReadFile(configPath); err == nil {
		json.Unmarshal(data, &mcpConfig)
	}
	if mcpConfig == nil {
		mcpConfig = make(map[string]any)
	}

	// Get or create mcpServers
	servers, ok := mcpConfig["mcpServers"].(map[string]any)
	if !ok {
		servers = make(map[string]any)
	}

	// Add hac server with config repo env
	envVars := map[string]string{
		"HA_URL":   haURL,
		"HA_TOKEN": haToken,
	}
	if configRepo != "" {
		envVars["HAC_CONFIG_REPO"] = configRepo
	}

	servers["hac"] = map[string]any{
		"command": exePath,
		"args":    []string{"mcp"},
		"env":     envVars,
	}
	mcpConfig["mcpServers"] = servers

	// Write config
	data, _ := json.MarshalIndent(mcpConfig, "", "  ")
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ MCP config written to %s\n", configPath)
	fmt.Println("\n⚠️  Please restart Windsurf to apply changes.")
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

func cmdDeploy(client *ha.Client, path string) error {
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

	// Connect WebSocket for category operations
	ws, err := client.NewWSClient()
	if err != nil {
		return fmt.Errorf("connect WebSocket: %w", err)
	}
	defer ws.Close()

	// Get or create categories
	categories, err := ws.ListCategories("automation")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to list categories: %v\n", err)
	}

	categoryMap := make(map[string]string) // group name -> category ID
	for _, cat := range categories {
		categoryMap[cat.Name] = cat.CategoryID
	}

	// Ensure required categories exist
	requiredGroups := []string{"人来灯亮", "人走灯灭", "热水器"}
	groupIcons := map[string]string{
		"人来灯亮": "mdi:lightbulb-on",
		"人走灯灭": "mdi:lightbulb-off",
		"热水器":  "mdi:water-boiler",
	}
	for _, group := range requiredGroups {
		if _, exists := categoryMap[group]; !exists {
			icon := groupIcons[group]
			cat, err := ws.CreateCategory("automation", group, icon)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to create category %s: %v\n", group, err)
			} else {
				categoryMap[group] = cat.CategoryID
				fmt.Printf("✓ Created category: %s\n", group)
			}
		}
	}

	deployed := 0
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to read %s: %v\n", file, err)
			continue
		}

		var automation map[string]any
		if err := yaml.Unmarshal(data, &automation); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to parse %s: %v\n", file, err)
			continue
		}

		if err := client.CreateAutomation(automation); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to deploy %s: %v\n", file, err)
			continue
		}

		alias := filepath.Base(file)
		if a, ok := automation["alias"].(string); ok {
			alias = a
		}

		// Assign category based on alias
		group := getAutomationGroup(alias)
		if categoryID, exists := categoryMap[group]; exists {
			// Get entity_id from automation id
			id, _ := automation["id"].(string)
			if id != "" {
				entityID := "automation." + strings.ReplaceAll(strings.ToLower(alias), " ", "_")
				// Try to find the actual entity_id
				automations, _ := client.GetAutomations()
				for _, a := range automations {
					if aid, ok := a.Attributes["id"].(string); ok && aid == id {
						entityID = a.EntityID
						break
					}
				}
				if err := ws.AssignCategory("automation", entityID, categoryID); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to assign category for %s: %v\n", alias, err)
				}
			}
		}

		fmt.Printf("✓ Deployed %s\n", alias)
		deployed++
	}

	fmt.Printf("\nDeployed %d automations\n", deployed)
	return nil
}

func cmdSync() {
	cfg, err := loadHacConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: run 'hac init' first to configure")
		os.Exit(1)
	}

	if cfg.ConfigRepo == "" {
		fmt.Fprintln(os.Stderr, "Error: config_repo not set. Run 'hac init' to configure.")
		os.Exit(1)
	}

	client := ha.NewClient(cfg.HAURL, cfg.HAToken)

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

// getAutomationGroup determines the group/category for an automation based on its alias
func getAutomationGroup(alias string) string {
	patterns := map[string][]string{
		"人来灯亮": {"_有人_开灯", "_有人移动_开灯"},
		"人走灯灭": {"_无人_关灯", "_无人5分钟_关灯"},
		"热水器":  {"热水器"},
		"马桶换气": {"_坐马桶_开换气", "_无人_关换气"},
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

	var automations []map[string]string
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

		// Extract trigger and action info
		triggerInfo := extractTriggerInfo(config)
		actionInfo := extractActionInfo(config)

		automations = append(automations, map[string]string{
			"name":    alias,
			"mode":    mode,
			"trigger": triggerInfo,
			"action":  actionInfo,
			"file":    e.Name(),
		})
	}

	if len(automations) == 0 {
		return nil
	}

	// Generate README content
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# %s\n\n", groupName))
	sb.WriteString(fmt.Sprintf("本目录包含 %d 个自动化配置。\n\n", len(automations)))

	sb.WriteString("## 自动化列表\n\n")
	sb.WriteString("| 名称 | 触发条件 | 动作 | 模式 |\n")
	sb.WriteString("|------|----------|------|------|\n")

	for _, a := range automations {
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n", a["name"], a["trigger"], a["action"], a["mode"]))
	}

	readmePath := filepath.Join(groupDir, "README.md")
	return os.WriteFile(readmePath, []byte(sb.String()), 0644)
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
		switch platform {
		case "state":
			to, _ := trigger["to"].(string)
			if to == "on" {
				parts = append(parts, "有人检测到")
			} else if to == "off" {
				forDuration := ""
				if forMap, ok := trigger["for"].(map[string]any); ok {
					if mins, ok := forMap["minutes"].(int); ok {
						forDuration = fmt.Sprintf(" %d分钟后", mins)
					}
				}
				parts = append(parts, fmt.Sprintf("无人%s", forDuration))
			}
		case "time":
			at, _ := trigger["at"].(string)
			parts = append(parts, fmt.Sprintf("时间 %s", at))
		}
	}

	if len(parts) == 0 {
		return "未知"
	}
	return strings.Join(parts, ", ")
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

	var parts []string
	for _, a := range actions {
		action, ok := a.(map[string]any)
		if !ok {
			continue
		}

		service, _ := action["action"].(string)
		if service == "" {
			service, _ = action["service"].(string)
		}
		target, _ := action["target"].(map[string]any)

		entityCount := 0
		if target != nil {
			if _, ok := target["entity_id"].(string); ok {
				entityCount = 1
			} else if entityIDs, ok := target["entity_id"].([]any); ok {
				entityCount = len(entityIDs)
			}
		}

		switch service {
		case "light.turn_on":
			if entityCount > 1 {
				parts = append(parts, fmt.Sprintf("开灯 (%d个)", entityCount))
			} else {
				parts = append(parts, "开灯")
			}
		case "light.turn_off":
			if entityCount > 1 {
				parts = append(parts, fmt.Sprintf("关灯 (%d个)", entityCount))
			} else {
				parts = append(parts, "关灯")
			}
		default:
			parts = append(parts, service)
		}
	}

	if len(parts) == 0 {
		return "未知"
	}
	return strings.Join(parts, ", ")
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
