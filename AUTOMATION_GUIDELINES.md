# 自动化配置规范

本规范用于指导 AI 生成 Home Assistant 自动化配置，确保配置结构清晰、易于维护。

## 命名规范

### 自动化名称格式
```
[房间/区域]_[触发条件]_[动作]
```

示例：
- `客厅_有人移动_开灯`
- `卧室_早上7点_开窗帘`
- `全屋_离家_关闭所有灯`

### 规则
1. **一个自动化只针对一个房间**（除非是全屋联动场景）
2. **名称使用中文**，简洁明了
3. **避免过于笼统的名称**，如"自动化1"

## 组织原则

### 按房间组织
每个房间的自动化应该独立，便于单独调试和修改。

✅ 正确：
```
客厅_有人移动_开灯.yaml
卧室_有人移动_开灯.yaml
```

❌ 错误：
```
所有房间有人移动开灯.yaml  # 太复杂，难以维护
```

### 例外：全屋场景
以下场景可以跨房间：
- 离家模式（关闭所有设备）
- 回家模式（开启欢迎灯光）
- 睡眠模式（关闭所有灯光）
- 紧急模式（全屋警报）

命名格式：`全屋_[场景]_[动作]`

## 触发器规范

### 一个自动化一个主触发器
避免在一个自动化中设置多个不相关的触发器。

✅ 正确：
```yaml
trigger:
  - platform: state
    entity_id: binary_sensor.motion_living_room
    to: "on"
```

❌ 错误：
```yaml
trigger:
  - platform: state
    entity_id: binary_sensor.motion_living_room
    to: "on"
  - platform: time
    at: "07:00:00"  # 不相关的触发器
```

### 相关触发器可以合并
如果多个触发器逻辑相关，可以合并：

✅ 正确（多个传感器触发同一动作）：
```yaml
trigger:
  - platform: state
    entity_id:
      - binary_sensor.motion_living_room_1
      - binary_sensor.motion_living_room_2
    to: "on"
```

## 条件规范

### 时间条件
使用 24 小时制，格式 `HH:MM:SS`

```yaml
condition:
  - condition: time
    after: "19:00:00"
    before: "23:59:59"
```

### 状态条件
检查设备状态时，明确指定期望值

```yaml
condition:
  - condition: state
    entity_id: input_boolean.guest_mode
    state: "off"
```

## 动作规范

### 一个自动化专注一类动作
避免在一个自动化中执行过多不相关的动作。

✅ 正确：
```yaml
action:
  - service: light.turn_on
    target:
      entity_id: light.living_room
    data:
      brightness_pct: 80
```

❌ 错误：
```yaml
action:
  - service: light.turn_on
    target:
      entity_id: light.living_room
  - service: media_player.play_media  # 不相关的动作
    target:
      entity_id: media_player.speaker
```

### 多个相关动作可以合并
如果动作逻辑相关，可以放在一起：

✅ 正确（开灯 + 调节亮度）：
```yaml
action:
  - service: light.turn_on
    target:
      entity_id: light.living_room
    data:
      brightness_pct: 80
      color_temp: 300
```

## 模式设置

### 推荐模式
- `single`: 默认，同时只运行一个实例
- `restart`: 重新触发时重启
- `queued`: 排队执行
- `parallel`: 并行执行

### 选择建议
| 场景 | 推荐模式 |
|------|----------|
| 人体感应灯 | `restart` |
| 定时任务 | `single` |
| 按钮触发 | `queued` |

## 文件组织

### 目录结构
```
ha-config/
├── automations/
│   ├── 客厅_有人移动_开灯.yaml
│   ├── 客厅_无人5分钟_关灯.yaml
│   ├── 卧室_早上7点_开窗帘.yaml
│   ├── 全屋_离家_关闭所有灯.yaml
│   └── ...
```

### 一个文件一个自动化
便于版本控制和单独修改。

## 示例

### 好的自动化示例

```yaml
# 客厅_晚间有人_开灯.yaml
alias: 客厅_晚间有人_开灯
description: 晚上7点到11点，客厅检测到人时自动开灯
mode: restart
trigger:
  - platform: state
    entity_id: binary_sensor.motion_living_room
    to: "on"
condition:
  - condition: time
    after: "19:00:00"
    before: "23:00:00"
action:
  - service: light.turn_on
    target:
      entity_id: light.living_room
    data:
      brightness_pct: 80
```

### 需要避免的模式

1. **一个自动化控制多个不相关房间** - 拆分成多个
2. **名称不清晰** - 使用规范的命名格式
3. **过多触发条件** - 保持简单
4. **混合不相关动作** - 拆分成多个自动化
