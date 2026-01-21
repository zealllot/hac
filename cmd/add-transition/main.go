package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func main() {
	dirs := []string{
		"/Users/zealllot/go/src/github.com/zealllot/ha-config/automations/人来灯亮",
		"/Users/zealllot/go/src/github.com/zealllot/ha-config/automations/人走灯灭",
	}

	transitionTemplate := "{{ states('input_number.quan_ju_huan_liang_huan_mie_shi_jian') | float }}"
	count := 0

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			fmt.Printf("Error reading %s: %v\n", dir, err)
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
				continue
			}

			filePath := filepath.Join(dir, entry.Name())
			data, err := os.ReadFile(filePath)
			if err != nil {
				fmt.Printf("Error reading %s: %v\n", filePath, err)
				continue
			}

			var config map[string]any
			if err := yaml.Unmarshal(data, &config); err != nil {
				fmt.Printf("Error parsing %s: %v\n", filePath, err)
				continue
			}

			actions, ok := config["actions"].([]any)
			if !ok {
				continue
			}

			modified := false
			for _, a := range actions {
				action, ok := a.(map[string]any)
				if !ok {
					continue
				}

				actionType, _ := action["action"].(string)
				if actionType != "light.turn_on" && actionType != "light.turn_off" {
					continue
				}

				data, ok := action["data"].(map[string]any)
				if !ok {
					data = make(map[string]any)
					action["data"] = data
				}

				if _, exists := data["transition"]; !exists {
					data["transition"] = transitionTemplate
					modified = true
				}
			}

			if modified {
				output, err := yaml.Marshal(config)
				if err != nil {
					fmt.Printf("Error marshaling %s: %v\n", filePath, err)
					continue
				}

				if err := os.WriteFile(filePath, output, 0644); err != nil {
					fmt.Printf("Error writing %s: %v\n", filePath, err)
					continue
				}

				fmt.Printf("Updated: %s\n", entry.Name())
				count++
			}
		}
	}

	fmt.Printf("\nTotal updated: %d files\n", count)
}
