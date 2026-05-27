# hac 只保留 CLI，移除 MCP server

hac 最初同时提供 CLI 和 MCP server，后者面向 Windsurf / Cursor 等 AI IDE。迁移到 Claude Code 后，Claude 通过内置 Bash 工具调用 shell 命令已经非常顺畅，MCP 这层对读操作只是重复表面，且 `~/.codeium/windsurf/mcp_config.json` 这种 Windsurf 专属配置路径不再适用。

决策：hac 是纯 CLI 工具。删除 `internal/mcp/` 整个目录和 `mcp` 子命令。原本走 MCP 二步确认流的写操作（`pending/` 暂存、`confirm_automation`、`cancel_pending`）合并成 `hac deploy` 单步——Claude Code 自己的 Edit/Write 审批弹窗 + git 历史已经提供了双层 review，无需再加第三层。
