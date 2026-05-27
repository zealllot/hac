package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zealllot/hac/internal/config"
)

func TestSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := &config.Config{
		HAURL:      "http://ha.example:8123",
		HAToken:    "tok-abc",
		ConfigRepo: "/some/repo",
	}

	if err := config.SaveTo(want, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// File created at the right path with 0600.
	path := filepath.Join(dir, ".hac.yaml")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("expected file at %s: %v", path, err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("perm = %o, want 0600", info.Mode().Perm())
	}

	// Round-trip: Load via LoadFrom using the same dir as HomeDir.
	got, err := config.LoadFrom(config.Sources{
		GetEnv:   func(string) string { return "" },
		HomeDir:  dir,
		ReadFile: os.ReadFile,
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if *got != *want {
		t.Errorf("round-trip: got %+v, want %+v", got, want)
	}
}

const (
	homeDir      = "/home/x"
	yamlPath     = homeDir + "/.hac.yaml"
	windsurfPath = homeDir + "/.codeium/windsurf/mcp_config.json"
)

func TestLoadFrom(t *testing.T) {
	yamlGood := []byte("ha_url: http://yaml.example\nha_token: tok-yaml\nconfig_repo: /yaml-repo\n")
	yamlBad := []byte("ha_url: [unclosed")
	windsurfGood := []byte(`{
  "mcpServers": {
    "hac": {
      "env": {
        "HA_URL": "http://ws.example",
        "HA_TOKEN": "tok-ws",
        "HAC_CONFIG_REPO": "/ws-repo"
      }
    }
  }
}`)

	cases := []struct {
		name     string
		env      map[string]string
		files    map[string][]byte
		wantCfg  *config.Config
		wantErr  string // empty = expect no error; "ErrNoCredentials" = expect sentinel; else substring of err.Error()
	}{
		{
			name: "env-only path",
			env: map[string]string{
				"HA_URL":          "http://env.example",
				"HA_TOKEN":        "tok-env",
				"HAC_CONFIG_REPO": "/env-repo",
			},
			files:   nil,
			wantCfg: &config.Config{HAURL: "http://env.example", HAToken: "tok-env", ConfigRepo: "/env-repo"},
		},
		{
			name:    "yaml-only path",
			env:     nil,
			files:   map[string][]byte{yamlPath: yamlGood},
			wantCfg: &config.Config{HAURL: "http://yaml.example", HAToken: "tok-yaml", ConfigRepo: "/yaml-repo"},
		},
		{
			name:    "windsurf fallback path",
			env:     nil,
			files:   map[string][]byte{windsurfPath: windsurfGood},
			wantCfg: &config.Config{HAURL: "http://ws.example", HAToken: "tok-ws", ConfigRepo: "/ws-repo"},
		},
		{
			name: "env wins over yaml",
			env: map[string]string{
				"HA_URL":          "http://env.example",
				"HA_TOKEN":        "tok-env",
				"HAC_CONFIG_REPO": "/env-repo",
			},
			files:   map[string][]byte{yamlPath: yamlGood},
			wantCfg: &config.Config{HAURL: "http://env.example", HAToken: "tok-env", ConfigRepo: "/env-repo"},
		},
		{
			name:    "all missing returns guidance error",
			env:     nil,
			files:   nil,
			wantErr: "ErrNoCredentials",
		},
		{
			name:    "corrupt yaml propagates parse error",
			env:     nil,
			files:   map[string][]byte{yamlPath: yamlBad},
			wantErr: "parse ~/.hac.yaml",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srcs := config.Sources{
				GetEnv:  func(k string) string { return tc.env[k] },
				HomeDir: homeDir,
				ReadFile: func(p string) ([]byte, error) {
					if b, ok := tc.files[p]; ok {
						return b, nil
					}
					return nil, os.ErrNotExist
				},
			}

			got, err := config.LoadFrom(srcs)

			switch tc.wantErr {
			case "":
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if *got != *tc.wantCfg {
					t.Errorf("got %+v, want %+v", got, tc.wantCfg)
				}
			case "ErrNoCredentials":
				if !errors.Is(err, config.ErrNoCredentials) {
					t.Fatalf("want ErrNoCredentials, got %v", err)
				}
				for _, frag := range []string{"hac init", "HA_URL", "HA_TOKEN"} {
					if !strings.Contains(err.Error(), frag) {
						t.Errorf("error %q missing guidance fragment %q", err.Error(), frag)
					}
				}
			default:
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if errors.Is(err, config.ErrNoCredentials) {
					t.Errorf("want parse error, got ErrNoCredentials (silent fallthrough)")
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("err %q missing %q", err.Error(), tc.wantErr)
				}
			}
		})
	}
}
