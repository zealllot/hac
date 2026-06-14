# 设计:hac automation 格式化(fmt)

日期:2026-06-14
状态:已确认设计

## 背景

`hac sync` 用 `yaml.Marshal` 把 HA 拉来的 automation 写回本地(字母键序 + yaml 默认缩进/引号),而本地文件是手写/历史格式(逻辑键序)。两种格式天生不同,导致**每次 sync 都把 deploy/手写过的文件整体重排**,产生大量纯格式 diff(本次一口气重排了 66 个文件)。

## 决定:gofmt 式格式化纪律

引入单一规范格式 **F**,所有来源都收敛到 F,从根上消除格式分歧。

### F 的定义
- 顶层键顺序按 CLAUDE.md 约定:`alias → id → description → triggers → conditions → actions → mode`,其余未列出的键追加在后、按字母序。
- 缩进、引号风格:由统一的编码器固定(确定性),不追求匹配旧手写风格——一次性归一后即为新的房子格式。
- 必须**幂等**:`F(F(x)) == F(x)`。

### 唯一格式真相:`FormatAutomation`
抽出共享函数 `FormatAutomation(config map[string]any) ([]byte, error)`,放在新包 `internal/autofmt`。`hac fmt`、`hac sync`、`hac deploy` 全部调它,保证三处输出字节一致。

### 命令
- `hac fmt <path>` —— 原地把文件改写成 F(update;默认行为)。path 可为文件或目录(目录则递归 `*.yaml`)。
- `hac fmt validate <path>` —— 只检查是否已是 F,有未格式化文件则列出并以非零退出(push 前的关卡)。

### 集成到既有命令
- `hac sync`:写文件改用 `FormatAutomation`(替换 `internal/syncer/syncer.go` 的 `yaml.Marshal`)。
- `hac deploy <file>`:推送前先对该本地文件做一次 `FormatAutomation` 原地格式化,使"AI 手写后自动 update"无需手动。

### 一次性归一
对存量文件跑一次 `hac fmt automations/`,把全部 automation 归到 F,作为**单独一个干净提交**(类似首次 gofmt)。归一后 sync/deploy 都产 F,长期零格式 churn。

### pre-push 关卡
给 ha-config 仓库加 `pre-push` git hook:push 前自动 `hac fmt validate automations/`,未格式化则阻止 push。

## 范围外
- `helpers/*.yaml` 不纳入 fmt:它们由 `hac sync` 的 `WriteManifest` 机器生成、已是确定性格式、无人手写,无格式纪律需求。
- 不动 automation 的语义,只动序列化排版。

## 模块边界
- 新增 `internal/autofmt`:`FormatAutomation(config) ([]byte, error)`(核心,纯逻辑)、`FormatFile(path) (changed bool, err)`、`IsFormatted(path) (ok bool, err)`。
- `internal/syncer` 改调 `autofmt.FormatAutomation`。
- `cmd/hac`:新增 `hac fmt` 分发与 `runFmt`;`deployOne` 调 `autofmt.FormatFile`。

## 关键不变量
- 幂等:存量归一后,无 HA 侧语义变化时再 sync 产出与磁盘字节一致 → 零 diff。
- 单一真相:三处共用 `FormatAutomation`,不会再分歧。
