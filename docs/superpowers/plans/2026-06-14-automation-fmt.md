# hac automation 格式化(fmt)Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development。逐任务实现,checkbox 跟踪。

**Goal:** 引入单一规范格式 F 与 `hac fmt`,让 sync/deploy/fmt 共用一个序列化函数,从根上消除 `hac sync` 反复重排既有 automation 的格式 churn。

**Architecture:** 新包 `internal/autofmt` 提供 `FormatAutomation(map)→[]byte`(规范键序 + 确定性编码,幂等)及文件级 `FormatFile`/`IsFormatted`。`hac sync` 和 `hac deploy` 改调它;新增 `hac fmt [validate] <path>`。存量文件一次性 `hac fmt` 归一,ha-config 加 pre-push validate hook。

**Tech Stack:** Go,`gopkg.in/yaml.v3`(用 `yaml.Node` 控制键序 + `Encoder.SetIndent` 固定缩进)。

**测试约定:** `internal/autofmt` 纯逻辑写 Go 单测(键序、幂等、数据保真);CLI 与 sync/deploy 靠实盘验证。

---

## File Structure

**新建:**
- `internal/autofmt/autofmt.go` — `FormatAutomation`、`orderedMapNode`、`FormatFile`、`IsFormatted`。
- `internal/autofmt/autofmt_test.go` — 单测。
- `docs/adr/0005-automation-fmt.md` — 决策记录。

**修改:**
- `internal/syncer/syncer.go` — 写文件改用 `autofmt.FormatAutomation`。
- `cmd/hac/main.go` — 新增 `fmt` 分发 + `runFmt` + `collectYAML`;`deployOne` 调 `autofmt.FormatFile`;`printUsage` 补 `hac fmt`。
- `README.md` — 命令文档。

**ha-config 仓库(代码之外的操作):**
- 一次性 `hac fmt automations/` 归一 + 干净提交。
- `.githooks/pre-push` + 配置 `core.hooksPath`(或 `.git/hooks/pre-push`)跑 `hac fmt validate automations/`。

---

## Task 1: autofmt 核心 — FormatAutomation

**Files:** Create `internal/autofmt/autofmt.go`, `internal/autofmt/autofmt_test.go`

- [ ] **Step 1: 写失败测试** `internal/autofmt/autofmt_test.go`

```go
package autofmt

import (
	"bytes"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestFormatAutomationKeyOrder(t *testing.T) {
	cfg := map[string]any{
		"mode":       "single",
		"actions":    []any{map[string]any{"action": "light.turn_on"}},
		"id":         "1700000000000",
		"alias":      "测试",
		"triggers":   []any{map[string]any{"platform": "state"}},
		"conditions": []any{},
	}
	out, err := FormatAutomation(cfg)
	if err != nil {
		t.Fatalf("format: %v", err)
	}
	s := string(out)
	// 顶层键必须按 alias→id→triggers→conditions→actions→mode 出现
	order := []string{"alias:", "id:", "triggers:", "conditions:", "actions:", "mode:"}
	last := -1
	for _, k := range order {
		idx := strings.Index(s, "\n"+k)
		if k == "alias:" {
			idx = strings.Index(s, k) // 第一行无前导换行
		}
		if idx < 0 {
			t.Fatalf("key %q not found in:\n%s", k, s)
		}
		if idx < last {
			t.Errorf("key %q out of order in:\n%s", k, s)
		}
		last = idx
	}
}

func TestFormatAutomationIdempotent(t *testing.T) {
	cfg := map[string]any{
		"alias": "测试", "id": "1700000000000", "mode": "restart",
		"triggers": []any{map[string]any{"platform": "state", "entity_id": "x.y"}},
		"actions":  []any{map[string]any{"action": "light.turn_on", "data": map[string]any{"brightness_pct": 100}}},
	}
	once, err := FormatAutomation(cfg)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	var reparsed map[string]any
	if err := yaml.Unmarshal(once, &reparsed); err != nil {
		t.Fatalf("reparse: %v", err)
	}
	twice, err := FormatAutomation(reparsed)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if !bytes.Equal(once, twice) {
		t.Errorf("not idempotent:\n--- once ---\n%s\n--- twice ---\n%s", once, twice)
	}
}

func TestFormatAutomationPreservesIDAsString(t *testing.T) {
	out, err := FormatAutomation(map[string]any{"alias": "a", "id": "1700000000000"})
	if err != nil {
		t.Fatalf("format: %v", err)
	}
	// id 必须带引号,避免被当成数字
	if !strings.Contains(string(out), `"1700000000000"`) && !strings.Contains(string(out), `'1700000000000'`) {
		t.Errorf("id should be quoted to stay a string:\n%s", out)
	}
}
```

- [ ] **Step 2: 跑测试确认失败**

Run: `cd /Users/zealllot/go/src/github.com/zealllot/hac && go test ./internal/autofmt/`
Expected: FAIL,`undefined: FormatAutomation`。

- [ ] **Step 3: 实现** `internal/autofmt/autofmt.go`

```go
// Package autofmt provides the single canonical serialization (format "F") for
// Home Assistant automation YAML. hac fmt, hac sync, and hac deploy all route
// through FormatAutomation so files never diverge in formatting. F orders the
// top-level keys the way automations are authored (see CLAUDE.md) and is
// idempotent.
package autofmt

import (
	"bytes"
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"
)

// automationKeyOrder is the canonical top-level key order. Keys not listed are
// appended afterwards in alphabetical order.
var automationKeyOrder = []string{
	"alias", "id", "description", "triggers", "trigger",
	"conditions", "condition", "actions", "action", "mode", "max", "variables",
}

// FormatAutomation serializes an automation config to canonical YAML (format F).
func FormatAutomation(config map[string]any) ([]byte, error) {
	node, err := orderedMapNode(config)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(node); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// orderedMapNode builds a YAML mapping node whose top-level keys follow
// automationKeyOrder (then alphabetical for the rest). Values are encoded with
// yaml's defaults (nested maps end up alphabetical, which is fine and stable).
func orderedMapNode(m map[string]any) (*yaml.Node, error) {
	rank := make(map[string]int, len(automationKeyOrder))
	for i, k := range automationKeyOrder {
		rank[k] = i
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		ri, oki := rank[keys[i]]
		rj, okj := rank[keys[j]]
		if oki && okj {
			return ri < rj
		}
		if oki != okj {
			return oki // ranked keys come before unranked
		}
		return keys[i] < keys[j] // both unranked: alphabetical
	})
	out := &yaml.Node{Kind: yaml.MappingNode}
	for _, k := range keys {
		kn := &yaml.Node{Kind: yaml.ScalarNode, Value: k}
		vn := &yaml.Node{}
		if err := vn.Encode(m[k]); err != nil {
			return nil, fmt.Errorf("encode key %q: %w", k, err)
		}
		out.Content = append(out.Content, kn, vn)
	}
	return out, nil
}

// FormatFile reads, canonicalizes, and rewrites a file in place. Returns whether
// the file content changed. A file already in F is left byte-identical.
func FormatFile(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	formatted, err := formatBytes(data, path)
	if err != nil {
		return false, err
	}
	if bytes.Equal(data, formatted) {
		return false, nil
	}
	if err := os.WriteFile(path, formatted, 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// IsFormatted reports whether a file is already in canonical form F.
func IsFormatted(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	formatted, err := formatBytes(data, path)
	if err != nil {
		return false, err
	}
	return bytes.Equal(data, formatted), nil
}

func formatBytes(data []byte, path string) ([]byte, error) {
	var config map[string]any
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return FormatAutomation(config)
}
```

- [ ] **Step 4: 跑测试确认通过**

Run: `go test ./internal/autofmt/`
Expected: PASS（3 个测试）。

- [ ] **Step 5: Commit**

```bash
cd /Users/zealllot/go/src/github.com/zealllot/hac
git add internal/autofmt/
git commit -m "feat(autofmt): automation 规范格式 F 与文件级格式化"
```

---

## Task 2: `hac fmt [validate] <path>` 命令

**Files:** Modify `cmd/hac/main.go`

- [ ] **Step 1: 加分发**

先 `Read` `cmd/hac/main.go` 的 `main()`,在它对子命令 if 分发区(`if sub == "helper" {...}` 附近)加:
```go
	if sub == "fmt" {
		runFmt(os.Args[2:])
		return
	}
```

- [ ] **Step 2: 实现 `runFmt` 和 `collectYAML`**(放在 `runHelper` 系列函数附近)

```go
func runFmt(args []string) {
	validate := false
	rest := args
	if len(rest) > 0 && rest[0] == "validate" {
		validate = true
		rest = rest[1:]
	}
	if len(rest) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: hac fmt [validate] <file_or_dir>")
		os.Exit(1)
	}

	files, err := collectYAML(rest[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if validate {
		var bad []string
		for _, f := range files {
			ok, err := autofmt.IsFormatted(f)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
				continue
			}
			if !ok {
				bad = append(bad, f)
			}
		}
		for _, f := range bad {
			fmt.Fprintf(os.Stderr, "not formatted: %s\n", f)
		}
		if len(bad) > 0 {
			fmt.Fprintf(os.Stderr, "%d file(s) need formatting; run `hac fmt %s`\n", len(bad), rest[0])
			os.Exit(1)
		}
		fmt.Printf("all %d file(s) formatted\n", len(files))
		return
	}

	var changed int
	for _, f := range files {
		ch, err := autofmt.FormatFile(f)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
			continue
		}
		if ch {
			changed++
			fmt.Printf("formatted %s\n", f)
		}
	}
	fmt.Printf("%d/%d file(s) changed\n", changed, len(files))
}

// collectYAML returns the .yaml file(s) at path: the file itself, or all
// *.yaml under it recursively if it is a directory.
func collectYAML(path string) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return []string{path}, nil
	}
	var files []string
	err = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(p, ".yaml") {
			files = append(files, p)
		}
		return nil
	})
	return files, err
}
```

- [ ] **Step 3: 加 import**

确保 `cmd/hac/main.go` import 含 `"io/fs"` 和 `"github.com/zealllot/hac/internal/autofmt"`(`os`/`path/filepath`/`strings`/`fmt` 已有)。

- [ ] **Step 4: 编译并安装**

Run: `cd /Users/zealllot/go/src/github.com/zealllot/hac && go build ./... && go install ./cmd/hac`
Expected: 无错误。（忽略 LSP GOROOT 误报。）

- [ ] **Step 5: 实盘验证(临时文件)**

```bash
mkdir -p /tmp/fmttest
printf 'mode: single\nid: "123"\nalias: T\ntriggers: []\n' > /tmp/fmttest/a.yaml
hac fmt validate /tmp/fmttest; echo "validate exit=$?"   # 期望非0(未格式化)
hac fmt /tmp/fmttest                                       # 期望 formatted .../a.yaml
cat /tmp/fmttest/a.yaml                                     # 期望 alias 在 id 之前
hac fmt validate /tmp/fmttest; echo "validate exit=$?"     # 期望 0(已格式化)
hac fmt /tmp/fmttest                                       # 期望 0/1 changed(幂等,不再变)
rm -rf /tmp/fmttest
```
把实际输出贴进报告。

- [ ] **Step 6: Commit**

```bash
git add cmd/hac/main.go
git commit -m "feat(fmt): hac fmt [validate] <path> 命令"
```

---

## Task 3: `hac sync` 改用 FormatAutomation

**Files:** Modify `internal/syncer/syncer.go`

- [ ] **Step 1: 替换序列化**

在 `internal/syncer/syncer.go` 找到(约 112 行):
```go
		data, err := yaml.Marshal(config)
```
替换为:
```go
		data, err := autofmt.FormatAutomation(config)
```

- [ ] **Step 2: 调整 import**

import 块加 `"github.com/zealllot/hac/internal/autofmt"`。若 `yaml` 在该文件别处仍被使用(如 `yaml.Unmarshal`)则保留 `gopkg.in/yaml.v3`;若替换后 `yaml` 不再被使用,删除该 import(以 `go build` 报错为准)。

- [ ] **Step 3: 编译 + 测试**

Run: `cd /Users/zealllot/go/src/github.com/zealllot/hac && go build ./... && go test ./...`
Expected: 无错误,测试通过(syncer 既有测试不应因键序变化而失败;若 syncer_test 里断言了具体文件内容/键序,按新规范格式更新断言)。

- [ ] **Step 4: Commit**

```bash
git add internal/syncer/syncer.go
git commit -m "feat(sync): sync 写文件改用 autofmt.FormatAutomation"
```

---

## Task 4: `hac deploy` 调 FormatFile

**Files:** Modify `cmd/hac/main.go`

- [ ] **Step 1: 在 deployOne 里格式化本地文件**

先 `Read` `deployOne(client, ws, file, opts)`(约 696 行)。在它**开头、读取/解析文件之前**插入:
```go
	// Keep the on-disk file in canonical format F so a later `hac sync` won't
	// reformat it (single source of truth: autofmt.FormatAutomation).
	if _, err := autofmt.FormatFile(file); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: fmt %s before deploy: %v\n", file, err)
	}
```
（autofmt 已在 Task 2 import;若该文件还没 import 则补上。)

- [ ] **Step 2: 编译并安装**

Run: `cd /Users/zealllot/go/src/github.com/zealllot/hac && go build ./... && go install ./cmd/hac`
Expected: 无错误。

- [ ] **Step 3: 实盘验证(不真改 HA 配置:用一个已存在 automation 文件验证格式化副作用)**

为避免改动生产 automation,这里只验证"deploy 会把文件格式化"。用一个临时复制:
```bash
cd /Users/zealllot/go/src/github.com/zealllot/ha-config
# 取一个文件复制成乱序版本,确认 deploy 前置 fmt 生效(不真 deploy,改为直接调 hac fmt 验证等价路径已在 Task2 覆盖)
echo "deploy 的 fmt 前置与 hac fmt 共用 FormatFile,已在 Task 2 实盘覆盖;此处仅确认 build 通过即可。"
```
Expected: build 通过即可(deploy 真路径会改 HA,留待用户在真实 deploy 时自然验证)。

- [ ] **Step 4: Commit**

```bash
cd /Users/zealllot/go/src/github.com/zealllot/hac
git add cmd/hac/main.go
git commit -m "feat(deploy): deploy 前置 autofmt.FormatFile 归一本地文件"
```

---

## Task 5: 文档(ADR + README)

**Files:** Create `docs/adr/0005-automation-fmt.md`, Modify `README.md`

- [ ] **Step 1: 写 ADR**

`docs/adr/0005-automation-fmt.md`,参照 0001-0004 格式(Context/Decision/Consequences,中文),记录:churn 根因(sync 与手写格式分歧)、决定(单一格式 F + `hac fmt` + sync/deploy 共用 `FormatAutomation`)、F 的键序约定、一次性归一、pre-push validate、helpers/ 排除在外。

- [ ] **Step 2: 更新 README**

命令清单加 `hac fmt [validate] <path>`;说明 sync/deploy 会维持规范格式;提到 pre-push validate 约定。保持原风格。

- [ ] **Step 3: Commit**

```bash
git add docs/adr/0005-automation-fmt.md README.md
git commit -m "docs: automation fmt 的 ADR 与 README"
```

---

## Task 6:(ha-config 仓库)一次性归一 + pre-push hook

> 在 `hac` 合并到 main 并 `go install` 之后执行。这一步操作 ha-config 仓库,不改 hac 代码。

- [ ] **Step 1: 归一存量 automation**

```bash
cd /Users/zealllot/go/src/github.com/zealllot/ha-config
hac fmt automations/
```
Expected: 打印若干 `formatted ...` 与 `N/M file(s) changed`。

- [ ] **Step 2: 单独提交归一**

```bash
git add automations/
git commit -m "format: 用 hac fmt 归一全部 automation 到规范格式"
```

- [ ] **Step 3: 加 pre-push hook**

```bash
cd /Users/zealllot/go/src/github.com/zealllot/ha-config
mkdir -p .githooks
cat > .githooks/pre-push <<'SH'
#!/bin/sh
# 阻止未格式化的 automation 被 push;运行 hac fmt validate。
if ! hac fmt validate automations/; then
	echo "✗ automation 未格式化,先跑 \`hac fmt automations/\` 再 push" >&2
	exit 1
fi
SH
chmod +x .githooks/pre-push
git config core.hooksPath .githooks
git add .githooks/pre-push
git commit -m "chore: 加 pre-push hook,push 前校验 automation 格式"
```
Expected: hook 可执行;`core.hooksPath` 指向 `.githooks`。

- [ ] **Step 4: 验证 hook + 干净 re-sync**

```bash
cd /Users/zealllot/go/src/github.com/zealllot/ha-config
hac fmt validate automations/; echo "validate exit=$?"   # 期望 0
git status --short                                          # 归一+hook 提交后应干净
hac sync                                                    # 关键:归一后再 sync
git status --short                                          # 期望 automations/ 无新改动(零 churn);可能仅 helpers/ 因 HA 实时状态微变
```
Expected:`hac sync` 后 `automations/` 不再出现格式重排 diff(churn 消除)。把 `git status` 结果贴进报告。

---

## Self-Review 备忘(已核对)

- **Spec 覆盖**:F 定义+共享函数→Task 1;`hac fmt [validate]`→Task 2;sync 共用→Task 3;deploy 共用→Task 4;一次性归一+pre-push→Task 6;文档→Task 5;helpers/ 排除→不纳入任何 fmt 路径。
- **类型一致**:`FormatAutomation(map[string]any)([]byte,error)`、`FormatFile(string)(bool,error)`、`IsFormatted(string)(bool,error)` 在 Task 1 定签名,Task 2/3/4 按此调用。
- **幂等保证**:Task 1 有幂等单测;Task 6 Step 4 实盘验证归一后 sync 零 churn——这是整个特性的验收点。
- **占位符**:无 TODO;改代码步骤均含完整代码。Task 3/4 对既有文件改动点明确给出定位与 import 处理规则。
