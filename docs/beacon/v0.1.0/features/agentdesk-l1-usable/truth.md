---
slug: agentdesk-l1-usable
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
  truth: docs/beacon/v0.1.0/features/agentdesk-l1-usable/truth.md
  tests: docs/beacon/v0.1.0/features/agentdesk-l1-usable/tests.md
  tasks: docs/beacon/v0.1.0/features/agentdesk-l1-usable/tasks.md
  evidence: docs/beacon/v0.1.0/features/agentdesk-l1-usable/evidence.md
---

# Requirement Truth: agentdesk-l1-usable

## 人话

当前版本只能聊天和调三个玩具工具，日常没法用。这一湖补上七件基础能力：工作区目录与路径沙箱、六个真实文件工具、`@文件` 附件、会话管理、设置持久化、AGENTS.md 注入、重试上一轮。目标是自己天天愿意打开它。

## User Intent Snapshot

```yaml
lake_or_ocean: 湖（三湖计划中的第一湖）
language: zh
source: user_intent
utterance: "现在按顺序 L1、L2、L3 全做"
goal: "L1 可用：工作区沙箱、六个文件工具、附件引用、会话管理、设置持久化、规则文件注入、重试"
non_goals: "MCP、子 agent、后台作业、补丁审阅、成本统计（属于 L2）；Source/Trigger/Ticket/看板（属于 L3）"
constraints: "文件工具不得越出工作区根目录；密钥不得写入仓库；既有 15 条 AC 的行为不能被破坏"
scope_mode: focused
```

## Open Questions

| Q ID | Question | Recommendation | Status | Answer |
|------|----------|----------------|--------|--------|
| Q1 | 三湖是否一次交付？ | 分三个 run 顺序交付，每湖独立冻结与验收 | closed | 用户要求按 L1、L2、L3 顺序全做，按湖分批验收 |
| Q2 | 附件的形态是哪一个？ | 先做 `@路径` 文本引用，文件选择器随后 | closed | 采用 `@路径`，路径受工作区限制 |

## Intent Coverage

| AC ID | Intent Source | Evidence |
|-------|---------------|----------|
| AC-101 | 参考 agy `--add-dir` 与 codex `sandbox` | 现状无工作区概念，工具可读写任意路径 |
| AC-102 | 参考 pi 的 read/bash/edit/write 四件套 | 现状仅 get_time、echo、run_shell |
| AC-103 | 参考 pi `@files` 语法 | 现状无附件能力 |
| AC-104 | 参考 opencode `session list/delete/export` | 现状只有创建与读取 |
| AC-105 | 参考截图中的 Settings 页 | 现状无设置界面，配置只能改文件 |
| AC-106 | 用户自有仓库普遍使用 AGENTS.md | 现状不读取规则文件 |
| AC-107 | 参考 codex `resume` 与 `queue` | 现状只能新发一条 |

## Acceptance Criteria

| AC ID | Criterion |
|-------|-----------|
| AC-101 | 会话绑定一个工作区根目录；文件类工具解析路径后必须落在该目录内，越界返回错误且不执行 |
| AC-102 | 提供 read_file、write_file、edit_file、list_dir、grep、fetch_url 六个工具，每个都声明 JSON Schema |
| AC-103 | 提示中的 `@相对路径` 在发送前展开为文件内容，内容与路径一起写入用户消息；路径同样受工作区限制 |
| AC-104 | 会话支持重命名、删除、按标题搜索、导出为 JSON 四种操作 |
| AC-105 | 设置可读写默认 provider、默认审批档、工作区根目录，并持久化到用户配置目录 |
| AC-106 | 工作区根目录下的 AGENTS.md 内容自动拼入本轮系统提示 |
| AC-107 | 重试会先删除上一轮用户消息之后的全部消息，再用同一条用户消息重新发送 |

## 边界（本湖明确不做）

- 不做 MCP、子 agent、后台作业、补丁审阅、成本统计
- 不做 Source/Trigger/Ticket 调度面
- 不做图片与语音输入
- 不做跨设备同步
- 不改变三档权限的含义

## Parity Matrix

| Capability | Covered | Evidence |
|------------|---------|----------|
| 工作区与路径沙箱 | yes | AC-101 |
| 文件工具集 | yes | AC-102 |
| 附件引用 | yes | AC-103 |
| 会话管理 | yes | AC-104 |
| 设置持久化 | yes | AC-105 |
| 规则文件 | yes | AC-106 |
| 重试 | yes | AC-107 |
