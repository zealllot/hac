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
	return []Tool{
		{
			Name:        "get_devices",
			Description: "获取 Home Assistant 中所有设备及其能力。返回每个设备的 entity_id、名称、当前状态、支持的服务和属性。注意：设备数量较多时可能被截断，建议使用 search_devices 按关键词搜索。",
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
				filePath := filepath.Join(configRepo, "automations", alias+".yaml")
				if err := os.Remove(filePath); err == nil {
					gitAdd(configRepo, filePath)
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
	return fmt.Sprintf("📝 Automation draft saved to:\n%s\n\n```yaml\n%s```\n\n⚠️ Please review the configuration above.\nSay \"确认\" or \"confirm\" to deploy to Home Assistant.", filePath, string(yamlData)), nil
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

	// Write to automations directory
	automationsDir := filepath.Join(configRepo, "automations")
	if err := os.MkdirAll(automationsDir, 0755); err != nil {
		return "", fmt.Errorf("create automations dir: %w", err)
	}

	filename := alias + ".yaml"
	filePath := filepath.Join(automationsDir, filename)

	// Marshal to YAML
	data, err := yaml.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("marshal yaml: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	// Git add and commit
	if err := gitAdd(configRepo, filePath); err != nil {
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
	entries, err := os.ReadDir(automationsDir)
	if err != nil {
		return nil
	}

	var updatedFiles []string
	for _, entry := range entries {
		if entry.IsDir() || (!strings.HasSuffix(entry.Name(), ".yaml") && !strings.HasSuffix(entry.Name(), ".yml")) {
			continue
		}

		filePath := filepath.Join(automationsDir, entry.Name())
		data, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}

		content := string(data)
		if strings.Contains(content, oldEntityID) {
			newContent := strings.ReplaceAll(content, oldEntityID, newEntityID)
			if err := os.WriteFile(filePath, []byte(newContent), 0644); err == nil {
				updatedFiles = append(updatedFiles, entry.Name())
			}
		}
	}

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

	// Move from pending to automations
	configRepo := os.Getenv("HAC_CONFIG_REPO")
	if configRepo == "" {
		return "", fmt.Errorf("HAC_CONFIG_REPO not configured")
	}

	automationsDir := filepath.Join(configRepo, "automations")
	if err := os.MkdirAll(automationsDir, 0755); err != nil {
		return "", fmt.Errorf("create automations dir: %w", err)
	}

	filename := filepath.Base(filePath)
	newPath := filepath.Join(automationsDir, filename)
	if err := os.Rename(filePath, newPath); err != nil {
		return "", fmt.Errorf("move file: %w", err)
	}

	// Git add and commit
	name := filename
	if alias, ok := automation["alias"].(string); ok {
		name = alias
	}

	if err := gitAdd(configRepo, newPath); err != nil {
		return "", fmt.Errorf("git add: %w", err)
	}
	commitMsg := fmt.Sprintf("Add automation: %s", name)
	if err := gitCommit(configRepo, commitMsg); err != nil {
		return "", fmt.Errorf("git commit: %w", err)
	}

	return fmt.Sprintf("✓ Deployed automation '%s' to Home Assistant\n✓ Saved to %s\n✓ Committed to git", name, newPath), nil
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
	// First, render the template to get the initial value
	value, err := s.haClient.RenderTemplate(stateTemplate)
	if err != nil {
		return "", fmt.Errorf("render template: %w", err)
	}

	// Create entity_id from unique_id
	entityID := "sensor." + uniqueID

	// Build attributes
	attributes := map[string]any{
		"friendly_name":  name,
		"state_template": stateTemplate,
	}
	if unit != "" {
		attributes["unit_of_measurement"] = unit
	}
	if deviceClass != "" {
		attributes["device_class"] = deviceClass
	}

	// Set the state via HA API
	if err := s.haClient.SetState(entityID, strings.TrimSpace(value), attributes); err != nil {
		return "", fmt.Errorf("set state: %w", err)
	}

	// Save template config to file for persistence reference
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

	return fmt.Sprintf("✓ Created template sensor: %s\n  Entity ID: %s\n  Current value: %s %s\n\n⚠️ Note: This sensor is created via API and will be lost after HA restart.\nTo make it persistent, add the following to your configuration.yaml:\n\n```yaml\ntemplate:\n  - sensor:\n      - name: \"%s\"\n        unique_id: %s\n        unit_of_measurement: \"%s\"\n        device_class: %s\n        state: >-\n          %s\n```", name, entityID, strings.TrimSpace(value), unit, name, uniqueID, unit, deviceClass, stateTemplate), nil
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
