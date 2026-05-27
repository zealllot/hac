# hac — Home Assistant CLI

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go" alt="Go Version">
  <img src="https://img.shields.io/badge/Home%20Assistant-2024.1+-41BDF5?style=flat&logo=homeassistant" alt="HA Version">
  <img src="https://img.shields.io/badge/License-MIT-yellow?style=flat" alt="License">
</p>

A single-binary CLI for inspecting and modifying Home Assistant from the shell. Designed to be invoked by [Claude Code](https://claude.com/claude-code) (and a human at a terminal), backed by a git repo of automation YAML.

## ✨ Features

- **Single binary, fast startup** — drop into `$PATH`, no runtime dependencies
- **Read** entity state, search by area / friendly name, query logbook history
- **Write** automations from local YAML to HA with one command
- **Sync** HA categories ↔ local directory layout (HA is the source of truth)
- **JSON-first output** for easy machine parsing; `--format=table` for human reading

## 🚀 Quick Start

### Prerequisites

- Go 1.21+
- Home Assistant instance with a long-lived access token
- Optional: git repo where you keep automation YAML

### Install

```bash
git clone https://github.com/zealllot/hac.git
cd hac
go build -o hac ./cmd/hac
# move the binary somewhere in $PATH
```

### First-time setup

```bash
hac init
```

The wizard asks for:

1. **Home Assistant URL** — e.g. `http://192.168.1.100:8123`
2. **Long-lived access token** — created in HA: User profile → Security → Long-lived access tokens
3. **Config repo path** — where automation YAML lives (optional)

It tests the connection, then saves credentials to `~/.hac.yaml` (mode 0600). That's the only file `hac init` writes.

## 🛠️ Commands

```bash
hac init                                       # initial setup
hac devices                                    # list all devices
hac state <entity_id>                          # query a single entity's state
hac call <domain> <service> <entity_id> [data] # call a service
hac automations                                # list automations
hac export <output_dir>                        # export all automations to YAML
hac deploy <file_or_dir>                       # deploy YAML automation(s) to HA
hac sync                                       # pull HA state into the config repo + commit
```

More commands shipping in upcoming releases (`hac history`, `hac search`, `hac area`, `hac.deploy --commit` etc.) — see open issues.

## 🔧 Configuration

`~/.hac.yaml` (created by `hac init`):

```yaml
ha_url: http://192.168.1.100:8123
ha_token: <your-long-lived-token>
config_repo: /path/to/ha-config
```

Loading precedence: env vars (`HA_URL` / `HA_TOKEN` / `HAC_CONFIG_REPO`) > `~/.hac.yaml` > error. Per-source, no field-level mixing.

## 📁 Config repo layout

```
ha-config/
├── automations/                # deployed automations, grouped by HA category
│   ├── 人来灯亮/                #   subdirectory == HA category name
│   │   ├── 客厅_有人_开灯.yaml
│   │   └── 卧室_有人_开灯.yaml
│   ├── 人走灯灭/
│   ├── 光暗灯亮/
│   ├── 光亮灯灭/
│   └── 全屋模式/
├── scripts/                    # script configs
├── scenes/                     # scene configs
└── input_number.yaml           # global helpers
```

The subdirectory under `automations/` is treated as the HA category name — see [ADR-0003](docs/adr/0003-directory-as-category.md).

## 📋 Naming convention

```
[Location]_[Trigger / State]_[Action]
```

Examples: `客厅_有人_开灯`, `卧室_无人5分钟_关灯`, `全屋_离家_关闭所有灯`. See [AUTOMATION_GUIDELINES.md](./AUTOMATION_GUIDELINES.md) for the full set.

## 📚 Design decisions

Architectural decisions live as short ADRs in [docs/adr/](docs/adr/):

- [0001 — CLI-only](docs/adr/0001-cli-only.md): why MCP server was removed
- [0002 — `hac deploy` doesn't auto-commit](docs/adr/0002-deploy-no-commit.md): orchestrator owns the commit
- [0003 — Directory-as-category](docs/adr/0003-directory-as-category.md): subdirectory under `automations/` is the canonical HA category name

## 🔒 Security

- `~/.hac.yaml` is mode 0600 and contains your HA token — don't commit it
- The config repo (automation YAML) does not contain credentials; safe to push to public git
- All HA mutations go through the long-lived token; rotate if exposed

## 📄 License

MIT
