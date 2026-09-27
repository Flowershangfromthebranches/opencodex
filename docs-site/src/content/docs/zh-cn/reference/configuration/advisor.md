---
title: 顾问
description: OpenCodex 自有的专家咨询 sidecar — 配置的专家模型为路由 Worker 提供建议，支持 manual 与 preflight 两种策略。
---

顾问是一个独立的专家模型，审阅 Worker 的任务并返回建议。OpenCodex 端到端地拥有整个咨询过程：代理向 Worker 的回合注入合成的 `advisor` 工具，自己通过正常路由权威执行咨询，并回注建议使原 Worker 继续。Worker 无需委托、无需 spawn 任何东西、也不携带 provider 凭据。

这与子代理面（见[代理配置](/zh-cn/reference/configuration/agents/)）不同：子代理是通过 Codex 协作工具由 Worker 发起的委托。顾问是客户端完全不可见的代理侧 sidecar —— 即使从不 spawn 的 Worker 也能获得建议。

## 配置

```json
{
  "advisor": {
    "enabled": true,
    "model": "gpt-6-astra",
    "effort": "max",
    "policy": "preflight"
  }
}
```

| 字段 | 类型 | 默认值 | 含义 |
| --- | --- | --- | --- |
| `enabled?` | `boolean` | `false` | 总开关。关闭时请求路径上没有任何 advisor 行为。 |
| `model?` | `string` | — | 专家模型。任何路由权威接受的模型字符串：裸原生模型（`gpt-6-astra`）、显式 `provider/model`（`anthropic/claude-sonnet-4-6`、`xai/grok-...`）或账户限定的原生模型。完整支持跨 provider：Worker 与 Advisor 无需同属一个 provider。 |
| `effort?` | `string` | `"max"` | Advisor 调用的推理强度（`low` 至 `ultra`）。 |
| `policy?` | `"manual" \| "preflight"` | `"manual"` | 何时咨询顾问。 |
| `timeoutMs?` | `number` | `120000` | 回环咨询超时。 |

通过仪表盘的 **Advisor** 页面或 `ocx advisor status|on|off|set --model <model> --effort <effort> --policy <manual|preflight>` 管理。

## 策略

- **`manual`** — 仅当 Worker 显式调用合成的 `advisor` 工具时咨询。该调用由代理拦截，客户端不可见，也不会作为本地工具执行。
- **`preflight`** — OpenCodex 会在每个任务自动尝试一次额外咨询。失败的咨询不会被当作建议：任务会在失败账本条目过期后重试。当 Worker 产出第一份方向性证据（最新用户消息之后的助手工具调用或工具结果）时，代理会咨询顾问并在 Worker 下一回合之前注入建议 —— 即使 Worker 从不调用该工具。触发条件是确定性的、有文档的近似规则，不是语义级"模型卡住了"检测器。

## Advisor 能看到什么

**跨 provider 数据传输：** 当顾问 provider 与 Worker 的 provider 不同时，咨询负载会把任务对话与工具结果发送给第二个模型 provider。请勿对不信任该任务内容的 provider 启用顾问。

OpenCodex 不会把自己的凭据注入负载（不含 provider API key、Authorization/OAuth 信息、后端专用机密与环境变量）。思维链不会被转移，加密的 provider 专用内容不会被解密或转发。**任务内容通常不会做凭据脱敏**：粘贴进任务里的凭据、或工具输出里打印的 token，都会按原样转发 —— OpenCodex 不会对会话执行 DLP。

咨询负载完全由 Worker 模型已被允许看到的已解析会话构成：用户任务、会话、工具调用及其结果、Worker 的工具目录，以及双方模型身份。Advisor 返回散文式建议，以可识别的 `<opencodex_advisor>` 包装回注，不具备 system 权限。思维链不会被转移，加密的 provider 内容不会被解密。代理不会注入自己的凭据，但任务内容本身按原样转发（见上方跨 provider 提示）。

## 成本与记账

每次咨询都是真实的额外模型调用。它以 **advisor 模型**计入用量 —— 绝不并入 Worker 的 token 计数 —— 并且每次咨询会写一条带触发方式、时长、状态和用量的 `[advisor]` 日志行，因此 advisor 调用永远可以从日志中证明。

## 失败行为

Advisor 失败是 fail-open 的：如果专家模型不可用、配置错误或超时，已经发出的咨询若失败（模型不可用、配置错误、超时），Worker 会收到简短、无误导性的"advisor 不可用"通知（preflight 为 `<opencodex_advisor_unavailable>` 消息，manual 为错误工具结果）并继续任务；只有咨询被取消时才什么都不注入，而计划根本未发起咨询（未启用或未配置模型）时也不会发送通知。Advisor 失败不会让编码请求失败，咨询也不会切换会话的主模型。

## PR1 限制

- 原生 OpenAI passthrough 回合（ChatGPT 池 Worker）不会获得合成工具；advisor 支持覆盖路由（translated）provider。preflight 咨询适用于 run-turn 适配器；工具不适用。
- 无自适应触发：没有卡住检测、重复失败分析、升级分层、多 Advisor 或投票。`manual` 与 `preflight` 是仅有的策略。
- preflight 去重账本是进程内的；代理重启后，进行中的任务可能再收到一次 preflight 咨询。
