# hac - Home Assistant CLI & MCP Server

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.21+-00ADD8?style=flat&logo=go" alt="Go Version">
  <img src="https://img.shields.io/badge/Home%20Assistant-2024.1+-41BDF5?style=flat&logo=homeassistant" alt="HA Version">
  <img src="https://img.shields.io/badge/MCP-Compatible-green?style=flat" alt="MCP Compatible">
  <img src="https://img.shields.io/badge/License-MIT-yellow?style=flat" alt="License">
</p>

让 AI 帮你控制智能家居、创建自动化规则。通过自然语言对话，无需编写 YAML 配置。

## ✨ 特性

- 🤖 **AI 驱动** - 通过自然语言对话控制智能家居
- 🔌 **MCP 协议** - 与 Windsurf/Cursor 等 AI IDE 无缝集成
- 📝 **自动化管理** - 创建、修改、删除自动化规则
- 🏠 **设备控制** - 直接控制灯光、开关、空调等设备
- 📦 **版本控制** - 自动化配置自动保存到 Git 仓库
- 🔒 **安全确认** - 所有自动化部署前需用户确认

## 🚀 快速开始

### 前置要求

- Go 1.21+
- Home Assistant 实例
- [Windsurf](https://codeium.com/windsurf) 或其他支持 MCP 的 AI IDE

### 安装

```bash
# 克隆仓库
git clone https://github.com/zealllot/hac.git
cd hac

# 编译
go build -o hac ./cmd/hac
```

### 初始化

```bash
./hac init
```

按提示输入：
1. **Home Assistant 地址** - 如 `http://192.168.1.100:8123`
2. **长期访问令牌** - 在 HA 用户设置 → 安全 → 长期访问令牌 中创建
3. **配置仓库路径** - 如 `~/ha-config`，用于版本控制自动化配置

初始化完成后，**重启 Windsurf** 即可开始使用。

## 💬 使用示例

在 Windsurf 中直接用自然语言对话：

### 控制设备

```
"帮我把客厅灯打开"
"把卧室灯调到 50% 亮度，色温 4000K"
"关掉所有灯"
"打开空调，设置到 26 度"
```

### 创建自动化

```
"帮我创建一个自动化：晚上7点到11点，客厅有人移动时自动开灯"
"创建一个自动化：每天早上7点打开窗帘"
"设置一个离家模式：我离开家时关闭所有灯和空调"
"当光照低于 100 lux 且有人时，自动开灯"
```

### 管理自动化

```
"列出所有自动化"
"删除客厅感应灯自动化"
"修改主卧开灯阈值为 150"
"有哪些待确认的自动化？"
```

### 查询状态

```
"客厅现在的光照是多少？"
"哪些灯是开着的？"
"搜索所有人体传感器"
```

## 🔄 自动化创建流程

```
┌─────────────────────────────────────────────────────────┐
│  你: "创建一个自动化：晚上客厅有人时开灯"                    │
└─────────────────────────────────────────────────────────┘
                           ↓
┌─────────────────────────────────────────────────────────┐
│  AI: 生成配置草稿，展示 YAML 配置供你确认                    │
│                                                         │
│  alias: 客厅_晚间有人_开灯                                │
│  trigger:                                               │
│    - platform: state                                    │
│      entity_id: binary_sensor.motion_living_room        │
│      to: "on"                                           │
│  ...                                                    │
└─────────────────────────────────────────────────────────┘
                           ↓
┌─────────────────────────────────────────────────────────┐
│  你: "确认" 或 "把亮度改成50%"                             │
└─────────────────────────────────────────────────────────┘
                           ↓
┌─────────────────────────────────────────────────────────┐
│  AI: ✓ 部署到 Home Assistant                            │
│      ✓ 保存到配置仓库                                    │
│      ✓ 提交 git commit                                  │
└─────────────────────────────────────────────────────────┘
```

## 🛠️ 命令行工具

除了 AI 对话，也可以直接使用命令行：

```bash
# 初始化配置
hac init

# 设备管理
hac devices                                    # 列出所有设备
hac state light.living_room                    # 查看设备状态
hac call light turn_on light.living_room       # 调用服务
hac call light turn_on light.living_room '{"brightness_pct":50}'

# 自动化管理
hac automations                                # 列出所有自动化
hac export ./automations                       # 导出自动化到本地
hac deploy ./automations/living_room.yaml      # 部署自动化
hac sync                                       # 同步 HA 配置到本地仓库
```

## 📁 配置文件

### hac 配置

初始化后，配置保存在 `~/.hac.yaml`：

```yaml
ha_url: http://192.168.1.100:8123
ha_token: your_long_lived_access_token
config_repo: /path/to/ha-config
```

### MCP 配置

自动写入 `~/.codeium/windsurf/mcp_config.json`，无需手动配置。

### 配置仓库结构

```
ha-config/
├── automations/           # 已部署的自动化配置
│   ├── 人来灯亮/          # 按功能分组
│   │   ├── 客厅_有人_开灯.yaml
│   │   └── 卧室_有人_开灯.yaml
│   ├── 人走灯灭/
│   ├── 光暗灯亮/
│   ├── 光亮灯灭/
│   └── 全屋模式/
├── pending/               # 待确认的自动化草稿
├── scripts/               # 脚本配置
├── scenes/                # 场景配置
└── input_number.yaml      # 全局变量配置
```

## 🔧 MCP 工具列表

hac 提供以下 MCP 工具供 AI 调用：

| 工具 | 描述 |
|------|------|
| `get_devices` | 获取所有设备及其状态 |
| `search_devices` | 按关键词搜索设备 |
| `get_state` | 获取指定设备状态 |
| `call_service` | 调用 HA 服务 |
| `list_automations` | 列出所有自动化 |
| `create_automation` | 创建自动化草稿 |
| `confirm_automation` | 确认并部署自动化 |
| `update_automation` | 更新已有自动化 |
| `delete_automation` | 删除自动化 |
| `create_input_number` | 创建全局变量 |
| `create_input_button` | 创建按钮实体 |
| `rename_entity` | 重命名实体 ID |
| `reload_integration` | 重新加载集成 |
| `render_template` | 渲染 Jinja2 模板 |
| `sync_automations` | 同步配置到本地 |

## 📋 自动化命名规范

为保持配置整洁，建议遵循以下命名规范：

```
[房间]_[触发条件]_[动作]
```

示例：
- `客厅_有人_开灯`
- `卧室_无人5分钟_关灯`
- `全屋_离家_关闭所有灯`

详细规范请参考 [AUTOMATION_GUIDELINES.md](./AUTOMATION_GUIDELINES.md)

## 🔒 安全说明

- 配置文件 `~/.hac.yaml` 包含 HA 访问令牌，请勿泄露
- 所有自动化部署前需用户明确确认
- 配置仓库不包含敏感信息，可安全公开

## 📄 License

MIT
