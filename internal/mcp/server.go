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
			Description: `按关键词搜索设备。支持搜索 entity_id 和设备名称。

示例：
- 搜索 "衣帽间" 会返回所有名称包含"衣帽间"的设备
- 搜索 "light" 会返回所有灯类设备
- 搜索 "客厅 灯" 会返回客厅的灯

推荐在创建自动化前使用此工具确认设备的 entity_id。`,
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

## Automation IR 格式
{
  "name": "客厅_晚间有人_开灯",
  "trigger": {"type": "state", "entity": "binary_sensor.motion_living_room", "to": "on"},
  "conditions": [{"type": "time", "after": "19:00", "before": "23:00"}],
  "actions": [{"action": "call_service", "service": "light.turn_on", "target": "light.living_room", "data": {"brightness_pct": 80}}],
  "constraints": {"mode": "restart"}
}

支持的 trigger.type: state, time
支持的 condition.type: state, time
推荐 mode: restart（人体感应）、single（定时任务）、queued（按钮触发）`,
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
				// 搜索 entity_id、名称和区域
				searchText := strings.ToLower(d.EntityID + " " + d.Name + " " + d.Area)
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
				data, _ := json.MarshalIndent(matched, "", "  ")
				result = fmt.Sprintf("找到 %d 个匹配的设备:\n%s", len(matched), string(data))
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
		if err := s.haClient.DeleteAutomation(automationID); err != nil {
			result = fmt.Sprintf("Error: %v", err)
			isError = true
		} else {
			result = fmt.Sprintf("Successfully deleted automation: %s", automationID)
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
