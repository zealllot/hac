package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	HAURL      string
	HAToken    string
	ConfigRepo string
}

type Sources struct {
	GetEnv   func(string) string
	HomeDir  string
	ReadFile func(string) ([]byte, error)
}

var ErrNoCredentials = errors.New(
	"HA credentials not found: run `hac init` first, or set HA_URL and HA_TOKEN env vars",
)

type yamlConfig struct {
	HAURL      string `yaml:"ha_url"`
	HAToken    string `yaml:"ha_token"`
	ConfigRepo string `yaml:"config_repo"`
}

func Load() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("locate home dir: %w", err)
	}
	return LoadFrom(Sources{
		GetEnv:   os.Getenv,
		HomeDir:  home,
		ReadFile: os.ReadFile,
	})
}

func LoadFrom(srcs Sources) (*Config, error) {
	if u, t := srcs.GetEnv("HA_URL"), srcs.GetEnv("HA_TOKEN"); u != "" && t != "" {
		return &Config{
			HAURL:      u,
			HAToken:    t,
			ConfigRepo: srcs.GetEnv("HAC_CONFIG_REPO"),
		}, nil
	}

	if data, err := srcs.ReadFile(filepath.Join(srcs.HomeDir, ".hac.yaml")); err == nil {
		var y yamlConfig
		if perr := yaml.Unmarshal(data, &y); perr != nil {
			return nil, fmt.Errorf("parse ~/.hac.yaml: %w", perr)
		}
		if y.HAURL != "" && y.HAToken != "" {
			return &Config{
				HAURL:      y.HAURL,
				HAToken:    y.HAToken,
				ConfigRepo: y.ConfigRepo,
			}, nil
		}
	}

	if data, err := srcs.ReadFile(filepath.Join(srcs.HomeDir, ".codeium", "windsurf", "mcp_config.json")); err == nil {
		var ws struct {
			MCPServers map[string]struct {
				Env map[string]string `json:"env"`
			} `json:"mcpServers"`
		}
		if err := json.Unmarshal(data, &ws); err == nil {
			if hac, ok := ws.MCPServers["hac"]; ok {
				if u, t := hac.Env["HA_URL"], hac.Env["HA_TOKEN"]; u != "" && t != "" {
					return &Config{
						HAURL:      u,
						HAToken:    t,
						ConfigRepo: hac.Env["HAC_CONFIG_REPO"],
					}, nil
				}
			}
		}
	}

	return nil, ErrNoCredentials
}
