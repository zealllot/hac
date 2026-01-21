package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zealllot/hac/internal/ha"
	"gopkg.in/yaml.v3"
)

func main() {
	haURL := os.Getenv("HA_URL")
	haToken := os.Getenv("HA_TOKEN")

	if haURL == "" || haToken == "" {
		fmt.Fprintln(os.Stderr, "Error: HA_URL and HA_TOKEN environment variables are required")
		os.Exit(1)
	}

	client := ha.NewClient(haURL, haToken)

	dirs := []string{
		"/Users/zealllot/go/src/github.com/zealllot/ha-config/automations/人来灯亮",
		"/Users/zealllot/go/src/github.com/zealllot/ha-config/automations/人走灯灭",
	}

	successCount := 0
	errorCount := 0

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			fmt.Printf("Error reading %s: %v\n", dir, err)
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") || entry.Name() == "README.md" {
				continue
			}

			filePath := filepath.Join(dir, entry.Name())
			data, err := os.ReadFile(filePath)
			if err != nil {
				fmt.Printf("Error reading %s: %v\n", filePath, err)
				errorCount++
				continue
			}

			var config map[string]any
			if err := yaml.Unmarshal(data, &config); err != nil {
				fmt.Printf("Error parsing %s: %v\n", filePath, err)
				errorCount++
				continue
			}

			id, _ := config["id"].(string)
			alias, _ := config["alias"].(string)

			if id == "" {
				fmt.Printf("Skipping %s: no id\n", entry.Name())
				continue
			}

			if err := client.UpdateAutomation(id, config); err != nil {
				fmt.Printf("✗ Error updating %s: %v\n", alias, err)
				errorCount++
				continue
			}

			fmt.Printf("✓ Updated: %s\n", alias)
			successCount++
		}
	}

	fmt.Printf("\nTotal: %d success, %d errors\n", successCount, errorCount)
}
