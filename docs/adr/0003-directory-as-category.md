# `automations/` 下的子目录名等于 HA category 名

HA 有一套通过 WebSocket API 管理的 automation category/group 机制（`list_categories`、`create_category`、`assign_category`）。hac 之前没有把这套机制接到 CLI deploy 路径上——只有 MCP server 的 `assign_category` 工具调用，绕过 MCP 的部署一律落到 HA 的默认分组"其他"。结果就是仓库里出现了像 `automations/其他/客厅_光亮_关灯.yaml` 和 `automations/光亮灯灭/客厅_光亮_关灯.yaml` 同 id 并存的孤儿文件。

决策：`automations/<category>/<file>.yaml` 这条路径里的 `<category>` 是 HA category 名的唯一真理源（single source of truth）。

- `hac deploy automations/光亮灯灭/foo.yaml` 从路径推断 category = `光亮灯灭`，部署 YAML 后 WS 调用 `list_categories` 精确匹配显示名，匹配到就 `assign_category`，匹配不到默认报错退出（需要 `--create-category` flag 才会自动创建并挂载）——这避免了"typo 出来的分类静悄悄被新建"。
- `hac sync` 使用反向规则：HA 上自动化的 category 决定本地目录位置，任何本地路径与 HA category 不一致的文件被视为孤儿并删除。

这条约定刻意不引入 YAML 内的 `category:` 字段——HA 自动化 YAML schema 没这字段，加了会让本仓库的 YAML 在 HA 原生工具链里产生差异，得不偿失。
