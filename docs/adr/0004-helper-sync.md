# `hac sync` 拉取全部 UI helper，`hac helper apply` 推回 HA

## 背景

`hac sync` 原本只同步 HA 自动化：把 HA 上的 automation YAML 按 category 拉到本地 `automations/` 目录并提交。HA 上通过 UI 手动建立的其他配置实体——`input_boolean`（手动标志）、`input_number`（全局变量）、`input_text`、`input_select`、`input_button`、`input_datetime`、`counter`、`timer`、`schedule`（统称"集合型 helper"）以及通过 config flow 创建的 `template` 传感器（统称"config-entry 型 helper"）——完全不在仓库里，没有版本控制，实例损坏或迁移时无法重建。

旧有的 `hac sync-config` 命令部分解决了这个问题，但只导出 `input_number.yaml` 一种类型，其余类型的 helper 仍处于孤岛状态。

## 两类 Helper 的存储机制

HA 对 UI helper 采用两套截然不同的存储机制，这是本决策的核心约束：

**集合型（Collection-based）**：`input_boolean`、`input_number`、`input_text`、`input_select`、`input_button`、`input_datetime`、`counter`、`timer`、`schedule`。HA 通过 `<domain>/list`（WebSocket）读取所有条目，通过 `<domain>/create` 写入新条目。数据存储在 HA 内部的 `.storage` 目录，不依赖 config flow。

**config-entry 型（Config-entry-based）**：`template`（包括 template 传感器、binary_sensor 等）。这类 helper 经由 HA config flow 创建（`config_entries/flow/init`），条目配置通过 options flow（`config_entries/options/flow/init`）读取 `suggested_value` 字段获得。entity_id 由 HA 根据名称生成 slug 后挂载，无法在创建前指定。

这两种机制由 HA 内部实现决定，按类型固定，**不可互转**。任何新增 helper 类型都天然落入其中一类，不存在中间态。

## 决策

### 1. `hac sync` 追加拉取全部 helper

`hac sync` 在拉取 automation 之后，额外调用集合型和 config-entry 型的读取接口，把所有 helper 写入 `<ConfigRepo>/helpers/<type>.yaml`，一类一文件，以 `object_id` 为 key。示例布局：

```
helpers/
├── input_boolean.yaml
├── input_number.yaml
├── input_text.yaml
├── input_select.yaml
├── counter.yaml
├── timer.yaml
├── template.yaml
└── ...
```

### 2. 新增 `hac helper apply` 推回 HA

`hac helper apply [dir]` 读取 `helpers/` 目录（或指定目录）中的 YAML 文件，把每条记录推回 HA，实现**幂等**语义：已存在的条目跳过，不存在才创建。这使仓库成为灾难恢复的单一入口，新实例可以一条命令重建所有 helper。

### 3. 首版只做"不存在则创建"

首版 `hac helper apply` 只处理"本地有、HA 没有"的情况（幂等新增）。"本地与 HA 差异时更新"留作后续迭代，避免意外覆盖用户在 HA UI 上的临时调整。

### 4. 退役 `hac sync-config`

`hac sync-config` 的功能（导出 input_number）已完整并入 `hac sync`，前者标记为弃用并在下一个 major 版本移除。现有脚本应切换到 `hac sync`。

### 5. 不为冷门 config-entry 类型建降级层

`group`、`threshold` 等极少使用的 config-entry 型 helper 当前在本仓库中不存在。按照 YAGNI 原则不为其预建降级处理路径，等实际出现后再针对性添加。

## 影响

**正向影响**：
- 仓库成为完整可恢复镜像，helper + automation 一起纳入版本控制。
- 重建 HA 实例只需 `git clone` + `hac helper apply` + `hac deploy automations/`，无需从 HA UI 手工重录几十条 input_number/input_boolean。

**约束与注意事项**：
- template helper 的配置读取依赖 options flow 的 `suggested_value` 字段；如果 HA 未来版本移除该字段，读取逻辑需要适配。
- config-entry 型 helper（如 template）的 entity_id 在 HA 侧由名称 slug 生成；`hac helper apply` 创建后需解析 HA 返回的 entry_id，再调用 entity registry 进行 rename，才能保证 entity_id 与仓库记录一致。
- 每新增一种 config-entry 型 helper 类型，都需要为其实现对应的 config flow 驱动代码（init → form step → create），集合型则只需增加 domain 字符串到 catalog 列表。
- `helpers/input_number.yaml` 取代了原有根目录的 `input_number.yaml`；已触发 `hac sync` 迁移，旧文件不再维护。
