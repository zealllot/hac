# 设计:hac helper 全量同步

日期:2026-06-14
状态:已确认设计,待实现计划

## 背景与动机

`hac sync` 目前只把 automation 从 HA 拉到本地仓库并提交;`hac sync-config`
只把 `input_number` 实体导出到根目录 `input_number.yaml`。HA 上手动创建的其他
UI helper(input_boolean 手动标志、template 传感器等)**不在仓库里**,既不受
版本控制,也无法从仓库重建——重装/清空 HA 后会静默丢失。

本次目标:让 hac 能把**所有 UI helper** 纳入仓库并与 HA 同步,使仓库成为一份
完整、可恢复的镜像。

参见前置工作:`hac helper create template_sensor` / `hac helper delete`
(feat/template-sensor-helper 分支)已能创建/删除 config-entry 型 helper。

## 关键事实(经实盘核实)

1. HA 的 helper 按类型固定为两种存储机制,**不可互转**:
   - **集合型(storage collection)**:input_boolean、input_number、input_text、
     input_select、input_button、input_datetime、counter、timer、schedule。
     有统一的 `<domain>/list`、`<domain>/create`、`<domain>/update`、
     `<domain>/delete` WS 接口。
   - **config-entry 型**:template、group、threshold、derivative 等。每种经各自的
     config flow(创建向导)生成。本仓库目前**只有一个**:
     `sensor.mijia_li_xian_shu`(掉线数模板传感器)。
2. 本仓库的 `input_number`(全局亮度/色温/阈值等)是 `editable: true` 的 UI helper
   (来自存储集合),根目录 `input_number.yaml` 只是 `sync-config` 的归档,
   **并未被 HA 的 configuration.yaml 加载**(同理 `template.yaml` 也未加载:
   `sensor.global_reference_illumination` 在 HA 上不存在,光照自动化直接引用原始
   米家传感器)。因此把 `input_number.yaml` 迁走是安全的。
3. 掉线数是"算出来的输出"(state = Jinja 公式,引用实体一变就自动重算),
   本质是 template 传感器,不能用 input_number(那是"人设的输入")替代。
   经确认:**保留为 config-entry UI helper**。

## 范围

- 覆盖**所有 UI helper**(集合型 9 种 + config-entry 型 template)。
- 同步方向:**拉为主 + 可推回恢复**(沿用现有 HA=源、仓库=镜像 的模型,不做
  双向冲突合并)。
- 集合型与 template **都做完整双向往返**;因当前不存在 group/threshold 等其他
  config-entry 型,**不设"冷门类型只备份不推回"的降级层**(YAGNI;未来出现再加)。

## 命令设计

| 命令 | 行为 |
|---|---|
| `hac sync` | 在拉取 automation 的基础上,**追加**把所有 UI helper 拉进 `helpers/*.yaml`。 |
| `hac helper apply [路径]` | 把 `helpers/*.yaml` 推回 HA。幂等:实体已存在则跳过(重建/恢复场景)。 |
| `hac helper create / delete` | 已实现,保留不变。 |
| `hac sync-config` | **退役**:保留为别名,打印弃用提示并引导改用 `hac sync`。 |

## 文件格式

每类一个文件,放在**配置仓库**(`ConfigRepo`,即 ha-config,与 `automations/`
同级)的 `helpers/` 目录下,沿用现有 `input_number.yaml` "以 object_id 为 key 的
map" 风格,便于迁移与人读。(注:本设计文档存于 hac 源码仓库;`helpers/` 指的是
被同步的配置仓库,不是 hac 仓库。)

```yaml
# helpers/input_number.yaml
quan_ju_liang_du:
  name: 全局亮度
  min: 1
  max: 100
  step: 5
  unit_of_measurement: "%"
  icon: mdi:brightness-percent
  mode: slider
```

```yaml
# helpers/input_boolean.yaml
ke_ting_shou_dong:
  name: 客厅手动
  icon: mdi:gesture-tap
```

```yaml
# helpers/template_sensor.yaml
mijia_li_xian_shu:
  name: 米家掉线数
  state: "{{ integration_entities('xiaomi_home') | select('search','_cn_') | reject('search','975871650') | select('is_state','unavailable') | list | count }}"
  unit_of_measurement: 个
  icon: mdi:lan-disconnect
```

每类文件的字段集 = 该 helper 类型创建时所需的配置项(由对应 `<domain>/list` 或
config entry options 决定)。object_id(entity_id 去掉域前缀)作为 key,既是
身份标识,也是 apply 时重命名的目标。

## 数据流

### 拉取(capture / pull,并入 `hac sync`)

1. **集合型**:对 9 种 domain 各调 `<domain>/list` WS,得到每个 helper 的
   object_id + 配置;按类型写入 `helpers/<domain>.yaml`。
2. **config-entry 型(template)**:按 domain 过滤读取 config entries,从每个 entry
   的 options 取出模板定义(name/state/unit/device_class 等),写入
   `helpers/template_sensor.yaml`。
   - 实现待核实:state 模板不在实体 state 属性里(属性是渲染后的值),必须从
     config entry 的 options 读取;具体 API 字段路径在实现计划阶段确认。
3. 孤儿处理:与 automation sync 一致——HA 上已不存在的本地条目,标记告警
   (不自动删,避免误删 WIP)。

### 推回(apply / push,`hac helper apply`)

1. **集合型**:`<domain>/create` 创建(HA 按 name 生成 object_id),再
   `RenameEntityID` 改到目标 object_id。已存在则跳过。
2. **config-entry 型(template)**:走已实现的 template config flow,再按
   config_entry_id 反查 entity_id 并重命名。已存在则跳过。
3. 末尾汇报:created / skipped / failed 计数。

## 迁移

- 根目录 `input_number.yaml` → `helpers/input_number.yaml`(格式基本一致,做一次
  转换迁移)。删除根目录旧文件。
- `sync-config` 退役为告警别名。
- `template.yaml` / `templates/` 不属于本次范围(它们是未加载的 YAML 配置遗留,
  与 UI helper 是两套机制;是否清理或接入由后续单独决定)。

## 模块边界

- 新增 `internal/helpers` 包,职责单一:helper 清单的枚举、(反)序列化、与
  apply 编排。对外暴露:
  - `Capture(ws, client) (map[domain][]Helper, error)` —— 从 HA 读出全部 helper。
  - `Marshal/Unmarshal` —— helper 清单 ⇄ `helpers/*.yaml`。
  - `Apply(ws, client, helpers) Report` —— 推回 HA,返回计数报告。
- HA 交互复用 `internal/ha` 的 WSClient(集合型 list/create/update/delete、
  rename)与 Client(config-entry 的 config flow / config_entries 读取,已具备)。
- CLI 层(`cmd/hac/main.go`)负责命令分发与文件读写,薄封装。

## 错误处理

- 逐 helper 隔离:单个 list/create 失败只警告并继续,不中断整体(与 sync 拉
  automation 的容错一致)。
- apply 对每个 helper 返回明确状态(created/skipped/failed),末尾汇总。

## 测试

- 纯逻辑加 Go 单测:helper 清单的 YAML 序列化/反序列化往返、object_id 与字段
  映射、`input_number.yaml` → `helpers/input_number.yaml` 的迁移转换。
- 与 HA 的交互照项目惯例靠实盘验证(创建→state 核对→删除清理)。

## 已确认的默认取舍

- apply 首版只做"不存在才建";"已存在且配置变化则更新"(集合型有
  `<domain>/update`,config-entry 有 options flow)留作后续,不进首版。
- `hac sync` 默认连 helper 一起拉,符合"config 与 HA 实际同步"的目标。
- 不设冷门 config-entry 降级层(当前无此类实体)。
