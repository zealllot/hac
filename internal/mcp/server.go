package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/zealllot/hac/internal/ha"
	"github.com/zealllot/hac/internal/ir"
	"gopkg.in/yaml.v3"
)

type Server struct {
	haClient *ha.Client
}

func NewServer(haClient *ha.Client) *Server {
	return &Server{haClient: haClient}
}

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   *Error `json:"error,omitempty"`
}

type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type InitializeResult struct {
	ProtocolVersion string       `json:"protocolVersion"`
	Capabilities    Capabilities `json:"capabilities"`
	ServerInfo      ServerInfo   `json:"serverInfo"`
}

type Capabilities struct {
	Tools *ToolsCapability `json:"tools,omitempty"`
}

type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema InputSchema `json:"inputSchema"`
}

type InputSchema struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties,omitempty"`
	Required   []string            `json:"required,omitempty"`
}

type Property struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

type ToolsListResult struct {
	Tools []Tool `json:"tools"`
}

type CallToolParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

type CallToolResult struct {
	Content []Content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

type Content struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (s *Server) Run() error {
	scanner := bufio.NewScanner(os.Stdin)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			s.sendError(nil, -32700, "Parse error")
			continue
		}

		s.handleRequest(&req)
	}

	return scanner.Err()
}

func (s *Server) handleRequest(req *Request) {
	switch req.Method {
	case "initialize":
		s.sendResult(req.ID, InitializeResult{
			ProtocolVersion: "2024-11-05",
			Capabilities: Capabilities{
				Tools: &ToolsCapability{},
			},
			ServerInfo: ServerInfo{
				Name:    "hac",
				Version: "0.1.0",
			},
		})

	case "notifications/initialized":
		// No response needed for notifications

	case "tools/list":
		s.sendResult(req.ID, ToolsListResult{
			Tools: s.getTools(),
		})

	case "tools/call":
		var params CallToolParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			s.sendError(req.ID, -32602, "Invalid params")
			return
		}
		result := s.callTool(params.Name, params.Arguments)
		s.sendResult(req.ID, result)

	default:
		s.sendError(req.ID, -32601, "Method not found: "+req.Method)
	}
}

func (s *Server) getTools() []Tool {
	// 全局说明会在第一个工具的描述中体现
	return []Tool{
		{
			Name: "get_devices",
			Description: `获取 Home Assistant 中所有设备及其能力。返回每个设备的 entity_id、名称、当前状态、支持的服务和属性。注意：设备数量较多时可能被截断，建议使用 search_devices 按关键词搜索。

【hac 全局规则】
1. 所有修改 HA 配置的操作都会自动同步到本地 config 仓库并提交 git
2. 自动同步的操作包括：
   - confirm_automation: 部署后移动到 automations/ + commit
   - update_automation: 更新后从 HA pull 配置 + commit
   - delete_automation: 删除后删除本地文件 + commit
   - create_input_number: 创建后同步到 input_number.yaml + commit
   - rename_entity: 重命名后更新所有引用该实体的本地文件 + commit
3. 灯光自动化中的色温、亮度等参数必须使用全局变量（input_number），不要硬编码
4. 创建自动化时会自动为灯组设置中文 friendly_name
5. ⚠️ 批量更新自动化后，必须执行 hac sync 命令来同步配置到本地并按组分类生成文档
6. ⚠️ 修改自动化配置（如阈值、条件、触发器等）前必须先与用户确认，得到用户同意后才能提交修改`,
			InputSchema: InputSchema{
				Type: "object",
			},
		},
		{
			Name: "search_devices",
			Description: `按关键词搜索设备。支持搜索 entity_id、设备名称和区域。

示例：
- 搜索 "衣帽间" 会返回所有区域是衣帽间的设备
- 搜索 "light" 会返回所有灯类设备
- 搜索 "客厅 灯" 会返回客厅的灯

推荐在创建自动化前使用此工具确认设备的 entity_id。

⚠️ 重要：必须向用户展示搜索返回的【全部】设备列表，不要只展示"主要的"或"部分"结果。用户需要看到完整列表才能做出正确选择。`,
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"keyword": {
						Type:        "string",
						Description: "搜索关键词，支持中文，多个关键词用空格分隔（AND 关系）",
					},
				},
				Required: []string{"keyword"},
			},
		},
		{
			Name:        "get_state",
			Description: "获取指定设备的当前状态和属性。",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"entity_id": {
						Type:        "string",
						Description: "设备的 entity_id，例如 light.living_room",
					},
				},
				Required: []string{"entity_id"},
			},
		},
		{
			Name:        "call_service",
			Description: "调用 Home Assistant 服务，例如开灯、关灯、调节亮度等。",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"domain": {
						Type:        "string",
						Description: "服务域，例如 light, switch, climate",
					},
					"service": {
						Type:        "string",
						Description: "服务名称，例如 turn_on, turn_off, toggle",
					},
					"entity_id": {
						Type:        "string",
						Description: "目标设备的 entity_id",
					},
					"data": {
						Type:        "string",
						Description: "额外的服务数据，JSON 格式，例如 {\"brightness\": 255}",
					},
				},
				Required: []string{"domain", "service", "entity_id"},
			},
		},
		{
			Name:        "list_automations",
			Description: "列出 Home Assistant 中所有自动化及其状态。",
			InputSchema: InputSchema{
				Type: "object",
			},
		},
		{
			Name:        "trigger_automation",
			Description: "手动触发一个自动化。",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"entity_id": {
						Type:        "string",
						Description: "自动化的 entity_id，例如 automation.night_light",
					},
				},
				Required: []string{"entity_id"},
			},
		},
		{
			Name:        "get_ha_config",
			Description: "获取 Home Assistant 的基本配置信息，包括位置、时区、版本等。",
			InputSchema: InputSchema{
				Type: "object",
			},
		},
		{
			Name: "execute_ir",
			Description: `执行一个 Action IR。会先校验 entity_id 和 service 是否合法，校验通过后执行。

IR 格式示例：
{
  "action": "call_service",
  "service": "light.turn_on",
  "target": "light.living_room",
  "data": {"brightness_pct": 50}
}

支持的 action: call_service
校验失败时会返回错误信息和建议的正确值。`,
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"ir": {
						Type:        "string",
						Description: "Action IR JSON 字符串",
					},
				},
				Required: []string{"ir"},
			},
		},
		{
			Name: "validate_ir",
			Description: `校验一个 Action IR 是否合法，不执行。用于在执行前检查 IR 是否正确。

返回校验结果，包括是否合法、错误信息和建议。`,
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"ir": {
						Type:        "string",
						Description: "Action IR JSON 字符串",
					},
				},
				Required: []string{"ir"},
			},
		},
		{
			Name: "create_automation",
			Description: `生成一个 Home Assistant 自动化配置草稿，保存到 pending 目录等待用户确认。

⚠️ 此工具不会直接部署到 HA，需要用户确认后调用 confirm_automation 才会部署。

## 配置规范（必须遵守）

### ⚠️ 分组规范（重要！）
创建自动化前必须先判断属于哪个分组，命名必须符合分组规则：
- **人来灯亮**: 命名包含 "_有人_开灯" 或 "_有人移动_开灯"
- **人走灯灭**: 命名包含 "_无人_关灯" 或 "_无人5分钟_关灯"
- **热水器**: 命名包含 "热水器"
- **马桶换气**: 命名包含 "_坐马桶_开换气" 或 "_无人_关换气"
- **睡眠模式**: 命名包含 "_睡眠模式_打开"、"_睡眠模式_关闭" 或 "_关闭睡眠模式"
- **光暗灯亮**: 命名包含 "_光暗_开灯"

如果是新类型的自动化，请先告知用户需要添加新的分组模式，不要放到"其他"分组！

### 命名规范
格式：[房间]_[触发条件]_[动作]
示例：客厅_有人移动_开灯、卧室_早上7点_开窗帘、全屋_离家_关闭所有灯

### 组织原则
1. 一个自动化只针对一个房间（除非是全屋场景如离家/回家/睡眠模式）
2. 一个自动化一个主触发器，避免混合不相关触发器
3. 一个自动化专注一类动作，避免混合不相关动作
4. 如果用户需求涉及多个房间，应拆分成多个独立的自动化

### 全屋场景例外
以下场景可以跨房间：离家模式、回家模式、睡眠模式、紧急模式
命名格式：全屋_[场景]_[动作]

### ⚠️ 全局变量（必须使用）
灯光自动化中的色温、亮度等参数必须使用全局变量，不要硬编码：
- 色温：使用 input_number.global_color_temp（如果不存在则先用 create_input_number 创建）
- 亮度：使用 input_number.global_brightness（如果不存在则先创建）

在 action 的 data 中使用模板引用：
"data": {"color_temp_kelvin": "{{ states('input_number.global_color_temp') | int }}"}

## Automation IR 格式
{
  "name": "客厅_晚间有人_开灯",
  "trigger": {"type": "state", "entity": "binary_sensor.motion_living_room", "to": "on"},
  "conditions": [{"type": "time", "after": "19:00", "before": "23:00"}],
  "actions": [{"action": "call_service", "service": "light.turn_on", "target": "light.living_room", "data": {"color_temp_kelvin": "{{ states('input_number.global_color_temp') | int }}"}}],
  "constraints": {"mode": "restart"},
  "labels": ["人来灯亮"]
}

支持的 trigger.type: state, time
- state trigger 支持 for 参数: {"type": "state", "entity": "xxx", "to": "on", "for": {"minutes": 3}}
支持的 condition.type: state, time, numeric_state
推荐 mode: restart（人体感应）、single（定时任务）、queued（按钮触发）
labels: 可选，用于分组自动化，如 ["人来灯亮"]、["热水器"]`,
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"ir": {
						Type:        "string",
						Description: "Automation IR JSON 字符串",
					},
				},
				Required: []string{"ir"},
			},
		},
		{
			Name: "confirm_automation",
			Description: `确认并部署一个待审核的自动化配置。

用户确认后调用此工具，会将 pending 目录中的配置部署到 Home Assistant，
并移动到 automations 目录，同时提交 git commit。`,
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"file_path": {
						Type:        "string",
						Description: "待确认的自动化配置文件路径（pending 目录下的 yaml 文件）",
					},
				},
				Required: []string{"file_path"},
			},
		},
		{
			Name:        "delete_automation",
			Description: "删除一个 Home Assistant 自动化规则。",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"automation_id": {
						Type:        "string",
						Description: "自动化的 ID，例如 1234567890",
					},
				},
				Required: []string{"automation_id"},
			},
		},
		{
			Name: "update_automation",
			Description: `更新一个已存在的 Home Assistant 自动化规则。

通过自动化 ID 更新其配置。可以从 list_automations 获取自动化的 ID。
配置格式与 create_automation 相同。

更新成功后会自动同步配置到本地并提交 git。`,
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"automation_id": {
						Type:        "string",
						Description: "自动化的 ID（从 list_automations 获取）",
					},
					"config": {
						Type:        "string",
						Description: "自动化配置的 YAML 字符串",
					},
				},
				Required: []string{"automation_id", "config"},
			},
		},
		{
			Name:        "list_pending",
			Description: "列出所有待确认的自动化草稿（pending 目录中的配置）。",
			InputSchema: InputSchema{
				Type: "object",
			},
		},
		{
			Name: "sync_automations",
			Description: `从 Home Assistant 同步自动化配置到本地。

用于将 HA 中的自动化配置拉取到本地 ha-config 仓库。
- 如果不指定 automation_ids，则同步所有自动化
- 如果指定 automation_ids，则只同步指定的自动化

同步后会自动提交 git commit。`,
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"automation_ids": {
						Type:        "string",
						Description: "要同步的自动化 ID 列表，用逗号分隔。留空则同步所有自动化。",
					},
				},
			},
		},
		{
			Name:        "cancel_pending",
			Description: "取消/删除一个待确认的自动化草稿。",
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"file_path": {
						Type:        "string",
						Description: "待删除的草稿文件路径",
					},
				},
				Required: []string{"file_path"},
			},
		},
		{
			Name: "create_template_sensor",
			Description: `创建一个 Template Sensor（模板传感器）。

Template Sensor 可以通过 Jinja2 模板动态计算值，常用于：
- 聚合多个传感器的数据（如计算平均值、最大值）
- 基于条件过滤数据（如只取未开灯房间的光照）
- 创建虚拟传感器

参数说明：
- name: 传感器名称（如 "全局参考光照"）
- unique_id: 唯一标识符（如 "global_reference_illumination"）
- state_template: Jinja2 模板，计算传感器的值
- unit: 单位（如 "lx"）
- device_class: 设备类型（如 "illuminance"）

注意：此工具通过 HA 的 POST /api/states 接口创建传感器，
传感器会在 HA 重启后消失。如需持久化，需要在 configuration.yaml 中配置。`,
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"name": {
						Type:        "string",
						Description: "传感器名称",
					},
					"unique_id": {
						Type:        "string",
						Description: "唯一标识符，用于生成 entity_id",
					},
					"state_template": {
						Type:        "string",
						Description: "Jinja2 模板，用于计算传感器的值",
					},
					"unit": {
						Type:        "string",
						Description: "单位，如 lx, °C, %",
					},
					"device_class": {
						Type:        "string",
						Description: "设备类型，如 illuminance, temperature, humidity",
					},
				},
				Required: []string{"name", "unique_id", "state_template"},
			},
		},
		{
			Name: "update_template_sensor",
			Description: `更新一个 Template Sensor 的值。

通过执行模板计算并更新传感器状态。适用于需要定期刷新的模板传感器。`,
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"entity_id": {
						Type:        "string",
						Description: "传感器的 entity_id，如 sensor.global_reference_illumination",
					},
					"state_template": {
						Type:        "string",
						Description: "Jinja2 模板，用于计算新的值",
					},
				},
				Required: []string{"entity_id", "state_template"},
			},
		},
		{
			Name: "render_template",
			Description: `执行一个 Jinja2 模板并返回结果。

用于测试模板是否正确，或获取模板计算的值。

示例模板：
- 获取光照值：{{ states('sensor.xxx_illumination') }}
- 计算最大值：{{ [states('sensor.a')|float, states('sensor.b')|float] | max }}
- 条件过滤：{% if is_state('light.xxx', 'off') %}{{ states('sensor.xxx') }}{% endif %}`,
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"template": {
						Type:        "string",
						Description: "Jinja2 模板字符串",
					},
				},
				Required: []string{"template"},
			},
		},
		{
			Name: "reload_integration",
			Description: `重新加载指定集成的配置。

常用的 domain 参数：
- homeassistant: 重新加载核心配置
- automation: 重新加载自动化
- script: 重新加载脚本
- scene: 重新加载场景
- template: 重新加载模板传感器
- input_boolean: 重新加载输入布尔值
- input_number: 重新加载输入数字
- xiaomi_miot: 重新加载小米设备（会重新发现新设备）

用于在添加新设备或修改配置后，无需重启 HA 即可生效。`,
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"domain": {
						Type:        "string",
						Description: "要重新加载的集成域名，如 automation, template, xiaomi_miot",
					},
				},
				Required: []string{"domain"},
			},
		},
		{
			Name: "reload_config_entry",
			Description: `重新加载指定的配置条目。

通过 entry_id 重新加载特定的集成配置，用于刷新单个设备或集成。`,
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"entry_id": {
						Type:        "string",
						Description: "配置条目的 ID",
					},
				},
				Required: []string{"entry_id"},
			},
		},
		{
			Name: "reload_all",
			Description: `重新加载所有可重新加载的集成。

一次性重新加载所有支持热重载的集成配置，包括自动化、脚本、场景等。`,
			InputSchema: InputSchema{
				Type: "object",
			},
		},
		{
			Name: "list_categories",
			Description: `列出所有自动化分组（category）。

返回 HA 中已创建的自动化分组列表。`,
			InputSchema: InputSchema{
				Type: "object",
			},
		},
		{
			Name: "create_category",
			Description: `创建一个新的自动化分组（category）。

用于在 HA 中创建分组，然后可以将自动化分配到该分组。`,
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"name": {
						Type:        "string",
						Description: "分组名称，如 \"人来灯亮\"、\"热水器\"",
					},
					"icon": {
						Type:        "string",
						Description: "可选，分组图标，如 \"mdi:lightbulb\"",
					},
				},
				Required: []string{"name"},
			},
		},
		{
			Name: "assign_category",
			Description: `将自动化分配到指定分组。

将一个或多个自动化分配到已创建的分组中。`,
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"entity_ids": {
						Type:        "string",
						Description: "自动化的 entity_id 列表，用逗号分隔，如 \"automation.xxx,automation.yyy\"",
					},
					"category_id": {
						Type:        "string",
						Description: "分组的 ID",
					},
				},
				Required: []string{"entity_ids", "category_id"},
			},
		},
		{
			Name: "rename_entity",
			Description: `重命名实体的 entity_id。

用于将自动生成的长 entity_id 改为更易读的名称。
例如将 light.mijia_cn_group_1992129452574117888_group3_s_2_light 改为 light.kewei_dengzu`,
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"old_entity_id": {
						Type:        "string",
						Description: "当前的 entity_id",
					},
					"new_entity_id": {
						Type:        "string",
						Description: "新的 entity_id，需要保持相同的域名前缀（如 light.）",
					},
				},
				Required: []string{"old_entity_id", "new_entity_id"},
			},
		},
		{
			Name: "set_entity_name",
			Description: `设置实体的显示名称（friendly_name）。

用于将实体在 HA UI 中显示的名称改为更易读的中文名称。
entity_id 不变，只改变显示名称。`,
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"entity_id": {
						Type:        "string",
						Description: "实体的 entity_id",
					},
					"name": {
						Type:        "string",
						Description: "新的显示名称，支持中文",
					},
				},
				Required: []string{"entity_id", "name"},
			},
		},
		{
			Name: "create_input_number",
			Description: `创建一个 input_number 帮助程序（全局变量）。

用于存储全局配置值，如色温、亮度等，方便在多个自动化中引用。
创建后可以在自动化中通过模板 {{ states('input_number.xxx') }} 引用。

⚠️ 重要：创建灯光自动化时，应使用全局变量而非硬编码值：
- 色温：使用 input_number.global_color_temp（如果不存在则先创建）
- 亮度：使用 input_number.global_brightness（如果不存在则先创建）`,
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"name": {
						Type:        "string",
						Description: "变量名称，如 \"全局色温\"",
					},
					"min": {
						Type:        "number",
						Description: "最小值",
					},
					"max": {
						Type:        "number",
						Description: "最大值",
					},
					"step": {
						Type:        "number",
						Description: "步进值",
					},
					"initial": {
						Type:        "number",
						Description: "初始值",
					},
					"unit": {
						Type:        "string",
						Description: "单位，如 \"K\"（色温）、\"%\"（亮度）",
					},
					"icon": {
						Type:        "string",
						Description: "图标，如 \"mdi:thermometer\"",
					},
				},
				Required: []string{"name", "min", "max", "initial"},
			},
		},
		{
			Name: "migrate_automations",
			Description: `迁移自动化文件到分组目录结构。

将 automations 目录下的所有自动化文件按功能分组移动到子目录，并生成每个分组的 README.md 文档。

分组规则：
- 人来灯亮：包含 "_有人_开灯" 的自动化
- 人走灯灭：包含 "_无人_关灯" 的自动化
- 热水器：包含 "热水器" 的自动化
- 其他：不匹配以上规则的自动化

迁移后的目录结构：
automations/
├── 人来灯亮/
│   ├── README.md
│   ├── 主卧_有人_开灯.yaml
│   └── ...
├── 人走灯灭/
│   ├── README.md
│   ├── 主卧_无人_关灯.yaml
│   └── ...
└── 热水器/
    ├── README.md
    └── ...`,
			InputSchema: InputSchema{
				Type: "object",
			},
		},
		{
			Name: "illumination_report",
			Description: `生成全屋光照报告。

获取全局光照传感器和各房间光照传感器的当前值，对比各房间"人来灯亮"自动化的阈值，
生成一份完整的光照报告，包括：
- 全局光照值
- 各房间本地光照值
- 各房间阈值
- 是否应该开灯的判断
- 系数分析（本地光照/全局光照）

用于检查光照阈值设置是否合理。`,
			InputSchema: InputSchema{
				Type: "object",
			},
		},
		{
			Name: "get_entity_device_info",
			Description: `获取实体的设备信息，包括 device_id 和 entity 内部 ID。

用于获取创建自动化时需要的 device_id 和 entity_id（内部 ID，非完整 entity_id）。
例如小爱音箱的 TTS 功能需要使用 device_id 和 text entity 的内部 ID。

返回信息包括：
- entity_id: 完整的实体 ID
- device_id: 设备 ID（用于自动化中的 device_id 字段）
- unique_id: 实体的唯一 ID（通常就是内部 entity_id）
- platform: 平台名称
- name: 显示名称`,
			InputSchema: InputSchema{
				Type: "object",
				Properties: map[string]Property{
					"entity_id": {
						Type:        "string",
						Description: "实体的 entity_id，如 media_player.xiaomi_cn_866197674_oh2p",
					},
				},
				Required: []string{"entity_id"},
			},
		},
	}
}

func (s *Server) callTool(name string, args map[string]any) CallToolResult {
	var result string
	var isError bool

	switch name {
	case "get_devices":
		devices, err := s.haClient.GetDevices()
		if err != nil {
			result = fmt.Sprintf("Error: %v", err)
			isError = true
		} else {
			data, _ := json.MarshalIndent(devices, "", "  ")
			result = string(data)
		}

	case "search_devices":
		keyword, _ := args["keyword"].(string)
		devices, err := s.haClient.GetDevices()
		if err != nil {
			result = fmt.Sprintf("Error: %v", err)
			isError = true
		} else {
			keywords := strings.Fields(strings.ToLower(keyword))
			var matched []ha.DeviceCapability
			for _, d := range devices {
				// 搜索 entity_id、名称和区域（只在有值时搜索）
				searchText := strings.ToLower(d.EntityID + " " + d.Name)
				if d.Area != "" {
					searchText += " " + strings.ToLower(d.Area)
				}
				allMatch := true
				for _, kw := range keywords {
					if !strings.Contains(searchText, kw) {
						allMatch = false
						break
					}
				}
				if allMatch {
					matched = append(matched, d)
				}
			}
			if len(matched) == 0 {
				result = fmt.Sprintf("未找到匹配 \"%s\" 的设备", keyword)
			} else {
				// 按域分组展示，更清晰
				var sb strings.Builder
				sb.WriteString(fmt.Sprintf("找到 %d 个匹配的设备（必须全部展示给用户，不要省略）:\n\n", len(matched)))
				grouped := make(map[string][]ha.DeviceCapability)
				for _, d := range matched {
					grouped[d.Domain] = append(grouped[d.Domain], d)
				}
				for domain, devs := range grouped {
					sb.WriteString(fmt.Sprintf("## %s (%d个)\n", domain, len(devs)))
					for _, d := range devs {
						area := ""
						if d.Area != "" {
							area = fmt.Sprintf(" [%s]", d.Area)
						}
						sb.WriteString(fmt.Sprintf("- %s (%s)%s\n", d.EntityID, d.Name, area))
					}
					sb.WriteString("\n")
				}
				sb.WriteString("提示：如需查看设备状态，请使用 get_state 工具获取实时状态。")
				result = sb.String()
			}
		}

	case "get_state":
		entityID, _ := args["entity_id"].(string)
		state, err := s.haClient.GetState(entityID)
		if err != nil {
			result = fmt.Sprintf("Error: %v", err)
			isError = true
		} else {
			data, _ := json.MarshalIndent(state, "", "  ")
			result = string(data)
		}

	case "call_service":
		domain, _ := args["domain"].(string)
		service, _ := args["service"].(string)
		entityID, _ := args["entity_id"].(string)

		serviceData := map[string]any{
			"entity_id": entityID,
		}

		if dataStr, ok := args["data"].(string); ok && dataStr != "" {
			var extraData map[string]any
			if err := json.Unmarshal([]byte(dataStr), &extraData); err == nil {
				for k, v := range extraData {
					serviceData[k] = v
				}
			}
		}

		if err := s.haClient.CallService(domain, service, serviceData); err != nil {
			result = fmt.Sprintf("Error: %v", err)
			isError = true
		} else {
			result = fmt.Sprintf("Successfully called %s.%s on %s", domain, service, entityID)
		}

	case "list_automations":
		automations, err := s.haClient.GetAutomations()
		if err != nil {
			result = fmt.Sprintf("Error: %v", err)
			isError = true
		} else {
			data, _ := json.MarshalIndent(automations, "", "  ")
			result = string(data)
		}

	case "trigger_automation":
		entityID, _ := args["entity_id"].(string)
		err := s.haClient.CallService("automation", "trigger", map[string]any{
			"entity_id": entityID,
		})
		if err != nil {
			result = fmt.Sprintf("Error: %v", err)
			isError = true
		} else {
			result = fmt.Sprintf("Successfully triggered %s", entityID)
		}

	case "get_ha_config":
		config, err := s.haClient.GetConfig()
		if err != nil {
			result = fmt.Sprintf("Error: %v", err)
			isError = true
		} else {
			data, _ := json.MarshalIndent(config, "", "  ")
			result = string(data)
		}

	case "validate_ir":
		irStr, _ := args["ir"].(string)
		validationResult, err := s.validateIR(irStr)
		if err != nil {
			result = fmt.Sprintf("Error parsing IR: %v", err)
			isError = true
		} else {
			data, _ := json.MarshalIndent(validationResult, "", "  ")
			result = string(data)
		}

	case "execute_ir":
		irStr, _ := args["ir"].(string)
		execResult, err := s.executeIR(irStr)
		if err != nil {
			result = fmt.Sprintf("Error: %v", err)
			isError = true
		} else {
			result = execResult
		}

	case "create_automation":
		irStr, _ := args["ir"].(string)
		createResult, err := s.createAutomation(irStr)
		if err != nil {
			result = fmt.Sprintf("Error: %v", err)
			isError = true
		} else {
			result = createResult
		}

	case "confirm_automation":
		filePath, _ := args["file_path"].(string)
		confirmResult, err := s.confirmAutomation(filePath)
		if err != nil {
			result = fmt.Sprintf("Error: %v", err)
			isError = true
		} else {
			result = confirmResult
		}

	case "delete_automation":
		automationID, _ := args["automation_id"].(string)
		// First get the automation config to find the alias for local file
		config, _ := s.haClient.GetAutomationConfig(automationID)
		alias, _ := config["alias"].(string)

		if err := s.haClient.DeleteAutomation(automationID); err != nil {
			result = fmt.Sprintf("Error: %v", err)
			isError = true
		} else {
			// Delete local file and commit
			configRepo := os.Getenv("HAC_CONFIG_REPO")
			if configRepo != "" && alias != "" {
				// Find file in group directory
				group := getAutomationGroup(alias)
				groupDir := filepath.Join(configRepo, "automations", group)
				filePath := filepath.Join(groupDir, alias+".yaml")
				if err := os.Remove(filePath); err == nil {
					// Regenerate group README
					s.generateGroupREADME(groupDir, group)
					gitAdd(configRepo, groupDir)
					gitCommit(configRepo, fmt.Sprintf("Delete automation: %s", alias))
				}
			}
			result = fmt.Sprintf("✓ 成功删除自动化: %s\n✓ 已同步删除本地文件并提交 git", alias)
		}

	case "update_automation":
		automationID, _ := args["automation_id"].(string)
		configYAML, _ := args["config"].(string)
		var config map[string]any
		if err := yaml.Unmarshal([]byte(configYAML), &config); err != nil {
			result = fmt.Sprintf("Error parsing YAML: %v", err)
			isError = true
		} else {
			config["id"] = automationID
			if err := s.haClient.UpdateAutomation(automationID, config); err != nil {
				result = fmt.Sprintf("Error: %v", err)
				isError = true
			} else {
				alias, _ := config["alias"].(string)
				// Auto sync to local
				configRepo := os.Getenv("HAC_CONFIG_REPO")
				syncResult, syncErr := s.syncAutomationConfig(configRepo, automationID)
				if syncErr != nil {
					result = fmt.Sprintf("✓ 成功更新自动化: %s (ID: %s)\n⚠️ 同步本地失败: %v", alias, automationID, syncErr)
				} else {
					result = fmt.Sprintf("✓ 成功更新自动化: %s (ID: %s)\n%s", alias, automationID, syncResult)
				}
			}
		}

	case "list_pending":
		listResult, err := s.listPending()
		if err != nil {
			result = fmt.Sprintf("Error: %v", err)
			isError = true
		} else {
			result = listResult
		}

	case "cancel_pending":
		filePath, _ := args["file_path"].(string)
		if err := s.cancelPending(filePath); err != nil {
			result = fmt.Sprintf("Error: %v", err)
			isError = true
		} else {
			result = fmt.Sprintf("✓ Deleted pending automation: %s", filePath)
		}

	case "sync_automations":
		automationIDs, _ := args["automation_ids"].(string)
		syncResult, err := s.syncAutomations(automationIDs)
		if err != nil {
			result = fmt.Sprintf("Error: %v", err)
			isError = true
		} else {
			result = syncResult
		}

	case "create_template_sensor":
		name, _ := args["name"].(string)
		uniqueID, _ := args["unique_id"].(string)
		stateTemplate, _ := args["state_template"].(string)
		unit, _ := args["unit"].(string)
		deviceClass, _ := args["device_class"].(string)

		createResult, err := s.createTemplateSensor(name, uniqueID, stateTemplate, unit, deviceClass)
		if err != nil {
			result = fmt.Sprintf("Error: %v", err)
			isError = true
		} else {
			result = createResult
		}

	case "update_template_sensor":
		entityID, _ := args["entity_id"].(string)
		stateTemplate, _ := args["state_template"].(string)

		updateResult, err := s.updateTemplateSensor(entityID, stateTemplate)
		if err != nil {
			result = fmt.Sprintf("Error: %v", err)
			isError = true
		} else {
			result = updateResult
		}

	case "render_template":
		template, _ := args["template"].(string)
		rendered, err := s.haClient.RenderTemplate(template)
		if err != nil {
			result = fmt.Sprintf("Error: %v", err)
			isError = true
		} else {
			result = rendered
		}

	case "reload_integration":
		domain, _ := args["domain"].(string)
		if err := s.haClient.ReloadIntegration(domain); err != nil {
			result = fmt.Sprintf("Error: %v", err)
			isError = true
		} else {
			result = fmt.Sprintf("✓ Successfully reloaded integration: %s", domain)
		}

	case "reload_config_entry":
		entryID, _ := args["entry_id"].(string)
		if err := s.haClient.ReloadConfigEntry(entryID); err != nil {
			result = fmt.Sprintf("Error: %v", err)
			isError = true
		} else {
			result = fmt.Sprintf("✓ Successfully reloaded config entry: %s", entryID)
		}

	case "reload_all":
		if err := s.haClient.ReloadAll(); err != nil {
			result = fmt.Sprintf("Error: %v", err)
			isError = true
		} else {
			result = "✓ Successfully reloaded all integrations"
		}

	case "list_categories":
		ws, err := s.haClient.NewWSClient()
		if err != nil {
			result = fmt.Sprintf("Error connecting to HA: %v", err)
			isError = true
		} else {
			defer ws.Close()
			categories, err := ws.ListCategories("automation")
			if err != nil {
				result = fmt.Sprintf("Error: %v", err)
				isError = true
			} else {
				if len(categories) == 0 {
					result = "没有找到自动化分组"
				} else {
					var sb strings.Builder
					sb.WriteString(fmt.Sprintf("找到 %d 个自动化分组:\n\n", len(categories)))
					for _, cat := range categories {
						sb.WriteString(fmt.Sprintf("- **%s** (ID: %s)\n", cat.Name, cat.CategoryID))
					}
					result = sb.String()
				}
			}
		}

	case "create_category":
		name, _ := args["name"].(string)
		icon, _ := args["icon"].(string)
		ws, err := s.haClient.NewWSClient()
		if err != nil {
			result = fmt.Sprintf("Error connecting to HA: %v", err)
			isError = true
		} else {
			defer ws.Close()
			cat, err := ws.CreateCategory("automation", name, icon)
			if err != nil {
				result = fmt.Sprintf("Error: %v", err)
				isError = true
			} else {
				result = fmt.Sprintf("✓ 创建分组成功: %s (ID: %s)", cat.Name, cat.CategoryID)
			}
		}

	case "assign_category":
		entityIDs, _ := args["entity_ids"].(string)
		categoryID, _ := args["category_id"].(string)
		ws, err := s.haClient.NewWSClient()
		if err != nil {
			result = fmt.Sprintf("Error connecting to HA: %v", err)
			isError = true
		} else {
			defer ws.Close()
			ids := strings.Split(entityIDs, ",")
			var successCount int
			var errors []string
			for _, id := range ids {
				id = strings.TrimSpace(id)
				if id == "" {
					continue
				}
				if err := ws.AssignCategory("automation", id, categoryID); err != nil {
					errors = append(errors, fmt.Sprintf("%s: %v", id, err))
				} else {
					successCount++
				}
			}
			if len(errors) > 0 {
				result = fmt.Sprintf("✓ 成功分配 %d 个自动化\n✗ 失败 %d 个:\n%s", successCount, len(errors), strings.Join(errors, "\n"))
				if successCount == 0 {
					isError = true
				}
			} else {
				result = fmt.Sprintf("✓ 成功将 %d 个自动化分配到分组", successCount)
			}
		}

	case "rename_entity":
		oldEntityID, _ := args["old_entity_id"].(string)
		newEntityID, _ := args["new_entity_id"].(string)
		ws, err := s.haClient.NewWSClient()
		if err != nil {
			result = fmt.Sprintf("Error connecting to HA: %v", err)
			isError = true
		} else {
			defer ws.Close()
			if err := ws.RenameEntityID(oldEntityID, newEntityID); err != nil {
				result = fmt.Sprintf("Error: %v", err)
				isError = true
			} else {
				// Update local automation files that reference this entity
				configRepo := os.Getenv("HAC_CONFIG_REPO")
				updatedFiles := s.updateEntityIDInLocalFiles(configRepo, oldEntityID, newEntityID)
				if len(updatedFiles) > 0 {
					gitAdd(configRepo, filepath.Join(configRepo, "automations"))
					gitCommit(configRepo, fmt.Sprintf("Rename entity: %s -> %s", oldEntityID, newEntityID))
					result = fmt.Sprintf("✓ 成功重命名: %s -> %s\n✓ 已更新 %d 个本地文件并提交 git", oldEntityID, newEntityID, len(updatedFiles))
				} else {
					result = fmt.Sprintf("✓ 成功重命名: %s -> %s", oldEntityID, newEntityID)
				}
			}
		}

	case "set_entity_name":
		entityID, _ := args["entity_id"].(string)
		name, _ := args["name"].(string)
		ws, err := s.haClient.NewWSClient()
		if err != nil {
			result = fmt.Sprintf("Error connecting to HA: %v", err)
			isError = true
		} else {
			defer ws.Close()
			if err := ws.SetEntityName(entityID, name); err != nil {
				result = fmt.Sprintf("Error: %v", err)
				isError = true
			} else {
				result = fmt.Sprintf("✓ 成功设置显示名称: %s -> %s", entityID, name)
			}
		}

	case "create_input_number":
		name, _ := args["name"].(string)
		min, _ := args["min"].(float64)
		max, _ := args["max"].(float64)
		step, _ := args["step"].(float64)
		if step == 0 {
			step = 1
		}
		initial, _ := args["initial"].(float64)
		unit, _ := args["unit"].(string)
		icon, _ := args["icon"].(string)

		ws, err := s.haClient.NewWSClient()
		if err != nil {
			result = fmt.Sprintf("Error connecting to HA: %v", err)
			isError = true
		} else {
			defer ws.Close()
			entityID, err := ws.CreateInputNumber(name, min, max, step, initial, unit, icon)
			if err != nil {
				result = fmt.Sprintf("Error: %v", err)
				isError = true
			} else {
				// Auto sync to local
				configRepo := os.Getenv("HAC_CONFIG_REPO")
				syncResult, syncErr := s.syncInputNumberConfig(configRepo)
				if syncErr != nil {
					result = fmt.Sprintf("✓ 成功创建 input_number: %s\n初始值: %.0f %s\n范围: %.0f - %.0f\n⚠️ 同步本地失败: %v", entityID, initial, unit, min, max, syncErr)
				} else {
					result = fmt.Sprintf("✓ 成功创建 input_number: %s\n初始值: %.0f %s\n范围: %.0f - %.0f\n%s", entityID, initial, unit, min, max, syncResult)
				}
			}
		}

	case "migrate_automations":
		migrateResult, err := s.migrateAutomations()
		if err != nil {
			result = fmt.Sprintf("Error: %v", err)
			isError = true
		} else {
			result = migrateResult
		}

	case "illumination_report":
		reportResult, err := s.generateIlluminationReport()
		if err != nil {
			result = fmt.Sprintf("Error: %v", err)
			isError = true
		} else {
			result = reportResult
		}

	case "get_entity_device_info":
		entityID, _ := args["entity_id"].(string)
		ws, err := s.haClient.NewWSClient()
		if err != nil {
			result = fmt.Sprintf("Error connecting to HA WebSocket: %v", err)
			isError = true
		} else {
			defer ws.Close()
			info, err := ws.GetEntityDeviceInfo(entityID)
			if err != nil {
				result = fmt.Sprintf("Error: %v", err)
				isError = true
			} else {
				data, _ := json.MarshalIndent(info, "", "  ")
				result = string(data)
			}
		}

	default:
		result = fmt.Sprintf("Unknown tool: %s", name)
		isError = true
	}

	return CallToolResult{
		Content: []Content{{Type: "text", Text: result}},
		IsError: isError,
	}
}

func (s *Server) sendResult(id any, result any) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	s.send(resp)
}

func (s *Server) sendError(id any, code int, message string) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &Error{Code: code, Message: message},
	}
	s.send(resp)
}

func (s *Server) send(resp Response) {
	data, _ := json.Marshal(resp)
	fmt.Println(string(data))
}

func (s *Server) getValidator() (*ir.Validator, error) {
	devices, err := s.haClient.GetDevices()
	if err != nil {
		return nil, fmt.Errorf("get devices: %w", err)
	}

	services, err := s.haClient.GetServices()
	if err != nil {
		return nil, fmt.Errorf("get services: %w", err)
	}

	return ir.NewValidator(devices, services), nil
}

func (s *Server) validateIR(irStr string) (*ir.ValidationResult, error) {
	var actionIR ir.ActionIR
	if err := json.Unmarshal([]byte(irStr), &actionIR); err != nil {
		return nil, fmt.Errorf("parse IR JSON: %w", err)
	}

	validator, err := s.getValidator()
	if err != nil {
		return nil, err
	}

	result := validator.ValidateAction(&actionIR)
	return &result, nil
}

func (s *Server) executeIR(irStr string) (string, error) {
	var actionIR ir.ActionIR
	if err := json.Unmarshal([]byte(irStr), &actionIR); err != nil {
		return "", fmt.Errorf("parse IR JSON: %w", err)
	}

	validator, err := s.getValidator()
	if err != nil {
		return "", err
	}

	result := validator.ValidateAction(&actionIR)
	if !result.Valid {
		data, _ := json.MarshalIndent(result, "", "  ")
		return "", fmt.Errorf("validation failed:\n%s", string(data))
	}

	switch actionIR.Action {
	case "call_service":
		parts := splitService(actionIR.Service)
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid service format: %s", actionIR.Service)
		}

		serviceData := map[string]any{
			"entity_id": actionIR.Target,
		}
		for k, v := range actionIR.Data {
			serviceData[k] = v
		}

		if err := s.haClient.CallService(parts[0], parts[1], serviceData); err != nil {
			return "", fmt.Errorf("call service: %w", err)
		}

		return fmt.Sprintf("Successfully executed: %s on %s", actionIR.Service, actionIR.Target), nil

	default:
		return "", fmt.Errorf("unsupported action: %s", actionIR.Action)
	}
}

func splitService(service string) []string {
	for i, c := range service {
		if c == '.' {
			return []string{service[:i], service[i+1:]}
		}
	}
	return []string{service}
}

func (s *Server) createAutomation(irStr string) (string, error) {
	var automationIR ir.AutomationIR
	if err := json.Unmarshal([]byte(irStr), &automationIR); err != nil {
		return "", fmt.Errorf("parse Automation IR JSON: %w", err)
	}

	validator, err := s.getValidator()
	if err != nil {
		return "", err
	}

	result := validator.ValidateAutomation(&automationIR)
	if !result.Valid {
		data, _ := json.MarshalIndent(result, "", "  ")
		return "", fmt.Errorf("validation failed:\n%s", string(data))
	}

	haAutomation, err := ir.CompileAutomation(&automationIR)
	if err != nil {
		return "", fmt.Errorf("compile automation: %w", err)
	}

	automationMap := map[string]any{
		"alias":   haAutomation.Alias,
		"mode":    haAutomation.Mode,
		"trigger": haAutomation.Trigger,
		"action":  haAutomation.Action,
	}
	if len(haAutomation.Condition) > 0 {
		automationMap["condition"] = haAutomation.Condition
	}
	if haAutomation.Description != "" {
		automationMap["description"] = haAutomation.Description
	}

	// Validate all entity_ids exist in HA
	missingEntities := s.validateEntityIDs(automationMap)
	var warningMsg string
	if len(missingEntities) > 0 {
		warningMsg = fmt.Sprintf("\n\n⚠️ 警告：以下 entity_id 在 HA 中不存在：\n- %s\n请确认这些实体是否正确。", strings.Join(missingEntities, "\n- "))
	}

	// Save to pending directory for user review (don't deploy yet)
	configRepo := os.Getenv("HAC_CONFIG_REPO")
	if configRepo == "" {
		return "", fmt.Errorf("HAC_CONFIG_REPO not configured. Run 'hac init' first")
	}

	filePath, err := s.savePendingAutomation(configRepo, automationIR.Name, automationMap)
	if err != nil {
		return "", fmt.Errorf("save pending automation: %w", err)
	}

	yamlData, _ := yaml.Marshal(automationMap)
	return fmt.Sprintf("📝 Automation draft saved to:\n%s\n\n```yaml\n%s```%s\n\n⚠️ Please review the configuration above.\nSay \"确认\" or \"confirm\" to deploy to Home Assistant.", filePath, string(yamlData), warningMsg), nil
}

// validateEntityIDs extracts all entity_ids from automation config and checks if they exist in HA
func (s *Server) validateEntityIDs(config map[string]any) []string {
	// Get all states from HA
	states, err := s.haClient.GetStates()
	if err != nil {
		return nil // Can't validate, skip
	}

	// Build a set of existing entity_ids
	existingIDs := make(map[string]bool)
	for _, state := range states {
		existingIDs[state.EntityID] = true
	}

	// Extract all entity_ids from config
	entityIDs := extractEntityIDs(config)

	// Find missing ones
	var missing []string
	for _, id := range entityIDs {
		if !existingIDs[id] {
			missing = append(missing, id)
		}
	}

	return missing
}

// extractEntityIDs recursively extracts all entity_id values from a config map
func extractEntityIDs(v any) []string {
	var ids []string

	switch val := v.(type) {
	case map[string]any:
		for k, v := range val {
			if k == "entity_id" {
				if id, ok := v.(string); ok {
					ids = append(ids, id)
				} else if idList, ok := v.([]any); ok {
					for _, item := range idList {
						if id, ok := item.(string); ok {
							ids = append(ids, id)
						}
					}
				}
			} else {
				ids = append(ids, extractEntityIDs(v)...)
			}
		}
	case []any:
		for _, item := range val {
			ids = append(ids, extractEntityIDs(item)...)
		}
	}

	return ids
}

func (s *Server) savePendingAutomation(repoPath, name string, automation map[string]any) (string, error) {
	pendingDir := filepath.Join(repoPath, "pending")
	if err := os.MkdirAll(pendingDir, 0755); err != nil {
		return "", fmt.Errorf("create pending dir: %w", err)
	}

	// Generate filename from name, replace unsafe characters for filesystem
	filename := strings.ReplaceAll(name, " ", "_")
	filename = strings.ReplaceAll(filename, "/", "_")
	filename = strings.ReplaceAll(filename, "\\", "_")
	filename = strings.ReplaceAll(filename, ":", "_")
	filename = strings.ReplaceAll(filename, "*", "_")
	filename = strings.ReplaceAll(filename, "?", "_")
	filename = strings.ReplaceAll(filename, "\"", "_")
	filename = strings.ReplaceAll(filename, "<", "_")
	filename = strings.ReplaceAll(filename, ">", "_")
	filename = strings.ReplaceAll(filename, "|", "_")

	if filename == "" {
		filename = fmt.Sprintf("automation_%d", time.Now().Unix())
	}
	filePath := filepath.Join(pendingDir, filename+".yaml")

	// Write YAML
	data, err := yaml.Marshal(automation)
	if err != nil {
		return "", fmt.Errorf("marshal yaml: %w", err)
	}
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	return filePath, nil
}

func (s *Server) syncAutomationConfig(configRepo, automationID string) (string, error) {
	// Get automation config from HA
	config, err := s.haClient.GetAutomationConfig(automationID)
	if err != nil {
		return "", fmt.Errorf("get automation config: %w", err)
	}

	// Get alias for filename
	alias, ok := config["alias"].(string)
	if !ok || alias == "" {
		return "", fmt.Errorf("automation has no alias")
	}

	// Determine group based on alias
	group := getAutomationGroup(alias)

	// Write to automations/[group] directory
	groupDir := filepath.Join(configRepo, "automations", group)
	if err := os.MkdirAll(groupDir, 0755); err != nil {
		return "", fmt.Errorf("create group dir: %w", err)
	}

	filename := alias + ".yaml"
	filePath := filepath.Join(groupDir, filename)

	// Remove old file from root automations/ if it exists (cleanup from old location)
	rootFile := filepath.Join(configRepo, "automations", filename)
	if rootFile != filePath {
		os.Remove(rootFile) // Ignore error if file doesn't exist
	}

	// Marshal to YAML
	data, err := yaml.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("marshal yaml: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	// Generate/update group README
	if err := s.generateGroupREADME(groupDir, group); err != nil {
		// Non-fatal, just log
		fmt.Fprintf(os.Stderr, "Warning: failed to generate README: %v\n", err)
	}

	// Git add and commit
	if err := gitAdd(configRepo, groupDir); err != nil {
		return "", fmt.Errorf("git add: %w", err)
	}
	commitMsg := fmt.Sprintf("Sync automation: %s", alias)
	if err := gitCommit(configRepo, commitMsg); err != nil {
		// If nothing to commit, that's OK
		if !strings.Contains(err.Error(), "nothing to commit") {
			return "", fmt.Errorf("git commit: %w", err)
		}
	}

	return fmt.Sprintf("✓ 已同步自动化配置到本地: %s\n✓ 已提交 git commit", filePath), nil
}

func (s *Server) syncAutomations(automationIDsStr string) (string, error) {
	configRepo := os.Getenv("HAC_CONFIG_REPO")
	if configRepo == "" {
		return "", fmt.Errorf("HAC_CONFIG_REPO environment variable not set")
	}

	var automationIDs []string
	if automationIDsStr != "" {
		// Parse comma-separated IDs
		for _, id := range strings.Split(automationIDsStr, ",") {
			id = strings.TrimSpace(id)
			if id != "" {
				automationIDs = append(automationIDs, id)
			}
		}
	}

	// If no specific IDs provided, get all automations
	if len(automationIDs) == 0 {
		automations, err := s.haClient.GetAutomations()
		if err != nil {
			return "", fmt.Errorf("get automations: %w", err)
		}
		for _, a := range automations {
			if id, ok := a.Attributes["id"].(string); ok && id != "" {
				automationIDs = append(automationIDs, id)
			}
		}
	}

	if len(automationIDs) == 0 {
		return "没有找到需要同步的自动化", nil
	}

	var synced []string
	var errors []string

	for _, id := range automationIDs {
		result, err := s.syncAutomationConfig(configRepo, id)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", id, err))
		} else {
			// Extract alias from result
			synced = append(synced, result)
		}
	}

	// Final git commit for all synced automations
	if len(synced) > 0 {
		gitAdd(configRepo, filepath.Join(configRepo, "automations"))
		gitCommit(configRepo, fmt.Sprintf("Sync %d automations from HA", len(synced)))
	}

	var resultParts []string
	if len(synced) > 0 {
		resultParts = append(resultParts, fmt.Sprintf("✓ 成功同步 %d 个自动化", len(synced)))
	}
	if len(errors) > 0 {
		resultParts = append(resultParts, fmt.Sprintf("✗ %d 个同步失败:\n  - %s", len(errors), strings.Join(errors, "\n  - ")))
	}

	return strings.Join(resultParts, "\n"), nil
}

func (s *Server) syncInputNumberConfig(configRepo string) (string, error) {
	// Get all input_number states from HA
	states, err := s.haClient.GetStates()
	if err != nil {
		return "", fmt.Errorf("get states: %w", err)
	}

	// Filter input_number entities
	var inputNumbers []map[string]any
	for _, state := range states {
		if strings.HasPrefix(state.EntityID, "input_number.") {
			// Only sync editable (user-created) input_numbers
			if editable, ok := state.Attributes["editable"].(bool); ok && editable {
				entry := map[string]any{
					"name":    state.Attributes["friendly_name"],
					"min":     state.Attributes["min"],
					"max":     state.Attributes["max"],
					"step":    state.Attributes["step"],
					"initial": state.Attributes["initial"],
				}
				if unit, ok := state.Attributes["unit_of_measurement"].(string); ok && unit != "" {
					entry["unit_of_measurement"] = unit
				}
				if icon, ok := state.Attributes["icon"].(string); ok && icon != "" {
					entry["icon"] = icon
				}
				// Use entity_id suffix as key
				key := strings.TrimPrefix(state.EntityID, "input_number.")
				inputNumbers = append(inputNumbers, map[string]any{key: entry})
			}
		}
	}

	if len(inputNumbers) == 0 {
		return "没有找到用户创建的 input_number", nil
	}

	// Build config map
	configMap := make(map[string]any)
	for _, item := range inputNumbers {
		for k, v := range item {
			configMap[k] = v
		}
	}

	// Write to input_number.yaml
	filePath := filepath.Join(configRepo, "input_number.yaml")
	data, err := yaml.Marshal(configMap)
	if err != nil {
		return "", fmt.Errorf("marshal yaml: %w", err)
	}

	// Add header comment
	header := "# 全局变量配置 - 由 hac sync_config 自动生成\n\n"
	if err := os.WriteFile(filePath, []byte(header+string(data)), 0644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	// Git add and commit
	if err := gitAdd(configRepo, filePath); err != nil {
		return "", fmt.Errorf("git add: %w", err)
	}
	commitMsg := fmt.Sprintf("Sync input_number config (%d items)", len(inputNumbers))
	if err := gitCommit(configRepo, commitMsg); err != nil {
		if !strings.Contains(err.Error(), "nothing to commit") {
			return "", fmt.Errorf("git commit: %w", err)
		}
	}

	return fmt.Sprintf("✓ 已同步 %d 个 input_number 到本地: %s\n✓ 已提交 git commit", len(inputNumbers), filePath), nil
}

// updateEntityIDInLocalFiles updates all local automation files that reference the old entity ID
func (s *Server) updateEntityIDInLocalFiles(configRepo, oldEntityID, newEntityID string) []string {
	if configRepo == "" {
		return nil
	}

	automationsDir := filepath.Join(configRepo, "automations")
	var updatedFiles []string

	// Walk through all subdirectories (group folders)
	filepath.Walk(automationsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".yaml") && !strings.HasSuffix(info.Name(), ".yml") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		content := string(data)
		if strings.Contains(content, oldEntityID) {
			newContent := strings.ReplaceAll(content, oldEntityID, newEntityID)
			if err := os.WriteFile(path, []byte(newContent), 0644); err == nil {
				relPath, _ := filepath.Rel(automationsDir, path)
				updatedFiles = append(updatedFiles, relPath)
			}
		}
		return nil
	})

	return updatedFiles
}

func (s *Server) confirmAutomation(filePath string) (string, error) {
	// Read pending file
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("read pending file: %w", err)
	}

	var automation map[string]any
	if err := yaml.Unmarshal(data, &automation); err != nil {
		return "", fmt.Errorf("parse yaml: %w", err)
	}

	// Deploy to HA
	if err := s.haClient.CreateAutomation(automation); err != nil {
		return "", fmt.Errorf("deploy to HA: %w", err)
	}

	// Auto-set friendly names for entities used in the automation
	s.autoSetFriendlyNames(automation)

	// Move from pending to automations/[group]
	configRepo := os.Getenv("HAC_CONFIG_REPO")
	if configRepo == "" {
		return "", fmt.Errorf("HAC_CONFIG_REPO not configured")
	}

	// Get alias and determine group
	alias, _ := automation["alias"].(string)
	if alias == "" {
		alias = strings.TrimSuffix(filepath.Base(filePath), ".yaml")
	}
	group := getAutomationGroup(alias)

	groupDir := filepath.Join(configRepo, "automations", group)
	if err := os.MkdirAll(groupDir, 0755); err != nil {
		return "", fmt.Errorf("create group dir: %w", err)
	}

	filename := filepath.Base(filePath)
	newPath := filepath.Join(groupDir, filename)
	if err := os.Rename(filePath, newPath); err != nil {
		return "", fmt.Errorf("move file: %w", err)
	}

	// Generate/update group README
	if err := s.generateGroupREADME(groupDir, group); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to generate README: %v\n", err)
	}

	// Git add and commit
	if err := gitAdd(configRepo, groupDir); err != nil {
		return "", fmt.Errorf("git add: %w", err)
	}
	commitMsg := fmt.Sprintf("Add automation: %s", alias)
	if err := gitCommit(configRepo, commitMsg); err != nil {
		return "", fmt.Errorf("git commit: %w", err)
	}

	return fmt.Sprintf("✓ Deployed automation '%s' to Home Assistant\n✓ Saved to %s\n✓ Committed to git", alias, newPath), nil
}

func (s *Server) listPending() (string, error) {
	configRepo := os.Getenv("HAC_CONFIG_REPO")
	if configRepo == "" {
		return "", fmt.Errorf("HAC_CONFIG_REPO not configured. Run 'hac init' first")
	}

	pendingDir := filepath.Join(configRepo, "pending")
	entries, err := os.ReadDir(pendingDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "No pending automations.", nil
		}
		return "", fmt.Errorf("read pending dir: %w", err)
	}

	var pending []string
	for _, e := range entries {
		if !e.IsDir() && (strings.HasSuffix(e.Name(), ".yaml") || strings.HasSuffix(e.Name(), ".yml")) {
			filePath := filepath.Join(pendingDir, e.Name())

			// Read and parse to get alias
			data, err := os.ReadFile(filePath)
			if err != nil {
				continue
			}
			var automation map[string]any
			yaml.Unmarshal(data, &automation)

			alias := e.Name()
			if a, ok := automation["alias"].(string); ok {
				alias = a
			}

			pending = append(pending, fmt.Sprintf("- %s\n  Path: %s", alias, filePath))
		}
	}

	if len(pending) == 0 {
		return "No pending automations.", nil
	}

	return fmt.Sprintf("📋 Pending automations (%d):\n\n%s\n\nSay \"确认 <path>\" to deploy, or \"取消 <path>\" to delete.", len(pending), strings.Join(pending, "\n\n")), nil
}

// autoSetFriendlyNames extracts light entities from automation and sets their friendly names
// based on the automation alias (room name)
func (s *Server) autoSetFriendlyNames(automation map[string]any) {
	// Get room name from alias (e.g., "客厅_有人_开灯" -> "客厅")
	alias, ok := automation["alias"].(string)
	if !ok || alias == "" {
		return
	}

	// Extract room name (before first underscore)
	parts := strings.Split(alias, "_")
	if len(parts) == 0 {
		return
	}
	roomName := parts[0]

	// Find light entities in action
	actions, ok := automation["action"].([]any)
	if !ok {
		return
	}

	ws, err := s.haClient.NewWSClient()
	if err != nil {
		return
	}
	defer ws.Close()

	for _, action := range actions {
		actionMap, ok := action.(map[string]any)
		if !ok {
			continue
		}

		// Check if it's a light service call
		service, _ := actionMap["service"].(string)
		if !strings.HasPrefix(service, "light.") {
			continue
		}

		// Get target entity_id
		target, ok := actionMap["target"].(map[string]any)
		if !ok {
			continue
		}

		entityID, ok := target["entity_id"].(string)
		if !ok {
			continue
		}

		// Only process light groups with long auto-generated names
		if !strings.HasPrefix(entityID, "light.mijia_cn_group_") {
			continue
		}

		// Set friendly name based on room name
		friendlyName := roomName + "灯组"
		ws.SetEntityName(entityID, friendlyName)
	}
}

func (s *Server) cancelPending(filePath string) error {
	// Verify it's in pending directory
	configRepo := os.Getenv("HAC_CONFIG_REPO")
	if configRepo == "" {
		return fmt.Errorf("HAC_CONFIG_REPO not configured")
	}

	pendingDir := filepath.Join(configRepo, "pending")
	if !strings.HasPrefix(filePath, pendingDir) {
		return fmt.Errorf("file is not in pending directory")
	}

	if err := os.Remove(filePath); err != nil {
		return fmt.Errorf("remove file: %w", err)
	}

	return nil
}

func (s *Server) createTemplateSensor(name, uniqueID, stateTemplate, unit, deviceClass string) (string, error) {
	// Use WebSocket API to create persistent template sensor
	ws, err := s.haClient.NewWSClient()
	if err != nil {
		return "", fmt.Errorf("connect to HA: %w", err)
	}
	defer ws.Close()

	entityID, err := ws.CreateTemplateSensor(name, stateTemplate, unit, deviceClass, "")
	if err != nil {
		return "", fmt.Errorf("create template sensor: %w", err)
	}

	// Render the template to show current value
	value, _ := s.haClient.RenderTemplate(stateTemplate)

	// Save template config to file for reference
	configRepo := os.Getenv("HAC_CONFIG_REPO")
	if configRepo != "" {
		templateConfig := map[string]any{
			"name":           name,
			"unique_id":      uniqueID,
			"entity_id":      entityID,
			"state_template": stateTemplate,
			"unit":           unit,
			"device_class":   deviceClass,
		}
		s.saveTemplateSensorConfig(configRepo, uniqueID, templateConfig)
	}

	return fmt.Sprintf("✓ 成功创建持久化模板传感器: %s\n  Entity ID: %s\n  当前值: %s %s", name, entityID, strings.TrimSpace(value), unit), nil
}

func (s *Server) updateTemplateSensor(entityID, stateTemplate string) (string, error) {
	// Get current state to preserve attributes
	currentState, err := s.haClient.GetState(entityID)
	if err != nil {
		return "", fmt.Errorf("get current state: %w", err)
	}

	// Render the new value
	value, err := s.haClient.RenderTemplate(stateTemplate)
	if err != nil {
		return "", fmt.Errorf("render template: %w", err)
	}

	// Update state_template in attributes
	attributes := currentState.Attributes
	if attributes == nil {
		attributes = make(map[string]any)
	}
	attributes["state_template"] = stateTemplate

	// Set the new state
	if err := s.haClient.SetState(entityID, strings.TrimSpace(value), attributes); err != nil {
		return "", fmt.Errorf("set state: %w", err)
	}

	return fmt.Sprintf("✓ Updated %s\n  New value: %s", entityID, strings.TrimSpace(value)), nil
}

func (s *Server) saveTemplateSensorConfig(repoPath, uniqueID string, config map[string]any) error {
	sensorsDir := filepath.Join(repoPath, "sensors")
	if err := os.MkdirAll(sensorsDir, 0755); err != nil {
		return err
	}

	filePath := filepath.Join(sensorsDir, uniqueID+".yaml")
	data, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, data, 0644)
}

func gitAdd(repoPath, filePath string) error {
	cmd := exec.Command("git", "add", filePath)
	cmd.Dir = repoPath
	return cmd.Run()
}

func gitCommit(repoPath, message string) error {
	cmd := exec.Command("git", "commit", "-m", message)
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if nothing to commit
		if strings.Contains(string(output), "nothing to commit") {
			return nil
		}
		return fmt.Errorf("%s: %w", string(output), err)
	}
	return nil
}

// migrateAutomations migrates existing automation files to group directories
func (s *Server) migrateAutomations() (string, error) {
	configRepo := os.Getenv("HAC_CONFIG_REPO")
	if configRepo == "" {
		return "", fmt.Errorf("HAC_CONFIG_REPO not configured")
	}

	automationsDir := filepath.Join(configRepo, "automations")
	entries, err := os.ReadDir(automationsDir)
	if err != nil {
		return "", fmt.Errorf("read automations dir: %w", err)
	}

	// Track migrations by group
	migrations := make(map[string][]string)
	var errors []string

	for _, entry := range entries {
		// Skip directories and non-yaml files
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".yaml") && !strings.HasSuffix(entry.Name(), ".yml") {
			continue
		}

		// Read and parse the automation file
		oldPath := filepath.Join(automationsDir, entry.Name())
		data, err := os.ReadFile(oldPath)
		if err != nil {
			errors = append(errors, fmt.Sprintf("读取 %s 失败: %v", entry.Name(), err))
			continue
		}

		var config map[string]any
		if err := yaml.Unmarshal(data, &config); err != nil {
			errors = append(errors, fmt.Sprintf("解析 %s 失败: %v", entry.Name(), err))
			continue
		}

		// Get alias and determine group
		alias, _ := config["alias"].(string)
		if alias == "" {
			alias = strings.TrimSuffix(entry.Name(), ".yaml")
		}
		group := getAutomationGroup(alias)

		// Create group directory
		groupDir := filepath.Join(automationsDir, group)
		if err := os.MkdirAll(groupDir, 0755); err != nil {
			errors = append(errors, fmt.Sprintf("创建目录 %s 失败: %v", group, err))
			continue
		}

		// Move file to group directory
		newPath := filepath.Join(groupDir, entry.Name())
		if err := os.Rename(oldPath, newPath); err != nil {
			errors = append(errors, fmt.Sprintf("移动 %s 失败: %v", entry.Name(), err))
			continue
		}

		migrations[group] = append(migrations[group], alias)
	}

	// Generate README for each group
	for group := range migrations {
		groupDir := filepath.Join(automationsDir, group)
		if err := s.generateGroupREADME(groupDir, group); err != nil {
			errors = append(errors, fmt.Sprintf("生成 %s/README.md 失败: %v", group, err))
		}
	}

	// Git add and commit
	if err := gitAdd(configRepo, automationsDir); err != nil {
		return "", fmt.Errorf("git add: %w", err)
	}
	if err := gitCommit(configRepo, "Migrate automations to group directories"); err != nil {
		if !strings.Contains(err.Error(), "nothing to commit") {
			return "", fmt.Errorf("git commit: %w", err)
		}
	}

	// Build result message
	var sb strings.Builder
	sb.WriteString("✓ 自动化迁移完成\n\n")

	totalMigrated := 0
	for group, files := range migrations {
		sb.WriteString(fmt.Sprintf("## %s (%d 个)\n", group, len(files)))
		for _, f := range files {
			sb.WriteString(fmt.Sprintf("  - %s\n", f))
		}
		sb.WriteString("\n")
		totalMigrated += len(files)
	}

	sb.WriteString(fmt.Sprintf("共迁移 %d 个自动化到 %d 个分组\n", totalMigrated, len(migrations)))

	if len(errors) > 0 {
		sb.WriteString(fmt.Sprintf("\n⚠️ %d 个错误:\n", len(errors)))
		for _, e := range errors {
			sb.WriteString(fmt.Sprintf("  - %s\n", e))
		}
	}

	return sb.String(), nil
}

// getAutomationGroup determines the group/category for an automation based on its alias
func getAutomationGroup(alias string) string {
	// Define group patterns
	patterns := map[string][]string{
		"人来灯亮":    {"_有人_开灯", "_有人移动_开灯"},
		"人走灯灭":    {"_无人_关灯", "_无人5分钟_关灯"},
		"热水器":     {"热水器"},
		"马桶换气":    {"_坐马桶_开换气", "_无人_关换气"},
		"睡眠模式":    {"_睡眠模式_打开", "_睡眠模式_关闭", "_关闭睡眠模式", "_睡眠模式打开", "_关闭窗帘_睡眠模式"},
		"光暗灯亮":    {"_光暗_开灯"},
		"衣柜灯":     {"_衣柜开门_开灯", "_衣柜关门_关灯", "_衣柜超时未关_提醒"},
		"洗澡模式":    {"_洗澡模式_", "_浴霸", "_进入洗澡模式", "_退出洗澡模式"},
		"全屋模式":    {"全屋_观影模式_", "全屋_会客模式_", "全屋_开灯模式", "全屋_关灯模式", "全屋_音量调节_"},
		"iPad自动化": {"_iPad"},
	}

	for group, suffixes := range patterns {
		for _, suffix := range suffixes {
			if strings.Contains(alias, suffix) {
				return group
			}
		}
	}

	// Check for room-based grouping as fallback
	rooms := []string{"主卧", "主卫", "父母房", "儿童房", "老人房", "北卧", "客厅", "餐厅", "厨房", "洗衣房", "衣帽间", "客卫"}
	for _, room := range rooms {
		if strings.HasPrefix(alias, room) {
			return room
		}
	}

	return "其他"
}

// generateGroupREADME generates a README.md file for a group directory
func (s *Server) generateGroupREADME(groupDir, groupName string) error {
	// Read all automation files in the group
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
		description, _ := config["description"].(string)
		mode, _ := config["mode"].(string)

		// Extract trigger info
		triggerInfo := extractTriggerInfo(config)
		// Extract action info
		actionInfo := extractActionInfo(config)

		automations = append(automations, map[string]string{
			"name":        alias,
			"description": description,
			"mode":        mode,
			"trigger":     triggerInfo,
			"action":      actionInfo,
			"file":        e.Name(),
		})
	}

	if len(automations) == 0 {
		return nil
	}

	// Generate README content
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# %s\n\n", groupName))
	sb.WriteString(fmt.Sprintf("本目录包含 %d 个自动化配置。\n\n", len(automations)))

	// Generate table
	sb.WriteString("## 自动化列表\n\n")
	sb.WriteString("| 名称 | 触发条件 | 动作 | 模式 |\n")
	sb.WriteString("|------|----------|------|------|\n")

	for _, a := range automations {
		name := a["name"]
		trigger := a["trigger"]
		action := a["action"]
		mode := a["mode"]
		if mode == "" {
			mode = "single"
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n", name, trigger, action, mode))
	}

	sb.WriteString("\n## 详细说明\n\n")
	for _, a := range automations {
		sb.WriteString(fmt.Sprintf("### %s\n\n", a["name"]))
		if a["description"] != "" {
			sb.WriteString(fmt.Sprintf("%s\n\n", a["description"]))
		}
		sb.WriteString(fmt.Sprintf("- **文件**: `%s`\n", a["file"]))
		sb.WriteString(fmt.Sprintf("- **触发**: %s\n", a["trigger"]))
		sb.WriteString(fmt.Sprintf("- **动作**: %s\n", a["action"]))
		sb.WriteString(fmt.Sprintf("- **模式**: %s\n\n", a["mode"]))
	}

	readmePath := filepath.Join(groupDir, "README.md")
	return os.WriteFile(readmePath, []byte(sb.String()), 0644)
}

// extractTriggerInfo extracts human-readable trigger information from automation config
func extractTriggerInfo(config map[string]any) string {
	// Try both "trigger" and "triggers" keys
	triggers, ok := config["trigger"].([]any)
	if !ok || len(triggers) == 0 {
		triggers, ok = config["triggers"].([]any)
		if !ok || len(triggers) == 0 {
			return "未知"
		}
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
			entityID, _ := trigger["entity_id"].(string)
			to, _ := trigger["to"].(string)
			// Extract friendly name from entity_id
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
			}
		case "time":
			at, _ := trigger["at"].(string)
			parts = append(parts, fmt.Sprintf("时间 %s", at))
		default:
			parts = append(parts, platform)
		}
	}

	if len(parts) == 0 {
		return "未知"
	}
	return strings.Join(parts, ", ")
}

// extractActionInfo extracts human-readable action information from automation config
func extractActionInfo(config map[string]any) string {
	// Try both "action" and "actions" keys
	actions, ok := config["action"].([]any)
	if !ok || len(actions) == 0 {
		actions, ok = config["actions"].([]any)
		if !ok || len(actions) == 0 {
			return "未知"
		}
	}

	var parts []string
	for _, a := range actions {
		action, ok := a.(map[string]any)
		if !ok {
			continue
		}

		// Try both "service" and "action" keys
		service, _ := action["service"].(string)
		if service == "" {
			service, _ = action["action"].(string)
		}
		target, _ := action["target"].(map[string]any)

		var entityNames []string
		if target != nil {
			if entityID, ok := target["entity_id"].(string); ok {
				entityNames = append(entityNames, extractEntityName(entityID))
			} else if entityIDs, ok := target["entity_id"].([]any); ok {
				for _, id := range entityIDs {
					if idStr, ok := id.(string); ok {
						entityNames = append(entityNames, extractEntityName(idStr))
					}
				}
			}
		}

		switch service {
		case "light.turn_on":
			if len(entityNames) > 3 {
				parts = append(parts, fmt.Sprintf("开灯 (%d个)", len(entityNames)))
			} else if len(entityNames) > 0 {
				parts = append(parts, fmt.Sprintf("开灯: %s", strings.Join(entityNames, ", ")))
			} else {
				parts = append(parts, "开灯")
			}
		case "light.turn_off":
			if len(entityNames) > 3 {
				parts = append(parts, fmt.Sprintf("关灯 (%d个)", len(entityNames)))
			} else if len(entityNames) > 0 {
				parts = append(parts, fmt.Sprintf("关灯: %s", strings.Join(entityNames, ", ")))
			} else {
				parts = append(parts, "关灯")
			}
		case "switch.turn_on":
			parts = append(parts, "开启开关")
		case "switch.turn_off":
			parts = append(parts, "关闭开关")
		default:
			parts = append(parts, service)
		}
	}

	if len(parts) == 0 {
		return "未知"
	}
	return strings.Join(parts, ", ")
}

// extractEntityName extracts a friendly name from entity_id
func extractEntityName(entityID string) string {
	// Remove domain prefix
	parts := strings.SplitN(entityID, ".", 2)
	if len(parts) != 2 {
		return entityID
	}

	name := parts[1]
	// Try to extract Chinese name from common patterns
	// e.g., "zhuwo_shedeng_dengzu" -> "主卧射灯灯组"
	// For now, just return the suffix part cleaned up
	name = strings.ReplaceAll(name, "_", " ")

	// Truncate long names
	if len(name) > 20 {
		name = name[:20] + "..."
	}

	return name
}

// illuminationSensorConfig defines the mapping between rooms and their illumination sensors
type illuminationSensorConfig struct {
	Room     string
	SensorID string
}

// generateIlluminationReport generates a full illumination report for all rooms
func (s *Server) generateIlluminationReport() (string, error) {
	// Global illumination sensor (领普ES5)
	globalSensorID := "sensor.linp_cn_blt_3_1nrd16kq8cg00_es5b_illumination_p_2_1005"

	// Room illumination sensors mapping
	roomSensors := []illuminationSensorConfig{
		{"客厅", "sensor.izq_cn_1205048022_24n_illumination_p_2_2"},
		{"餐厅", "sensor.izq_cn_1189446445_24n_illumination_p_2_2"},
		{"主卧", "sensor.izq_cn_1205048835_24n_illumination_p_2_2"},
		{"北卧", "sensor.izq_cn_1189446822_24n_illumination_p_2_2"},
		{"儿童房", "sensor.izq_cn_1205048242_24n_illumination_p_2_2"},
		{"父母房", "sensor.izq_cn_1189446888_24n_illumination_p_2_2"},
		{"老人房", "sensor.izq_cn_1205048236_24n_illumination_p_2_2"},
		{"厨房", "sensor.izq_cn_1205048246_24n_illumination_p_2_2"},
		{"主卧门口过道", "sensor.izq_cn_1189445770_24n_illumination_p_2_2"},
		{"客卫门口过道", "sensor.izq_cn_1189446350_24n_illumination_p_2_2"},
		{"洗衣房", "sensor.izq_cn_1205048024_24n_illumination_p_2_2"},
		{"衣帽间", "sensor.izq_cn_1205048226_24n_illumination_p_2_2"},
		{"客厅阳台过道", "sensor.izq_cn_1205048235_24n_illumination_p_2_2"},
		{"父母房卫生间", "sensor.izq_cn_1189445850_24n_illumination_p_2_2"},
		{"父母房过道", "sensor.linp_cn_blt_3_1nrd8nqto4002_es5b_illumination_p_2_1005"},
		{"客卫", "sensor.izq_cn_1138358875_24n_illumination_p_2_2"},
	}

	// Get global illumination
	globalState, err := s.haClient.GetState(globalSensorID)
	if err != nil {
		return "", fmt.Errorf("获取全局光照失败: %w", err)
	}
	globalIllum := parseFloat(globalState.State)

	// Read thresholds from automation files
	configRepo := os.Getenv("HAC_CONFIG_REPO")
	thresholds := make(map[string]float64)
	if configRepo != "" {
		automationDir := filepath.Join(configRepo, "automations", "人来灯亮")
		entries, _ := os.ReadDir(automationDir)
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
				continue
			}
			filePath := filepath.Join(automationDir, e.Name())
			data, err := os.ReadFile(filePath)
			if err != nil {
				continue
			}
			var config map[string]any
			if err := yaml.Unmarshal(data, &config); err != nil {
				continue
			}
			alias, _ := config["alias"].(string)
			// Extract room name from alias (e.g., "客厅_有人_开灯" -> "客厅")
			roomName := strings.Split(alias, "_")[0]

			// Find threshold in conditions (only for illumination sensor)
			if conditions, ok := config["conditions"].([]any); ok {
				for _, c := range conditions {
					if cond, ok := c.(map[string]any); ok {
						if cond["condition"] == "numeric_state" {
							// Only get threshold for illumination sensor
							entityID, _ := cond["entity_id"].(string)
							if strings.Contains(entityID, "illumination") || entityID == globalSensorID {
								if below, ok := cond["below"].(int); ok {
									thresholds[roomName] = float64(below)
								} else if below, ok := cond["below"].(float64); ok {
									thresholds[roomName] = below
								}
							}
						}
					}
				}
			}
		}
	}

	// Build report
	var sb strings.Builder
	sb.WriteString("## 全屋光照报告\n\n")
	sb.WriteString(fmt.Sprintf("**全局光照（领普ES5）：%.0f lx**\n\n", globalIllum))

	sb.WriteString("| 区域 | 本地光照 (lx) | 阈值 (lx) | 全局 vs 阈值 | 是否应开灯 |\n")
	sb.WriteString("|------|-------------|-----------|-------------|------------|\n")

	type roomData struct {
		Room       string
		LocalIllum float64
		Threshold  float64
		ShouldOn   bool
		Ratio      float64
	}
	var roomDataList []roomData

	for _, rs := range roomSensors {
		state, err := s.haClient.GetState(rs.SensorID)
		localIllum := float64(0)
		if err == nil {
			localIllum = parseFloat(state.State)
		}

		threshold := thresholds[rs.Room]
		shouldOn := globalIllum < threshold
		ratio := float64(0)
		if globalIllum > 0 {
			ratio = localIllum / globalIllum
		}

		roomDataList = append(roomDataList, roomData{
			Room:       rs.Room,
			LocalIllum: localIllum,
			Threshold:  threshold,
			ShouldOn:   shouldOn,
			Ratio:      ratio,
		})

		shouldOnStr := "❌ 不开灯"
		if shouldOn {
			shouldOnStr = "✅ **应开灯**"
		}
		comparison := fmt.Sprintf("%.0f > %.0f", globalIllum, threshold)
		if shouldOn {
			comparison = fmt.Sprintf("%.0f < %.0f", globalIllum, threshold)
		}

		sb.WriteString(fmt.Sprintf("| **%s** | %.0f | %.0f | %s | %s |\n",
			rs.Room, localIllum, threshold, comparison, shouldOnStr))
	}

	// Summary section
	sb.WriteString("\n---\n\n### 当前会开灯的房间\n")
	hasRoomToLight := false
	for _, rd := range roomDataList {
		if rd.ShouldOn {
			sb.WriteString(fmt.Sprintf("- **%s**（本地 %.0f lx，阈值 %.0f）\n", rd.Room, rd.LocalIllum, rd.Threshold))
			hasRoomToLight = true
		}
	}
	if !hasRoomToLight {
		sb.WriteString("无\n")
	}

	// Ratio analysis
	sb.WriteString("\n### 系数分析（本地光照 / 全局光照）\n")
	sb.WriteString("| 区域 | 系数 | 说明 |\n")
	sb.WriteString("|------|------|------|\n")

	// Sort by ratio descending
	for i := 0; i < len(roomDataList)-1; i++ {
		for j := i + 1; j < len(roomDataList); j++ {
			if roomDataList[j].Ratio > roomDataList[i].Ratio {
				roomDataList[i], roomDataList[j] = roomDataList[j], roomDataList[i]
			}
		}
	}

	for _, rd := range roomDataList {
		desc := "采光中等"
		if rd.Ratio > 1.0 {
			desc = "采光好"
		} else if rd.Ratio > 0.5 {
			desc = "采光中等"
		} else if rd.Ratio > 0.2 {
			desc = "采光较差"
		} else {
			desc = "采光很差"
		}
		sb.WriteString(fmt.Sprintf("| %s | %.2f | %s |\n", rd.Room, rd.Ratio, desc))
	}

	return sb.String(), nil
}

// parseFloat parses a string to float64, returns 0 on error
func parseFloat(s string) float64 {
	var f float64
	fmt.Sscanf(s, "%f", &f)
	return f
}
