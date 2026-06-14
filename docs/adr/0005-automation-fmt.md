# automation YAML 规范格式（autofmt）

## 背景

`hac sync` 使用 Go 标准库 `yaml.Marshal` 把 automation YAML 写回本地文件。`yaml.Marshal` 的默认行为是：键按字母序排列、缩进固定为 4 个空格、字符串按自身内容决定是否加引号。

但本仓库的 automation 文件长期以手写或历史导入的格式存在，键序遵循逻辑顺序（`alias → id → triggers → conditions → actions → mode`），缩进/引号风格也与 `yaml.Marshal` 不同。

两种格式的分歧导致每次 `hac sync` 都会整体重排这些文件，产生大量纯格式 diff。这些 diff 不携带任何语义信息，却占据 `git log` 的噪声，也让 code review 时很难区分"真改了逻辑"和"只是被 sync 重排了格式"。`hac deploy` 推送前不做格式归一，手写文件只要不经过 sync 就一直保持原始格式，进一步加剧了文件间的格式碎片化。

## 决策

引入单一规范格式 **F**，用 gofmt 式纪律在三个写入点强制执行。

### 1. 新包 `internal/autofmt`

新包对外暴露两个函数：

- `FormatAutomation(v any) ([]byte, error)` — 把一个 automation 结构体序列化为 F 格式的字节流。
- `FormatFile(path string) error` — 读取文件、反序列化、再用 `FormatAutomation` 原地覆写。

F 格式的规则：
1. **顶层键序**：`alias → id → description → triggers → conditions → actions → mode`；不在此列表的键追加到末尾，按字母序排列。
2. **缩进**：2 个空格（与 HA 官方 UI 导出一致）。
3. **引号**：只在 YAML 规范要求或避免歧义时加引号；其余纯文本值不加引号。
4. **幂等**：对已经是 F 格式的文件再次调用 `FormatAutomation`，输出字节完全相同。

`hac fmt`、`hac sync`、`hac deploy` 三处均调用同一实现，保证三个路径的输出字节一致。helpers/*.yaml 不纳入 fmt——这些文件由 HA 机器生成，格式已经确定性，且无人手写。

### 2. 新命令 `hac fmt`

```
hac fmt <file_or_dir>           # 原地格式化（单文件或整目录递归）
hac fmt validate <file_or_dir>  # 只检查，未格式化则以非零退出码退出
```

`hac fmt validate` 作为 push 前关卡：ha-config 仓库在 pre-push hook 中调用 `hac fmt validate automations/`，阻止未格式化的文件进入远端。

### 3. `hac deploy` 与 `hac sync` 自动维持格式

- `hac deploy` 在推送前对本地文件调用 `FormatFile`，确保推送的内容与格式化后一致。
- `hac sync` 写文件时调用 `FormatAutomation`，保证 sync 写回的文件已是 F 格式。

### 4. 存量文件一次性归一

对已有的所有 automation 文件执行一次 `hac fmt automations/`，以单独干净提交归一。此后 sync/deploy/手写三条路径都产生 F，长期零格式 churn。

## 影响

**正向影响**：
- sync 和 deploy 不再产生纯格式 diff；`git log` 只剩语义变更。
- 手写文件经过 `hac deploy` 自动归一，无需人工整理格式。
- pre-push hook 在仓库层面守住关卡，F 成为整个 ha-config 的统一风格。

**约束与注意事项**：
- 第一次 `hac fmt automations/` 会产生一次较大的格式提交，建议单独提交并在 PR 中注明"纯格式，无语义变更"。
- helpers/*.yaml 故意排除在 fmt 之外；如果未来有人手写 helper 文件，需另行决策是否纳入。
- `FormatFile` 会原地覆写文件，使用前确保文件在 git 版本控制下以便回滚。
