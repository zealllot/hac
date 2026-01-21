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

		name := filepath.Base(file)
		if alias, ok := automation["alias"].(string); ok {
			name = alias
		}
		fmt.Printf("✓ Deployed %s\n", name)
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

	synced := 0
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

		filename := filepath.Join(automationsDir, id+".yaml")
		if err := os.WriteFile(filename, data, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to write %s: %v\n", filename, err)
			continue
		}

		synced++
	}

	fmt.Printf("✓ Synced %d automations to %s\n", synced, automationsDir)

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
