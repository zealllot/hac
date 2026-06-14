# hac helper 全量同步 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 `hac sync` 把 HA 上所有 UI helper 拉进配置仓库的 `helpers/*.yaml`,并新增 `hac helper apply` 把它们推回 HA,使仓库成为完整可恢复的镜像。

**Architecture:** 新增 `internal/helpers` 包(纯逻辑 + 编排),复用 `internal/ha` 的 WS/REST 客户端。集合型 helper 走统一的 `<domain>/list` 与 `<domain>/create`;config-entry 型(template)走 REST 读 config entries + options flow 读模板定义、config flow 创建。CLI 层把拉取并入 `cmdSync`,新增 `apply` 子命令,退役 `sync-config`。

**Tech Stack:** Go,Home Assistant WebSocket API(`<domain>/list`、`<domain>/create`、`config/entity_registry/update`)+ REST API(`/api/config/config_entries/entry`、`/api/config/config_entries/options/flow`、`/api/config/config_entries/flow`),`gopkg.in/yaml.v3`。

**测试约定:** 与既有代码一致——`internal/ha` 的方法靠实盘验证(创建→`hac state` 核对→删除清理),不写单测;`internal/helpers` 的纯逻辑(清单序列化、options-flow 提取、域目录)写 Go 单测。

**实盘已确认的事实(实现时可直接依赖):**
- `GET /api/config/config_entries/entry?domain=template` 返回数组,每项含 `entry_id`、`title`、`domain`、`state`,**不含** options。
- `POST /api/config/config_entries/options/flow` body `{"handler": "<entry_id>"}` 返回 `{"type":"form","flow_id":...,"data_schema":[{name, description:{suggested_value}, ...}]}`;`state` 字段的 `description.suggested_value` 即模板公式,`unit_of_measurement`/`device_class`/`state_class` 同理。读完用 `DELETE /api/config/config_entries/options/flow/<flow_id>` 中止。
- template 的 `name` = config entry 的 `title`(options 表单里没有 name)。
- template 的 `icon` 不在配置流里,是实体注册表属性。
- 创建 template 的 config flow:`POST /api/config/config_entries/flow {"handler":"template"}` → `POST …/flow/<flow_id> {"next_step_id":"sensor"}` → `POST …/flow/<flow_id> {name, state, ...}`(已在 `Client.CreateTemplateSensor` 实现)。
- 集合型 helper 的 `editable: true`;`<domain>/list` 返回 `[{id, name, ...config}]`,`id` 即 object_id。

---

## File Structure

**新建:**
- `internal/helpers/catalog.go` — 域目录:哪些是集合型、哪些是 config-entry 型。
- `internal/helpers/model.go` — `Helper` 与 `Manifest` 类型。
- `internal/helpers/manifest.go` — `helpers/<domain>.yaml` 的读写(纯 YAML)。
- `internal/helpers/capture.go` — 从 HA 枚举所有 helper。
- `internal/helpers/apply.go` — 把 helper 推回 HA。
- `internal/helpers/catalog_test.go` / `model_test.go` / `manifest_test.go` — 纯逻辑单测。
- `docs/adr/0004-helper-sync.md` — 记录设计决定。

**修改:**
- `internal/ha/client.go` — 加 `ListCollectionHelpers`、`CreateCollectionHelper`、`GetConfigEntriesByDomain`、`ReadConfigEntryOptions`、`AbortOptionsFlow`、`SetEntityIcon`;把 `CreateTemplateSensor` 改成接 config map。
- `cmd/hac/main.go` — `cmdSync` 追加 helper 拉取;新增 `runHelperApply` 并接入 `runHelper` 分发;`cmdSyncConfig` 改为弃用提示;顶层帮助文本更新。
- `README.md` — 命令文档更新。

---

## Task 1: ha client — 集合型 helper 的 list 与 create

**Files:**
- Modify: `internal/ha/client.go`

集合型 9 种共用 `<domain>/list` 和 `<domain>/create`,加两个通用方法。

- [ ] **Step 1: 加 `ListCollectionHelpers`**

在 `internal/ha/client.go` 的 WSClient 方法区(紧接 `DeleteCollectionHelper` 之后)加:

```go
// ListCollectionHelpers returns every item of a storage-collection helper
// (input_boolean, input_number, ...). Each item is the raw config map and
// includes an "id" key holding the object_id.
func (ws *WSClient) ListCollectionHelpers(domain string) ([]map[string]any, error) {
	result, err := ws.sendCommand(domain+"/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	raw, ok := result["result"].([]any)
	if !ok {
		return nil, fmt.Errorf("%s/list: unexpected result type", domain)
	}
	items := make([]map[string]any, 0, len(raw))
	for _, r := range raw {
		if m, ok := r.(map[string]any); ok {
			items = append(items, m)
		}
	}
	return items, nil
}
```

- [ ] **Step 2: 加 `CreateCollectionHelper`**

```go
// CreateCollectionHelper creates a storage-collection helper via <domain>/create
// using the given config (name + type-specific fields, WITHOUT an "id" key) and
// returns the entity_id HA assigned (domain + slugified name). Rename afterwards
// to pin the desired object_id.
func (ws *WSClient) CreateCollectionHelper(domain string, config map[string]any) (string, error) {
	result, err := ws.sendCommand(domain+"/create", config)
	if err != nil {
		return "", err
	}
	if resultData, ok := result["result"].(map[string]any); ok {
		if id, ok := resultData["id"].(string); ok {
			return domain + "." + id, nil
		}
	}
	return "", fmt.Errorf("%s/create: no id in result: %v", domain, result)
}
```

- [ ] **Step 3: 编译**

Run: `cd ~/go/src/github.com/zealllot/hac && go build ./...`
Expected: 无错误。

- [ ] **Step 4: 实盘验证 list**

临时在 `cmd/hac` 用 `hac` 已有命令无法直接调这两个方法,改为最小验证:用 `hac state 'input_boolean.*'` 确认本地能连 HA(已知可用)。两方法的真正验证放在 Task 6/7 的 Capture/Apply 实盘环节,此处只确保编译通过。

- [ ] **Step 5: Commit**

```bash
cd ~/go/src/github.com/zealllot/hac
git add internal/ha/client.go
git commit -m "feat(ha): 集合型 helper 的通用 list/create"
```

---

## Task 2: ha client — 读 config entries 与 options

**Files:**
- Modify: `internal/ha/client.go`

config-entry 型 helper(template)的枚举与配置读取。

- [ ] **Step 1: 加 `ConfigEntry` 类型与 `GetConfigEntriesByDomain`**

在 `client.go` 类型区加:

```go
// ConfigEntry is the subset of a config entry returned by the REST list endpoint.
type ConfigEntry struct {
	EntryID string `json:"entry_id"`
	Domain  string `json:"domain"`
	Title   string `json:"title"`
	State   string `json:"state"`
}
```

在 Client 方法区加:

```go
// GetConfigEntriesByDomain lists config entries for one integration domain.
// Used to enumerate config-entry helpers (e.g. template sensors).
func (c *Client) GetConfigEntriesByDomain(domain string) ([]ConfigEntry, error) {
	data, err := c.doRequest("GET", "/api/config/config_entries/entry?domain="+domain, nil)
	if err != nil {
		return nil, err
	}
	var entries []ConfigEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("unmarshal config entries (body=%q): %w", string(data), err)
	}
	return entries, nil
}
```

- [ ] **Step 2: 加 `ReadConfigEntryOptions` + `AbortOptionsFlow`**

options flow 的 form 里每个字段的当前值在 `description.suggested_value`。提取成 `name -> value` map,然后中止 flow。

```go
// ReadConfigEntryOptions reads the current stored options of a config entry by
// starting its options flow and harvesting each field's suggested_value, then
// aborting the flow. Works for helper config entries such as template sensors.
func (c *Client) ReadConfigEntryOptions(entryID string) (map[string]any, error) {
	data, err := c.doRequest("POST", "/api/config/config_entries/options/flow",
		map[string]any{"handler": entryID})
	if err != nil {
		return nil, err
	}
	var form struct {
		Type       string `json:"type"`
		FlowID     string `json:"flow_id"`
		DataSchema []struct {
			Name        string `json:"name"`
			Description struct {
				SuggestedValue any `json:"suggested_value"`
			} `json:"description"`
		} `json:"data_schema"`
	}
	if err := json.Unmarshal(data, &form); err != nil {
		return nil, fmt.Errorf("unmarshal options form (body=%q): %w", string(data), err)
	}
	if form.FlowID != "" {
		// Best-effort cleanup; ignore error (flows expire on their own).
		_ = c.AbortOptionsFlow(form.FlowID)
	}
	opts := make(map[string]any)
	for _, f := range form.DataSchema {
		if f.Name != "" && f.Description.SuggestedValue != nil {
			opts[f.Name] = f.Description.SuggestedValue
		}
	}
	return opts, nil
}

// AbortOptionsFlow cancels an in-progress options flow.
func (c *Client) AbortOptionsFlow(flowID string) error {
	_, err := c.doRequest("DELETE", "/api/config/config_entries/options/flow/"+flowID, nil)
	return err
}
```

- [ ] **Step 3: 编译**

Run: `cd ~/go/src/github.com/zealllot/hac && go build ./...`
Expected: 无错误。

- [ ] **Step 4: Commit**

```bash
git add internal/ha/client.go
git commit -m "feat(ha): 读 config entries 与 options(供 template helper 捕获)"
```

---

## Task 3: ha client — CreateTemplateSensor 接 config map + SetEntityIcon

**Files:**
- Modify: `internal/ha/client.go`
- Modify: `cmd/hac/main.go`(更新 `CreateTemplateSensor` 调用处)

让 template 创建支持 state_class 等任意字段,并能设置实体图标。

- [ ] **Step 1: 改 `CreateTemplateSensor` 签名为接 config map**

把现有 `func (c *Client) CreateTemplateSensor(name, stateTemplate, unit, deviceClass, icon string) (string, error)` 整体替换为:

```go
// CreateTemplateSensor creates a template sensor (UI "Template" helper) by
// driving its config flow, and returns the created config entry id. opts may
// contain state, unit_of_measurement, device_class, state_class. `name` becomes
// the entry title. Icon is NOT a config-flow field — set it separately on the
// entity via SetEntityIcon after resolving the entity_id.
func (c *Client) CreateTemplateSensor(name string, opts map[string]any) (string, error) {
	start, err := c.startConfigFlow("template")
	if err != nil {
		return "", fmt.Errorf("start template flow: %w", err)
	}
	flowID, _ := start["flow_id"].(string)
	if flowID == "" {
		return "", fmt.Errorf("template flow returned no flow_id: %v", start)
	}
	menu, err := c.configFlowStep(flowID, map[string]any{"next_step_id": "sensor"})
	if err != nil {
		return "", fmt.Errorf("select sensor step: %w", err)
	}
	if ft, _ := menu["type"].(string); ft != "form" {
		return "", fmt.Errorf("expected sensor form, got %v: %v", menu["type"], menu)
	}
	form := map[string]any{"name": name}
	for k, v := range opts {
		if v == nil || v == "" {
			continue
		}
		form[k] = v
	}
	done, err := c.configFlowStep(flowID, form)
	if err != nil {
		return "", fmt.Errorf("submit sensor form: %w", err)
	}
	if ft, _ := done["type"].(string); ft != "create_entry" {
		return "", fmt.Errorf("template flow did not create entry (type=%v): %v", done["type"], done)
	}
	if result, ok := done["result"].(map[string]any); ok {
		if entryID, ok := result["entry_id"].(string); ok {
			return entryID, nil
		}
	}
	return "", fmt.Errorf("template flow created entry but returned no entry_id: %v", done)
}
```

(`startConfigFlow` / `configFlowStep` 已存在,无需改。)

- [ ] **Step 2: 加 `SetEntityIcon`**

紧接 `SetEntityName` 之后:

```go
// SetEntityIcon sets an entity's icon override in the entity registry.
func (ws *WSClient) SetEntityIcon(entityID, icon string) error {
	_, err := ws.sendCommand("config/entity_registry/update", map[string]any{
		"entity_id": entityID,
		"icon":      icon,
	})
	return err
}
```

- [ ] **Step 3: 更新 `runHelper` 里 `CreateTemplateSensor` 的调用处**

在 `cmd/hac/main.go` 的 `runHelper` 中,`case "template_sensor":` 分支替换为:

```go
	case "template_sensor":
		// The template helper is a config-entry helper: create via its config flow
		// (returns the entry id), resolve the entity_id it spawned, then set icon.
		var entryID string
		entryID, err = client.CreateTemplateSensor(name, map[string]any{
			"state":               state,
			"unit_of_measurement": unit,
			"device_class":        deviceClass,
		})
		if err == nil {
			created, err = ws.ResolveEntityByConfigEntry(entryID)
		}
		if err == nil && icon != "" {
			if e := ws.SetEntityIcon(created, icon); e != nil {
				fmt.Fprintf(os.Stderr, "Warning: created %s but failed to set icon: %v\n", created, e)
			}
		}
```

- [ ] **Step 4: 编译**

Run: `cd ~/go/src/github.com/zealllot/hac && go build ./... && go install ./cmd/hac`
Expected: 无错误。

- [ ] **Step 5: 实盘验证创建 + 图标 + 删除**

```bash
hac helper create template_sensor hac_t3_probe --state "{{ 5 }}" --name "hac_t3_probe" --unit "个" --icon "mdi:lan-disconnect"
sleep 2
hac state sensor.hac_t3_probe   # 期望 state=5, attributes.icon=mdi:lan-disconnect, unit=个
hac helper delete sensor.hac_t3_probe
```
Expected: 创建成功、state=5、icon 生效;删除成功。

- [ ] **Step 6: Commit**

```bash
git add internal/ha/client.go cmd/hac/main.go
git commit -m "feat(ha): CreateTemplateSensor 接 config map + 设置实体图标"
```

---

## Task 4: helpers 包 — 域目录(catalog)

**Files:**
- Create: `internal/helpers/catalog.go`
- Test: `internal/helpers/catalog_test.go`

- [ ] **Step 1: 写失败测试**

`internal/helpers/catalog_test.go`:

```go
package helpers

import "testing"

func TestCollectionDomains(t *testing.T) {
	got := CollectionDomains()
	want := map[string]bool{
		"input_boolean": true, "input_number": true, "input_text": true,
		"input_select": true, "input_button": true, "input_datetime": true,
		"counter": true, "timer": true, "schedule": true,
	}
	if len(got) != len(want) {
		t.Fatalf("got %d domains, want %d", len(got), len(want))
	}
	for _, d := range got {
		if !want[d] {
			t.Errorf("unexpected domain %q", d)
		}
	}
}

func TestConfigEntryDomains(t *testing.T) {
	got := ConfigEntryDomains()
	if len(got) != 1 || got[0] != "template" {
		t.Fatalf("got %v, want [template]", got)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd ~/go/src/github.com/zealllot/hac && go test ./internal/helpers/`
Expected: FAIL(`undefined: CollectionDomains`)。

- [ ] **Step 3: 实现 catalog**

`internal/helpers/catalog.go`:

```go
// Package helpers captures Home Assistant UI helpers into the config repo and
// applies them back. It spans two HA storage mechanisms: storage-collection
// helpers (input_*, counter, timer, schedule) and config-entry helpers
// (template sensors).
package helpers

// CollectionDomains lists the storage-collection helper domains, each managed
// through uniform <domain>/list and <domain>/create WS commands.
func CollectionDomains() []string {
	return []string{
		"input_boolean", "input_number", "input_text", "input_select",
		"input_button", "input_datetime", "counter", "timer", "schedule",
	}
}

// ConfigEntryDomains lists the config-entry helper domains hac can round-trip.
// Only template today; extend as more config-flow drivers are added.
func ConfigEntryDomains() []string {
	return []string{"template"}
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/helpers/`
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/helpers/catalog.go internal/helpers/catalog_test.go
git commit -m "feat(helpers): helper 域目录"
```

---

## Task 5: helpers 包 — 清单读写

**Files:**
- Create: `internal/helpers/model.go`
- Create: `internal/helpers/manifest.go`
- Test: `internal/helpers/manifest_test.go`

清单文件 = `map[objectID]config`,每个域一个文件。`config` 不含 `id`(object_id 是 key)。

- [ ] **Step 1: 写 model 类型**

`internal/helpers/model.go`:

```go
package helpers

// Manifest is one helper-type file: object_id -> config map (config never
// contains an "id" key; the object_id is the map key).
type Manifest map[string]map[string]any
```

- [ ] **Step 2: 写失败测试**

`internal/helpers/manifest_test.go`:

```go
package helpers

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestManifestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	m := Manifest{
		"quan_ju_liang_du": {"name": "全局亮度", "min": 1, "max": 100, "step": 5},
		"ke_ting_shou_dong": {"name": "客厅手动", "icon": "mdi:gesture-tap"},
	}
	path := filepath.Join(dir, "input_number.yaml")
	if err := WriteManifest(path, m); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := ReadManifest(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got["ke_ting_shou_dong"]["icon"] != "mdi:gesture-tap" {
		t.Errorf("icon round-trip failed: %v", got["ke_ting_shou_dong"])
	}
	if len(got) != 2 {
		t.Errorf("got %d entries, want 2", len(got))
	}
}

func TestReadManifestMissingFileIsEmpty(t *testing.T) {
	got, err := ReadManifest(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want empty manifest, got %v", got)
	}
}

func TestFromCollectionItems(t *testing.T) {
	items := []map[string]any{
		{"id": "a", "name": "A", "min": 0},
		{"id": "b", "name": "B"},
	}
	m := FromCollectionItems(items)
	want := Manifest{"a": {"name": "A", "min": 0}, "b": {"name": "B"}}
	if !reflect.DeepEqual(m, want) {
		t.Errorf("got %v, want %v", m, want)
	}
	if _, hasID := m["a"]["id"]; hasID {
		t.Error("id should be stripped from config")
	}
}

var _ = os.Stdout
```

- [ ] **Step 3: 跑测试确认失败**

Run: `go test ./internal/helpers/ -run TestManifest`
Expected: FAIL(`undefined: WriteManifest`)。

- [ ] **Step 4: 实现 manifest**

`internal/helpers/manifest.go`:

```go
package helpers

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ReadManifest loads one helper-type file. A missing file yields an empty
// manifest (not an error), so callers can treat absent files as "no helpers".
func ReadManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Manifest{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	m := Manifest{}
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return m, nil
}

// WriteManifest writes one helper-type file, creating parent dirs. yaml.v3
// sorts map keys, so output is deterministic across runs.
func WriteManifest(path string, m Manifest) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// FromCollectionItems converts <domain>/list output into a Manifest, using each
// item's "id" as the object_id key and the remaining fields as its config.
func FromCollectionItems(items []map[string]any) Manifest {
	m := Manifest{}
	for _, item := range items {
		id, _ := item["id"].(string)
		if id == "" {
			continue
		}
		cfg := make(map[string]any, len(item))
		for k, v := range item {
			if k == "id" {
				continue
			}
			cfg[k] = v
		}
		m[id] = cfg
	}
	return m
}
```

- [ ] **Step 5: 跑测试确认通过**

Run: `go test ./internal/helpers/`
Expected: PASS。

- [ ] **Step 6: Commit**

```bash
git add internal/helpers/model.go internal/helpers/manifest.go internal/helpers/manifest_test.go
git commit -m "feat(helpers): 清单读写与集合型项转换"
```

---

## Task 6: helpers 包 — Capture(拉取)

**Files:**
- Create: `internal/helpers/capture.go`

把所有 helper 从 HA 读成 `map[domain]Manifest`。

- [ ] **Step 1: 定义客户端接口与 Capture**

`internal/helpers/capture.go`:

```go
package helpers

import (
	"fmt"
	"strings"

	"github.com/zealllot/hac/internal/ha"
)

// Capturer is the subset of HA client behaviour Capture needs.
type Capturer struct {
	WS     *ha.WSClient
	Client *ha.Client
}

// Capture reads every UI helper from HA, returning domain -> Manifest. Per-domain
// failures are collected as warnings (returned), never abort the whole capture.
func (c Capturer) Capture() (map[string]Manifest, []string) {
	out := make(map[string]Manifest)
	var warns []string

	for _, domain := range CollectionDomains() {
		items, err := c.WS.ListCollectionHelpers(domain)
		if err != nil {
			warns = append(warns, fmt.Sprintf("list %s: %v", domain, err))
			continue
		}
		if m := FromCollectionItems(items); len(m) > 0 {
			out[domain] = m
		}
	}

	for _, domain := range ConfigEntryDomains() {
		if domain != "template" {
			continue // only template is supported today (see catalog.go)
		}
		m, ws := c.captureTemplates()
		warns = append(warns, ws...)
		if len(m) > 0 {
			out["template_sensor"] = m
		}
	}

	return out, warns
}

// captureTemplates reads template sensors: entry title -> name, options flow ->
// state/unit/device_class/state_class, entity registry -> object_id + icon.
func (c Capturer) captureTemplates() (Manifest, []string) {
	m := Manifest{}
	var warns []string

	entries, err := c.Client.GetConfigEntriesByDomain("template")
	if err != nil {
		return m, []string{fmt.Sprintf("list template entries: %v", err)}
	}

	regByEntry := map[string]ha.EntityRegistryEntry{}
	if reg, err := c.WS.GetEntityRegistry(); err == nil {
		for _, e := range reg {
			if e.ConfigEntryID != "" {
				regByEntry[e.ConfigEntryID] = e
			}
		}
	}

	for _, e := range entries {
		ent, ok := regByEntry[e.EntryID]
		if !ok || !strings.HasPrefix(ent.EntityID, "sensor.") {
			warns = append(warns, fmt.Sprintf("template %s: no sensor entity for entry", e.Title))
			continue
		}
		objectID := strings.TrimPrefix(ent.EntityID, "sensor.")

		opts, err := c.Client.ReadConfigEntryOptions(e.EntryID)
		if err != nil {
			warns = append(warns, fmt.Sprintf("read options %s: %v", e.Title, err))
			continue
		}
		cfg := map[string]any{"name": e.Title}
		for _, k := range []string{"state", "unit_of_measurement", "device_class", "state_class"} {
			if v, ok := opts[k]; ok && v != nil && v != "" {
				cfg[k] = v
			}
		}
		if ent.Icon != "" {
			cfg["icon"] = ent.Icon
		}
		m[objectID] = cfg
	}
	return m, warns
}
```

- [ ] **Step 2: 给 `EntityRegistryEntry` 加 `Icon` 字段并解析**

在 `internal/ha/client.go` 的 `EntityRegistryEntry` 结构加字段:

```go
	Icon          string            `json:"icon,omitempty"`
```

在 `GetEntityRegistry` 的解析循环里(`config_entry_id` 解析之后)加:

```go
			if icon, ok := em["icon"].(string); ok {
				ent.Icon = icon
			}
```

- [ ] **Step 3: 编译**

Run: `cd ~/go/src/github.com/zealllot/hac && go build ./...`
Expected: 无错误。

- [ ] **Step 4: Commit**

```bash
git add internal/helpers/capture.go internal/ha/client.go
git commit -m "feat(helpers): 从 HA 捕获全部 helper"
```

---

## Task 7: helpers 包 — Apply(推回)

**Files:**
- Create: `internal/helpers/apply.go`

把 `map[domain]Manifest` 推回 HA;幂等(实体已存在则跳过)。

- [ ] **Step 1: 实现 Apply**

`internal/helpers/apply.go`:

```go
package helpers

import (
	"fmt"

	"github.com/zealllot/hac/internal/ha"
)

// ApplyReport summarises an apply run.
type ApplyReport struct {
	Created []string
	Skipped []string
	Failed  []string // "entity_id: reason"
}

// Applier pushes manifests back to HA.
type Applier struct {
	WS     *ha.WSClient
	Client *ha.Client
}

// Apply creates every helper in `byDomain` that does not already exist on HA.
// byDomain keys are the manifest file stems: the 9 collection domains plus
// "template_sensor".
func (a Applier) Apply(byDomain map[string]Manifest) ApplyReport {
	var rep ApplyReport
	for fileDomain, m := range byDomain {
		for objectID, cfg := range m {
			entityID := entityDomain(fileDomain) + "." + objectID
			if _, err := a.Client.GetState(entityID); err == nil {
				rep.Skipped = append(rep.Skipped, entityID)
				continue
			}
			if err := a.create(fileDomain, objectID, entityID, cfg); err != nil {
				rep.Failed = append(rep.Failed, fmt.Sprintf("%s: %v", entityID, err))
				continue
			}
			rep.Created = append(rep.Created, entityID)
		}
	}
	return rep
}

// entityDomain maps a manifest file stem to the entity domain. Collection files
// are named after their domain; template_sensor lands in the sensor domain.
func entityDomain(fileDomain string) string {
	if fileDomain == "template_sensor" {
		return "sensor"
	}
	return fileDomain
}

func (a Applier) create(fileDomain, objectID, entityID string, cfg map[string]any) error {
	if fileDomain == "template_sensor" {
		name, _ := cfg["name"].(string)
		entryID, err := a.Client.CreateTemplateSensor(name, map[string]any{
			"state":               cfg["state"],
			"unit_of_measurement": cfg["unit_of_measurement"],
			"device_class":        cfg["device_class"],
			"state_class":         cfg["state_class"],
		})
		if err != nil {
			return err
		}
		created, err := a.WS.ResolveEntityByConfigEntry(entryID)
		if err != nil {
			return err
		}
		if created != entityID {
			if err := a.WS.RenameEntityID(created, entityID); err != nil {
				return err
			}
		}
		if icon, _ := cfg["icon"].(string); icon != "" {
			_ = a.WS.SetEntityIcon(entityID, icon)
		}
		return nil
	}

	// Collection helper: create with config (drop nulls), then rename to object_id.
	clean := make(map[string]any, len(cfg))
	for k, v := range cfg {
		if v != nil {
			clean[k] = v
		}
	}
	created, err := a.WS.CreateCollectionHelper(fileDomain, clean)
	if err != nil {
		return err
	}
	if created != entityID {
		if err := a.WS.RenameEntityID(created, entityID); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 2: 编译**

Run: `cd ~/go/src/github.com/zealllot/hac && go build ./...`
Expected: 无错误。

- [ ] **Step 3: Commit**

```bash
git add internal/helpers/apply.go
git commit -m "feat(helpers): 把 helper 推回 HA(幂等创建)"
```

---

## Task 8: cmd — `hac sync` 追加 helper 拉取

**Files:**
- Modify: `cmd/hac/main.go`

`cmdSync` 拉完 automation 后,调用 `helpers.Capture` 并写入 `<ConfigRepo>/helpers/<domain>.yaml`。

- [ ] **Step 1: 在 `cmd/hac/main.go` 顶部 import 加 helpers 包**

确保 import 块含:

```go
	"github.com/zealllot/hac/internal/helpers"
```

- [ ] **Step 2: 在 `cmdSync` 末尾(automation 同步报告打印之后、函数返回之前)插入 helper 拉取**

定位 `cmdSync`(约 `main.go:723`)。在它创建了 `client`、`ws`(若没有 ws,新建一个)并完成 automation 同步后,加入:

```go
	// Capture UI helpers into <ConfigRepo>/helpers/<domain>.yaml.
	cap := helpers.Capturer{WS: ws, Client: client}
	byDomain, warns := cap.Capture()
	for _, w := range warns {
		fmt.Fprintf(os.Stderr, "Warning: helper capture: %s\n", w)
	}
	helpersDir := filepath.Join(cfg.ConfigRepo, "helpers")
	for domain, m := range byDomain {
		path := filepath.Join(helpersDir, domain+".yaml")
		if err := helpers.WriteManifest(path, m); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: write %s: %v\n", path, err)
			continue
		}
		fmt.Printf("helpers: wrote %s (%d)\n", path, len(m))
	}
```

> 实现注意:`cmdSync` 现有代码若尚未建立 `ws`(WSClient)或 `cfg`,在调用处之前补:`ws, err := client.NewWSClient(); if err != nil { ... }; defer ws.Close()` 和 `cfg, _ := config.Load()`。沿用文件中既有的 client/ws/cfg 变量名,避免重复创建。

- [ ] **Step 3: 编译并安装**

Run: `cd ~/go/src/github.com/zealllot/hac && go build ./... && go install ./cmd/hac`
Expected: 无错误。

- [ ] **Step 4: 实盘验证**

```bash
cd ~/go/src/github.com/zealllot/ha-config
hac sync
ls helpers/
cat helpers/input_number.yaml | head
cat helpers/template_sensor.yaml   # 应含 mijia_li_xian_shu，state 为掉线数公式
```
Expected: `helpers/` 下出现 input_boolean.yaml、input_number.yaml、template_sensor.yaml 等;template_sensor.yaml 里 `mijia_li_xian_shu.state` 为那串 Jinja。

> 注意:`hac sync` 会 git 提交。本次先不提交 `helpers/` 到 ha-config(由用户决定),验证后可 `git -C ~/go/src/github.com/zealllot/ha-config restore --staged helpers/` 暂存撤回观察,或接受提交。

- [ ] **Step 5: Commit(hac 仓库)**

```bash
cd ~/go/src/github.com/zealllot/hac
git add cmd/hac/main.go
git commit -m "feat(sync): hac sync 追加拉取全部 UI helper 到 helpers/"
```

---

## Task 9: cmd — `hac helper apply`

**Files:**
- Modify: `cmd/hac/main.go`

- [ ] **Step 1: 在 `runHelper` 分发里接入 apply**

`runHelper` 开头(`delete` 分支旁)加:

```go
	if args[0] == "apply" {
		runHelperApply(args[1:])
		return
	}
```

并把 usage 常量补一行:`  hac helper apply [dir]   (default: <ConfigRepo>/helpers)`。

- [ ] **Step 2: 实现 `runHelperApply`(放在 `runHelperDelete` 之后)**

```go
func runHelperApply(args []string) {
	flags, rest, err := cliflags.ParseWith("helper", args, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	dir := filepath.Join(cfg.ConfigRepo, "helpers")
	if len(rest) > 0 {
		dir = rest[0]
	}

	// Load every <stem>.yaml in dir into byDomain keyed by file stem.
	byDomain := map[string]helpers.Manifest{}
	for _, stem := range append(helpers.CollectionDomains(), "template_sensor") {
		path := filepath.Join(dir, stem+".yaml")
		m, err := helpers.ReadManifest(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if len(m) > 0 {
			byDomain[stem] = m
		}
	}

	client := getClient(flags.Timeout)
	ws, err := client.NewWSClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer ws.Close()

	rep := helpers.Applier{WS: ws, Client: client}.Apply(byDomain)
	fmt.Printf("apply: created=%d skipped=%d failed=%d\n",
		len(rep.Created), len(rep.Skipped), len(rep.Failed))
	for _, id := range rep.Created {
		fmt.Printf("  created %s\n", id)
	}
	for _, f := range rep.Failed {
		fmt.Fprintf(os.Stderr, "  FAILED %s\n", f)
	}
	if len(rep.Failed) > 0 {
		os.Exit(1)
	}
}
```

- [ ] **Step 3: 编译并安装**

Run: `cd ~/go/src/github.com/zealllot/hac && go build ./... && go install ./cmd/hac`
Expected: 无错误。

- [ ] **Step 4: 实盘验证(往返)**

用一个临时清单验证 apply 能创建并幂等:

```bash
mkdir -p /tmp/happly
printf 'hac_t9_probe:\n  name: hac_t9_probe\n  min: 0\n  max: 10\n  step: 1\n' > /tmp/happly/input_number.yaml
hac helper apply /tmp/happly        # 期望 created=1
sleep 2
hac state input_number.hac_t9_probe # 期望 state 在 0..10
hac helper apply /tmp/happly        # 期望 skipped=1（幂等）
hac helper delete input_number.hac_t9_probe
rm -rf /tmp/happly
```
Expected: 首次 created=1、实体存在;二次 skipped=1;删除成功。

- [ ] **Step 5: Commit**

```bash
git add cmd/hac/main.go
git commit -m "feat(helper): hac helper apply 把 helpers/ 推回 HA"
```

---

## Task 10: cmd — 退役 sync-config + 迁移 input_number.yaml

**Files:**
- Modify: `cmd/hac/main.go`
- Modify(ha-config 仓库): 删 `input_number.yaml`,新增 `helpers/`(由 Task 8 的 `hac sync` 产出)

- [ ] **Step 1: 把 `cmdSyncConfig` 改为弃用提示**

整体替换 `cmdSyncConfig` 函数体为:

```go
func cmdSyncConfig(timeout time.Duration) {
	_ = timeout
	fmt.Fprintln(os.Stderr,
		"hac sync-config 已弃用:请改用 `hac sync`(现已包含全部 UI helper,写入 helpers/)。")
	os.Exit(2)
}
```

(若 `cmdSyncConfig` 内原有 import 现在没用了,删掉相应未使用 import 以通过编译。)

- [ ] **Step 2: 编译**

Run: `cd ~/go/src/github.com/zealllot/hac && go build ./...`
Expected: 无错误(如报未使用 import,删除之)。

- [ ] **Step 3: 顶层帮助文本同步**

把 `printUsage`(约 `main.go:809+`)里 sync-config 相关行改为标注弃用,并补 `hac helper apply`、`hac sync` 含 helper 的说明。

- [ ] **Step 4: 实盘验证弃用提示**

Run: `hac sync-config`
Expected: 打印弃用提示,退出码 2。

- [ ] **Step 5: 迁移 ha-config 仓库的 input_number.yaml**

确认 Task 8 的 `hac sync` 已生成 `helpers/input_number.yaml` 且内容覆盖原 `input_number.yaml` 的全部条目后,删除根文件:

```bash
cd ~/go/src/github.com/zealllot/ha-config
# 比对:确认 helpers/input_number.yaml 的 key 覆盖旧文件
diff <(grep -oE '^[a-z_]+:' input_number.yaml | sort) \
     <(grep -oE '^[a-z_]+:' helpers/input_number.yaml | sort) || true
git rm input_number.yaml
```

> 若 diff 显示旧文件有新文件缺失的 key,停下排查(可能是某些 input_number 非 editable / 已删),不要直接删。

- [ ] **Step 6: Commit(两个仓库)**

```bash
cd ~/go/src/github.com/zealllot/hac
git add cmd/hac/main.go
git commit -m "feat(sync): 退役 sync-config，引导改用 hac sync"

cd ~/go/src/github.com/zealllot/ha-config
git add helpers/ && git rm --cached input_number.yaml 2>/dev/null; git add -A
git commit -m "迁移:input_number.yaml → helpers/，纳入全部 UI helper 同步"
```

---

## Task 11: 文档

**Files:**
- Create: `docs/adr/0004-helper-sync.md`
- Modify: `README.md`

- [ ] **Step 1: 写 ADR**

`docs/adr/0004-helper-sync.md`,记录:为何 helper 分集合型/config-entry 两类、为何 template 用 config flow + options flow 读写、为何退役 sync-config、为何首版只"不存在才建"。沿用 docs/adr/0001-0003 的格式(Context / Decision / Consequences)。

- [ ] **Step 2: 更新 README**

在命令清单加 `hac helper apply`、`hac sync` 含 helper 的说明,标注 `sync-config` 弃用。

- [ ] **Step 3: Commit**

```bash
git add docs/adr/0004-helper-sync.md README.md
git commit -m "docs: helper 同步 ADR 与 README"
```

---

## Self-Review 备忘(已核对)

- **Spec 覆盖**:范围(所有 UI helper)→ Task 4 目录 + 6/7 捕获/推回;拉为主+可推回 → Task 8(sync 拉)+ 9(apply 推);一类一文件 → Task 5 清单 + 8 写入;退役 sync-config + 迁移 input_number.yaml → Task 10;集合型与 template 完整往返 → Task 1/3/6/7;不设降级层 → catalog 只含 template。
- **类型一致性**:`Manifest`(Task 5)贯穿 capture/apply/cmd;`Capturer`/`Applier` 都用 `WS *ha.WSClient` + `Client *ha.Client`;`CreateTemplateSensor(name string, opts map[string]any)` 在 Task 3 定签名,Task 7 按此调用;`entityDomain` 把 `template_sensor` 文件名映射到 `sensor` 域,capture(写 `out["template_sensor"]`)与 apply(读 `template_sensor`)一致。
- **占位符**:无 TODO/TBD;每个改代码的步骤含完整代码。Task 8 对 `cmdSync` 既有变量名(client/ws/cfg)做了"沿用现有、缺则补建"的明确说明,执行时需读一遍 `cmdSync` 现状对接。
