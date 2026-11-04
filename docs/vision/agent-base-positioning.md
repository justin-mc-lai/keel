# 个人 Agent 底座：定位与演进（2026-11）

## 一句话

一个可自托管的 Agent 底座：云端部署，插件化接入任意 ACP 编程 Agent，由一个 superagent 统一调度，自带中继供多端使用，并用 SDD 规范约束交付。

## 五条能力线

| 线 | 内容 | 与现有实现的关系 |
|---|---|---|
| **云原生** | 单二进制 + 容器；无状态进程 + 状态目录；健康检查、指标、优雅停机、备份恢复 | L6 已完成。S0 的 HTTP 适配器让它天然可部署 |
| **ACP 插件桥** | 实现 Agent Client Protocol 客户端；每个商用编程 Agent（Codex、Claude Code、Gemini CLI 等）作为一个可安装插件接入 | 未开始。现有 `provider` 是「模型 API」层，ACP 是「Agent 进程」层，两层要并存 |
| **Superagent 调度** | 一个控制面管理多个 ACP Agent：派活、并发槽位、状态回传、人工介入 | L3 的 Ticket/Slot/Panel 已经是这套骨架；缺的是把执行体从「内置循环」换成「外部 ACP 进程」 |
| **中继** | 一个中继服务，让手机、浏览器、桌面共用同一个后端会话 | S0 已把界面与传输解耦；中继是同一服务加一条长连接通道与鉴权 |
| **SDD 规范** | 需求先冻结成可验收规格，再进入实现；用 beacon-go 的包结构与门禁约束 | 未开始。当前 keel 自己就用 beacon 开发，是现成的食粮 |

## 关键分层（不要混）

```
控制面  superagent：Ticket / Slot / Panel / Policy
执行面  Executor：
          ├─ 内置循环（现在的 provider + tool loop）
          └─ ACP 插件（Codex / Claude Code / 其它）
传输面  desktop(Wails) | http+sse | relay(长连接)
状态面  SQLite + JSON 文件 + 对象存储（可选）
规范面  SDD 包：truth / tests / tasks / evidence
```

三层各自可替换，任何一层都不该知道另一层的实现细节。S0 已经把「传输面」抽出来了，ACP 插件要照同样方式抽「执行面」。

## 演进阶段

| 阶段 | 目标 | 判定标准 |
|---|---|---|
| P1（已完成） | 能对话、能调工具、能跑流程、能看板、能部署 | L1–L6 的 36 条 AC 全部通过 |
| P2 | ACP 执行面 | 至少两个商用 Agent 作为插件接入，superagent 能给它们派活并收结果 |
| P3 | 中继 | 手机与桌面同时连同一后端，会话互见 |
| P4 | SDD 落地 | 一条需求从冻结包到实现到验收全流程走通 |
| P5 | 插件生态 | 第三方能只写一个插件就接入新的 Agent 或新的工具源 |

## 明确不做

- 不做模型训练与推理服务
- 不做通用工作流市场的运营
- 不做多租户 SaaS 计费
- 不做自研编程 Agent（接入即可，不重造）

## 风险

1. **ACP 是移动靶。** 协议版本会变，商用 Agent 的实现差异大。插件层要能容忍字段缺失，不能假设所有 Agent 行为一致。
2. **两套执行面会分叉。** 内置循环与 ACP 进程如果各写一套工具与审批，会迅速腐烂。必须共用同一份 Ticket/Slot/Policy。
3. **中继是安全边界。** 一旦对外暴露，鉴权、限流、审计缺一不可；不能靠「反正是内网」。
4. **SDD 会增加摩擦。** 小改动也要走冻结包，容易让人绕过。要做「小改快车道」，否则规范会被弃用。

## 名字：Keel（2026-11-05 定稿）

旧名 AgentDesk 已全部替换为 Keel：模块路径 `github.com/justin-mc-lai/keel`、二进制 `keel`、环境变量前缀 `KEEL_`、状态目录 `~/.keel`。历史冻结包（`docs/beacon/**`）保留原名作为记录，不改写。

下列候选保留作决策依据。

## 名字候选（存档）

| 名字 | 意象 | CLI 形态 | 强项 | 弱项 |
|---|---|---|---|---|
| **Keel** | 龙骨，船的结构脊梁 | `keel serve` / `keel acp add codex` | 短、好念、直指「底座」；不与 agent/hive/conductor 撞名 | 语义偏静，弱化了「调度」 |
| **Trellis** | 让藤蔓生长的棚架 | `trellis serve` | 插件生长的意象最准 | 稍长，拼写易错 |
| **Dockhand** | 码头工，负责船只停靠 | `dockhand acp add codex` | 与「接入多个 Agent」最贴 | 偏工具感，不像平台 |
| **Switchboard** | 总机，接线与转接 | `switchboard serve` | 桥接 + 中继两层都覆盖 | 通用词，搜索与商标风险高 |
| **Aviary** | 鸟舍，群鸟共处 | `aviary serve` | 强调「群管理」 | 弱化了底座与部署 |

### 推荐

**Keel。** 理由：

1. 你要的是「底座」，Keel 就是结构本身，语义不漂。
2. 四字母，CLI 与仓库名都好用，`keel:serve`、`keel acp` 读起来不别扭。
3. 不会与当前拥挤的 agent 命名撞车（agent / hive / conductor / harness 都已占用）。
4. 后续加 ACP、中继、SDD 都不需要改名——它们都是长在龙骨上的东西。

次选 **Dockhand**：如果你希望「接入多个商用 Agent」成为第一印象，这个名字更直接。

## 未确认

- 五个名字的 npm、PyPI、商标可用性未查，只确认了本账号下 GitHub 仓库名均未占用。
- Keel 在 Kubernetes 生态有同名词（keel.sh，自动更新镜像的控制器），搜索时可能混。若要彻底避开，次选 Dockhand。
