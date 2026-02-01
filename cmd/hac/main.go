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
	requiredGroups := []string{"人来灯亮", "人走灯灭", "热水器", "马桶换气", "睡眠模式", "光暗灯亮"}
	groupIcons := map[string]string{
		"人来灯亮": "mdi:lightbulb-on",
		"人走灯灭": "mdi:lightbulb-off",
		"热水器":  "mdi:water-boiler",
		"马桶换气": "mdi:toilet",
		"睡眠模式": "mdi:sleep",
		"光暗灯亮": "mdi:weather-sunset-down",
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
		"人来灯亮":    {"_有人_开灯", "_有人移动_开灯"},
		"人走灯灭":    {"_无人_关灯", "_无人5分钟_关灯"},
		"热水器":     {"热水器"},
		"马桶换气":    {"_坐马桶_开换气", "_无人_关换气"},
		"睡眠模式":    {"_睡眠模式_打开", "_睡眠模式_关闭", "_关闭睡眠模式", "_睡眠模式打开", "_关闭窗帘_睡眠模式"},
		"光暗灯亮":    {"_光暗_开灯"},
		"衣柜灯":     {"_衣柜开门_开灯", "_衣柜关门_关灯", "_衣柜超时未关_提醒"},
		"洗澡模式":    {"_洗澡模式_", "_浴霸", "_进入洗澡模式", "_退出洗澡模式"},
		"全屋模式":    {"全屋_观影模式_", "全屋_会客模式_", "全屋_开灯模式", "全屋_关灯模式", "全屋_音量调节_", "全屋_乔迁模式_", "全屋_夜间模式_", "全屋_白天模式_"},
		"iPad自动化": {"_iPad"},
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
