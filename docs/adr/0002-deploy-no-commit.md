# `hac deploy` 不自动 commit，由 orchestrator 接力

`hac sync` 会自动 `git commit` 输出，但 `hac deploy` 刻意不这么做。

决策：`hac deploy` 推送到 HA + `git add <file>` 后就停，由调用者（Claude Code 会话里的 Claude，或 terminal 里的用户）撰写 commit message 并运行 `git commit`。

理由：hac 是 Go 二进制，自动生成的 message 必然是模板味的 `deploy(光亮灯灭): 客厅_光亮_关灯`，而 orchestrator 看得到完整对话上下文，能写出符合本项目风格的"剩余 8 房间 暖通开关"、"主卧 暖通开关（原型对）"这种语境化 message。terminal 直接用 hac 的场景有 `--commit "<msg>"` flag 作为单次速记兜底，但不引入 `claude -p` 类的耦合，因为那既增加延迟又让"deploy 失败"的语义变模糊。
