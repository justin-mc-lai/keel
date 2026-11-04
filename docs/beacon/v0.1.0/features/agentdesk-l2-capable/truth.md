---
slug: agentdesk-l2-capable
version: v0.1.0
status: frozen
language: zh
domain_required: false
ux_required: false
parser_contract: beacon-feature-package-v2
truth_source_model: feature_package_authoritative
package_maturity: draft
materials_status: current
intent_confirmed: true
canonical_refs:
  truth: docs/beacon/v0.1.0/features/agentdesk-l2-capable/truth.md
  tests: docs/beacon/v0.1.0/features/agentdesk-l2-capable/tests.md
  tasks: docs/beacon/v0.1.0/features/agentdesk-l2-capable/tasks.md
  evidence: docs/beacon/v0.1.0/features/agentdesk-l2-capable/evidence.md
---

# Requirement Truth: agentdesk-l2-capable

## 人话

L1 让客户端日常可用。L2 让它能接住一个真实任务：长命令不占住对话、改动先给人看 diff、花多少 token 有账可查、外部工具能接进来、大活能拆给子 agent。

## User Intent Snapshot

```yaml
lake_or_ocean: 湖（三湖计划中的第二湖）
language: zh
source: user_intent
utterance: "现在按顺序 L1、L2、L3 全做"
goal: "L2 能干活：后台作业、补丁预览、成本统计、MCP 客户端、子 agent"
non_goals: "Source/Trigger/Ticket 调度面（L3）；沙箱隔离；多机执行"
constraints: "后台作业必须有硬上限；MCP 连接失败不得拖垮主流程；子 agent 不得共享父会话上下文"
scope_mode: focused
```

## Open Questions

| Q ID | Question | Recommendation | Status | Answer |
|------|----------|----------------|--------|--------|
| Q1 | 一个湖装五件事是否过大？ | 五件同湖交付，按「先本机、后协议」排序 | closed | 用户要求与 L1 同样方式交付，同湖实现 |
| Q2 | 子 agent 用什么执行体？ | 进程内隔离会话，不引入外部进程 | closed | 采用进程内独立会话，复用同一 provider |

## Intent Coverage

| AC ID | Intent Source | Evidence |
|-------|---------------|----------|
| AC-201 | 参考 dsh 的 5 秒转后台与 30 分钟上限 | 现状 run_shell 超过 30 秒直接被杀，长任务不可用 |
| AC-202 | 参考 codex `review` 与 `apply` | 现状写文件前无预览，错了只能靠撤销 |
| AC-203 | 参考 opencode `stats` | 现状 usage 只在事件里闪一下，不留账 |
| AC-204 | 参考 opencode `mcp` 与 codex `mcp` | 现状无外部工具接入 |
| AC-205 | 参考 dsh 的 Executor 与 codex `agents` | 现状所有工作挤在一个会话里 |

## Acceptance Criteria

| AC ID | Criterion |
|-------|-----------|
| AC-201 | `run_shell` 超过 5 秒的前台命令转为后台作业并立即返回作业 id；提供 `job_output` 与 `job_kill` 两个工具；单个作业存活上限 30 分钟，超时被终止并标记 |
| AC-202 | `write_file` 与 `edit_file` 执行前产出 unified diff 事件，内容含目标路径与前后行；拒绝执行时不落盘 |
| AC-203 | 每次模型调用记录的 usage 与费用写入本地库，`agentdesk stats` 输出按 provider、按会话的汇总 |
| AC-204 | 支持 stdio MCP server 配置；启动时拉取工具清单并注册为可用工具；单个 server 启动失败只记录错误，不影响其他工具 |
| AC-205 | 提供 `spawn_subagent` 工具，用独立会话与独立消息历史执行一个任务并返回摘要，父会话不获得子会话的中间消息 |

## 边界（本湖明确不做）

- 不做沙箱隔离与权限下沉到子 agent
- 不做多机执行
- 不做 HTTP 与 SSE 传输的 MCP server
- 不做费用预算拦截，只记账
- 不改变三档权限的语义，子 agent 沿用父会话权限

## Parity Matrix

| Capability | Covered | Evidence |
|------------|---------|----------|
| 后台作业 | yes | AC-201 |
| 补丁预览 | yes | AC-202 |
| 成本统计 | yes | AC-203 |
| MCP 客户端 | yes | AC-204 |
| 子 agent | yes | AC-205 |
