package ha

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
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
	_, err := c.doRequest("POST", "/api/config/automation/config", automation)
	return err
}

func (c *Client) UpdateAutomation(id string, automation map[string]any) error {
	_, err := c.doRequest("PUT", "/api/config/automation/config/"+id, automation)
	return err
}

func (c *Client) DeleteAutomation(id string) error {
	_, err := c.doRequest("DELETE", "/api/config/automation/config/"+id, nil)
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
			State:      s.State,
			Supports:   serviceMap[domain],
			Attributes: attrs,
		}
	}

	return devices, nil
}
