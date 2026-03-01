package ha

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) doRequest(method, path string, body any) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, c.baseURL+path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

func (c *Client) GetStates() ([]EntityState, error) {
	data, err := c.doRequest("GET", "/api/states", nil)
	if err != nil {
		return nil, err
	}

	var states []EntityState
	if err := json.Unmarshal(data, &states); err != nil {
		return nil, fmt.Errorf("unmarshal states: %w", err)
	}

	return states, nil
}

func (c *Client) GetState(entityID string) (*EntityState, error) {
	data, err := c.doRequest("GET", "/api/states/"+entityID, nil)
	if err != nil {
		return nil, err
	}

	var state EntityState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("unmarshal state: %w", err)
	}

	return &state, nil
}

func (c *Client) GetServices() ([]ServiceDomain, error) {
	data, err := c.doRequest("GET", "/api/services", nil)
	if err != nil {
		return nil, err
	}

	var services []ServiceDomain
	if err := json.Unmarshal(data, &services); err != nil {
		return nil, fmt.Errorf("unmarshal services: %w", err)
	}

	return services, nil
}

func (c *Client) CallService(domain, service string, data map[string]any) error {
	_, err := c.doRequest("POST", fmt.Sprintf("/api/services/%s/%s", domain, service), data)
	return err
}

func (c *Client) GetConfig() (*Config, error) {
	data, err := c.doRequest("GET", "/api/config", nil)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	return &config, nil
}

func (c *Client) RenderTemplate(template string) (string, error) {
	body := map[string]string{"template": template}
	data, err := c.doRequest("POST", "/api/template", body)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (c *Client) GetAreaRegistry() (map[string]string, error) {
	// 使用模板获取所有区域
	template := `{% for area in areas() %}{{ area }}|{{ area_name(area) }}
{% endfor %}`
	result, err := c.RenderTemplate(template)
	if err != nil {
		return nil, err
	}

	areas := make(map[string]string)
	for _, line := range strings.Split(result, "\n") {
		parts := strings.SplitN(line, "|", 2)
		if len(parts) == 2 {
			areas[parts[0]] = parts[1]
		}
	}
	return areas, nil
}

func (c *Client) GetEntityArea(entityID string) (string, error) {
	template := fmt.Sprintf(`{{ area_name(area_id('%s')) }}`, entityID)
	result, err := c.RenderTemplate(template)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(result), nil
}

func (c *Client) GetAutomations() ([]EntityState, error) {
	states, err := c.GetStates()
	if err != nil {
		return nil, err
	}

	var automations []EntityState
	for _, s := range states {
		if strings.HasPrefix(s.EntityID, "automation.") {
			automations = append(automations, s)
		}
	}

	return automations, nil
}

func (c *Client) CreateAutomation(automation map[string]any) error {
	// Generate a unique ID if not provided
	id, ok := automation["id"].(string)
	if !ok || id == "" {
		id = fmt.Sprintf("%d", time.Now().UnixNano())
		automation["id"] = id
	}
	_, err := c.doRequest("POST", "/api/config/automation/config/"+id, automation)
	return err
}

// GetAutomationConfig gets a single automation's configuration by ID
func (c *Client) GetAutomationConfig(id string) (map[string]any, error) {
	data, err := c.doRequest("GET", "/api/config/automation/config/"+id, nil)
	if err != nil {
		return nil, err
	}

	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("unmarshal automation config: %w", err)
	}

	return config, nil
}

func (c *Client) UpdateAutomation(id string, automation map[string]any) error {
	// HA uses POST for both create and update
	_, err := c.doRequest("POST", "/api/config/automation/config/"+id, automation)
	return err
}

func (c *Client) DeleteAutomation(id string) error {
	_, err := c.doRequest("DELETE", "/api/config/automation/config/"+id, nil)
	return err
}

// CreateTemplateSensor creates a template sensor via the HA config API
// Note: This requires the template integration to be set up in configuration.yaml
// or using the UI-based template helpers
func (c *Client) CreateInputNumber(config map[string]any) error {
	_, err := c.doRequest("POST", "/api/config/input_number", config)
	return err
}

// SetState sets the state of an entity (useful for template sensors or input helpers)
func (c *Client) SetState(entityID string, state string, attributes map[string]any) error {
	body := map[string]any{
		"state": state,
	}
	if attributes != nil {
		body["attributes"] = attributes
	}
	_, err := c.doRequest("POST", "/api/states/"+entityID, body)
	return err
}

func (c *Client) GetDevices() (map[string]DeviceCapability, error) {
	states, err := c.GetStates()
	if err != nil {
		return nil, err
	}

	services, err := c.GetServices()
	if err != nil {
		return nil, err
	}

	// 批量获取所有实体的区域信息
	entityAreas := c.getEntityAreas(states)

	serviceMap := make(map[string][]string)
	for _, svc := range services {
		var names []string
		for name := range svc.Services {
			names = append(names, name)
		}
		serviceMap[svc.Domain] = names
	}

	devices := make(map[string]DeviceCapability)
	for _, s := range states {
		parts := strings.SplitN(s.EntityID, ".", 2)
		if len(parts) != 2 {
			continue
		}
		domain := parts[0]

		friendlyName := ""
		if name, ok := s.Attributes["friendly_name"].(string); ok {
			friendlyName = name
		}

		var attrs []string
		for k := range s.Attributes {
			attrs = append(attrs, k)
		}

		devices[s.EntityID] = DeviceCapability{
			EntityID:   s.EntityID,
			Domain:     domain,
			Name:       friendlyName,
			Area:       entityAreas[s.EntityID],
			State:      s.State,
			Supports:   serviceMap[domain],
			Attributes: attrs,
		}
	}

	return devices, nil
}

func (c *Client) getEntityAreas(states []EntityState) map[string]string {
	areas := make(map[string]string)

	// 分批处理，每批 50 个实体
	batchSize := 50
	for i := 0; i < len(states); i += batchSize {
		end := i + batchSize
		if end > len(states) {
			end = len(states)
		}
		batch := states[i:end]

		var sb strings.Builder
		for j, s := range batch {
			if j > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(fmt.Sprintf("{{ '%s' }}|{{ area_name(area_id('%s')) | default('') }}", s.EntityID, s.EntityID))
		}

		result, err := c.RenderTemplate(sb.String())
		if err != nil {
			continue
		}

		for _, line := range strings.Split(result, "\n") {
			parts := strings.SplitN(line, "|", 2)
			if len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
				areas[parts[0]] = strings.TrimSpace(parts[1])
			}
		}
	}
	return areas
}

// ReloadIntegration reloads a specific integration's configuration
// Common domains: homeassistant, automation, script, scene, template, input_boolean, etc.
func (c *Client) ReloadIntegration(domain string) error {
	_, err := c.doRequest("POST", fmt.Sprintf("/api/services/%s/reload", domain), map[string]any{})
	if err != nil {
		return fmt.Errorf("reload %s: %w", domain, err)
	}
	return nil
}

// ReloadConfigEntry reloads a specific config entry by its entry_id
func (c *Client) ReloadConfigEntry(entryID string) error {
	_, err := c.doRequest("POST", "/api/services/homeassistant/reload_config_entry", map[string]any{
		"entry_id": entryID,
	})
	if err != nil {
		return fmt.Errorf("reload config entry %s: %w", entryID, err)
	}
	return nil
}

// ReloadAll reloads all reloadable integrations
func (c *Client) ReloadAll() error {
	_, err := c.doRequest("POST", "/api/services/homeassistant/reload_all", map[string]any{})
	if err != nil {
		return fmt.Errorf("reload all: %w", err)
	}
	return nil
}

// WebSocket client for category management
type WSClient struct {
	conn  *websocket.Conn
	token string
	msgID int
	mu    sync.Mutex
}

func (c *Client) NewWSClient() (*WSClient, error) {
	// Convert HTTP URL to WebSocket URL
	wsURL := strings.Replace(c.baseURL, "http://", "ws://", 1)
	wsURL = strings.Replace(wsURL, "https://", "wss://", 1)
	wsURL += "/api/websocket"

	u, err := url.Parse(wsURL)
	if err != nil {
		return nil, fmt.Errorf("parse websocket url: %w", err)
	}

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("websocket dial: %w", err)
	}

	ws := &WSClient{
		conn:  conn,
		token: c.token,
		msgID: 1,
	}

	// Read auth_required message
	var authReq map[string]any
	if err := conn.ReadJSON(&authReq); err != nil {
		conn.Close()
		return nil, fmt.Errorf("read auth_required: %w", err)
	}

	// Send auth message
	authMsg := map[string]any{
		"type":         "auth",
		"access_token": c.token,
	}
	if err := conn.WriteJSON(authMsg); err != nil {
		conn.Close()
		return nil, fmt.Errorf("send auth: %w", err)
	}

	// Read auth result
	var authResult map[string]any
	if err := conn.ReadJSON(&authResult); err != nil {
		conn.Close()
		return nil, fmt.Errorf("read auth result: %w", err)
	}

	if authResult["type"] != "auth_ok" {
		conn.Close()
		return nil, fmt.Errorf("auth failed: %v", authResult)
	}

	return ws, nil
}

func (ws *WSClient) Close() error {
	return ws.conn.Close()
}

func (ws *WSClient) sendCommand(msgType string, data map[string]any) (map[string]any, error) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	msg := map[string]any{
		"id":   ws.msgID,
		"type": msgType,
	}
	for k, v := range data {
		msg[k] = v
	}
	ws.msgID++

	if err := ws.conn.WriteJSON(msg); err != nil {
		return nil, fmt.Errorf("send command: %w", err)
	}

	var result map[string]any
	if err := ws.conn.ReadJSON(&result); err != nil {
		return nil, fmt.Errorf("read result: %w", err)
	}

	if success, ok := result["success"].(bool); ok && !success {
		return nil, fmt.Errorf("command failed: %v", result["error"])
	}

	return result, nil
}

// Category represents an automation category
type Category struct {
	CategoryID string `json:"category_id"`
	Name       string `json:"name"`
	Icon       string `json:"icon,omitempty"`
}

// ListCategories lists all automation categories
func (ws *WSClient) ListCategories(scope string) ([]Category, error) {
	result, err := ws.sendCommand("config/category_registry/list", map[string]any{
		"scope": scope,
	})
	if err != nil {
		return nil, err
	}

	categoriesRaw, ok := result["result"].([]any)
	if !ok {
		return nil, fmt.Errorf("unexpected result type")
	}

	var categories []Category
	for _, c := range categoriesRaw {
		if cm, ok := c.(map[string]any); ok {
			cat := Category{
				CategoryID: cm["category_id"].(string),
				Name:       cm["name"].(string),
			}
			if icon, ok := cm["icon"].(string); ok {
				cat.Icon = icon
			}
			categories = append(categories, cat)
		}
	}

	return categories, nil
}

// CreateCategory creates a new automation category
func (ws *WSClient) CreateCategory(scope, name, icon string) (*Category, error) {
	data := map[string]any{
		"scope": scope,
		"name":  name,
	}
	if icon != "" {
		data["icon"] = icon
	}

	result, err := ws.sendCommand("config/category_registry/create", data)
	if err != nil {
		return nil, err
	}

	catRaw, ok := result["result"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unexpected result type")
	}

	return &Category{
		CategoryID: catRaw["category_id"].(string),
		Name:       catRaw["name"].(string),
	}, nil
}

// AssignCategory assigns a category to an entity
func (ws *WSClient) AssignCategory(scope, entityID, categoryID string) error {
	_, err := ws.sendCommand("config/entity_registry/update", map[string]any{
		"entity_id": entityID,
		"categories": map[string]any{
			scope: categoryID,
		},
	})
	return err
}

// RenameEntityID renames an entity's entity_id
func (ws *WSClient) RenameEntityID(oldEntityID, newEntityID string) error {
	_, err := ws.sendCommand("config/entity_registry/update", map[string]any{
		"entity_id":     oldEntityID,
		"new_entity_id": newEntityID,
	})
	return err
}

// SetEntityName sets an entity's friendly name (display name)
func (ws *WSClient) SetEntityName(entityID, name string) error {
	_, err := ws.sendCommand("config/entity_registry/update", map[string]any{
		"entity_id": entityID,
		"name":      name,
	})
	return err
}

// CreateInputNumber creates an input_number helper
func (ws *WSClient) CreateInputNumber(name string, min, max, step, initial float64, unit, icon string) (string, error) {
	data := map[string]any{
		"name":    name,
		"min":     min,
		"max":     max,
		"step":    step,
		"initial": initial,
	}
	if unit != "" {
		data["unit_of_measurement"] = unit
	}
	if icon != "" {
		data["icon"] = icon
	}

	result, err := ws.sendCommand("input_number/create", data)
	if err != nil {
		return "", err
	}

	// Extract the created entity_id
	if resultData, ok := result["result"].(map[string]any); ok {
		if id, ok := resultData["id"].(string); ok {
			return "input_number." + id, nil
		}
	}

	return "", fmt.Errorf("failed to get created entity_id")
}

// CreateTemplateSensor creates a persistent template sensor via WebSocket API (UI Helper)
func (ws *WSClient) CreateTemplateSensor(name, stateTemplate, unit, deviceClass, icon string) (string, error) {
	// Use template/item/create for UI-based template helpers
	data := map[string]any{
		"template_type": "sensor",
		"name":          name,
		"state":         stateTemplate,
	}
	if unit != "" {
		data["unit_of_measurement"] = unit
	}
	if deviceClass != "" {
		data["device_class"] = deviceClass
	}

	result, err := ws.sendCommand("template/item/create", data)
	if err != nil {
		return "", err
	}

	// Extract the created entity_id
	if resultData, ok := result["result"].(map[string]any); ok {
		if id, ok := resultData["id"].(string); ok {
			return "sensor.template_" + id, nil
		}
	}

	return "", fmt.Errorf("failed to get created entity_id from result: %v", result)
}

// DeviceRegistryEntry represents a device in the device registry
type DeviceRegistryEntry struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	NameByUser   string  `json:"name_by_user,omitempty"`
	Manufacturer string  `json:"manufacturer,omitempty"`
	Model        string  `json:"model,omitempty"`
	AreaID       string  `json:"area_id,omitempty"`
	Identifiers  [][]any `json:"identifiers,omitempty"`
}

// EntityRegistryEntry represents an entity in the entity registry
type EntityRegistryEntry struct {
	EntityID     string `json:"entity_id"`
	UniqueID     string `json:"unique_id,omitempty"`
	Platform     string `json:"platform,omitempty"`
	DeviceID     string `json:"device_id,omitempty"`
	AreaID       string `json:"area_id,omitempty"`
	Name         string `json:"name,omitempty"`
	OriginalName string `json:"original_name,omitempty"`
}

// GetDeviceRegistry gets all devices from the device registry
func (ws *WSClient) GetDeviceRegistry() ([]DeviceRegistryEntry, error) {
	result, err := ws.sendCommand("config/device_registry/list", map[string]any{})
	if err != nil {
		return nil, err
	}

	devicesRaw, ok := result["result"].([]any)
	if !ok {
		return nil, fmt.Errorf("unexpected result type")
	}

	var devices []DeviceRegistryEntry
	for _, d := range devicesRaw {
		if dm, ok := d.(map[string]any); ok {
			dev := DeviceRegistryEntry{}
			if id, ok := dm["id"].(string); ok {
				dev.ID = id
			}
			if name, ok := dm["name"].(string); ok {
				dev.Name = name
			}
			if nameByUser, ok := dm["name_by_user"].(string); ok {
				dev.NameByUser = nameByUser
			}
			if manufacturer, ok := dm["manufacturer"].(string); ok {
				dev.Manufacturer = manufacturer
			}
			if model, ok := dm["model"].(string); ok {
				dev.Model = model
			}
			if areaID, ok := dm["area_id"].(string); ok {
				dev.AreaID = areaID
			}
			devices = append(devices, dev)
		}
	}

	return devices, nil
}

// GetEntityRegistry gets all entities from the entity registry
func (ws *WSClient) GetEntityRegistry() ([]EntityRegistryEntry, error) {
	result, err := ws.sendCommand("config/entity_registry/list", map[string]any{})
	if err != nil {
		return nil, err
	}

	entitiesRaw, ok := result["result"].([]any)
	if !ok {
		return nil, fmt.Errorf("unexpected result type")
	}

	var entities []EntityRegistryEntry
	for _, e := range entitiesRaw {
		if em, ok := e.(map[string]any); ok {
			ent := EntityRegistryEntry{}
			if entityID, ok := em["entity_id"].(string); ok {
				ent.EntityID = entityID
			}
			if uniqueID, ok := em["unique_id"].(string); ok {
				ent.UniqueID = uniqueID
			}
			if platform, ok := em["platform"].(string); ok {
				ent.Platform = platform
			}
			if deviceID, ok := em["device_id"].(string); ok {
				ent.DeviceID = deviceID
			}
			if areaID, ok := em["area_id"].(string); ok {
				ent.AreaID = areaID
			}
			if name, ok := em["name"].(string); ok {
				ent.Name = name
			}
			if originalName, ok := em["original_name"].(string); ok {
				ent.OriginalName = originalName
			}
			entities = append(entities, ent)
		}
	}

	return entities, nil
}

// GetEntityDeviceInfo gets the device_id and related info for an entity
func (ws *WSClient) GetEntityDeviceInfo(entityID string) (*EntityRegistryEntry, error) {
	entities, err := ws.GetEntityRegistry()
	if err != nil {
		return nil, err
	}

	for _, e := range entities {
		if e.EntityID == entityID {
			return &e, nil
		}
	}

	return nil, fmt.Errorf("entity not found: %s", entityID)
}

// CreateScript creates a script via the config API
func (c *Client) CreateScript(id string, config map[string]any) error {
	_, err := c.doRequest("POST", "/api/config/script/config/"+id, config)
	return err
}
