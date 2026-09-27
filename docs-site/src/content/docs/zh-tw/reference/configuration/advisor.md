---
title: 顧問
description: OpenCodex 自有的專家諮詢 sidecar — 設定的專家模型為路由 Worker 提供建議，支援 manual 與 preflight 兩種策略。
---

顧問是一個獨立的專家模型，審閱 Worker 的任務並返回建議。OpenCodex 端到端地擁有整個諮詢過程：代理向 Worker 的回合注入合成的 `advisor` 工具，自己透過正常路由權威執行諮詢，並回注建議使原 Worker 繼續。Worker 無需委託、無需 spawn 任何東西、也不攜帶 provider 憑證。

這與子代理面（見[代理設定](/zh-tw/reference/configuration/agents/)）不同：子代理是透過 Codex 協作工具由 Worker 發起的委託。顧問是客户端完全不可見的代理側 sidecar —— 即使從不 spawn 的 Worker 也能獲得建議。

## 設定

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

| 欄位 | 類型 | 預設值 | 意義 |
| --- | --- | --- | --- |
| `enabled?` | `boolean` | `false` | 總開關。關閉時請求路徑上沒有任何 advisor 行為。 |
| `model?` | `string` | — | 專家模型。任何路由權威接受的模型字串：裸原生模型（`gpt-6-astra`）、明確 `provider/model`（`anthropic/claude-sonnet-4-6`、`xai/grok-...`）或帳戶限定的原生模型。完整支援跨 provider：Worker 與 Advisor 無需同屬一個 provider。 |
| `effort?` | `string` | `"max"` | Advisor 呼叫的推理強度（`low` 至 `ultra`）。 |
| `policy?` | `"manual" \| "preflight" \| "adaptive"` | `"manual"` | 何時諮詢顧問。 |
| `timeoutMs?` | `number` | `120000` | 回環諮詢逾時。 |

透過儀表板的 **Advisor** 頁面或 `ocx advisor status|on|off|set --model <model> --effort <effort> --policy <manual|preflight|adaptive>` 管理。

## 策略

- **`manual`** — 僅當 Worker 明確呼叫合成的 `advisor` 工具時諮詢。該呼叫由代理攔截，客户端不可見，也不會作為本地工具執行。
- **`preflight`** — OpenCodex 會在每個任務自動嘗試一次額外諮詢。失敗的諮詢不會被當作建議：任務會在失敗帳本條目過期後重試。當 Worker 產出第一份方向性證據（最新使用者訊息之後的助手工具呼叫或工具結果）時，代理會諮詢顧問並在 Worker 下一回合之前注入建議 —— 即使 Worker 從不呼叫該工具。觸發條件是確定性的、有文件記載的近似規則，不是語義級「模型卡住了」偵測器。

## Advisor 能看到什麼

**跨 provider 資料傳輸：** 當顧問 provider 與 Worker 的 provider 不同時，諮詢負載會把任務對話與工具結果傳送給第二個模型 provider。請勿對不信任該任務內容的 provider 啟用顧問。

OpenCodex 不會把自己的憑證注入負載（不含 provider API key、Authorization/OAuth 資訊、後端專用機密與環境變數）。思維鏈不會被轉移，加密的 provider 專用內容不會被解密或轉送。**任務內容通常不會做憑證脫敏**：貼進任務的憑證、或工具輸出裡列印的 token，都會按原樣轉送 —— OpenCodex 不會對會話執行 DLP。

諮詢負載完全由 Worker 模型已被允許看到的已解析會話構成：使用者任務、會話、工具呼叫及其結果、Worker 的工具目錄，以及雙方模型身份。Advisor 返回散文式建議，以可識別的包裝回注，不具備 system 權限：manual 建議以攜帶 `<opencodex_advisor>` 包裝的工具結果注入，自動 preflight 建議以攜帶 `<opencodex_advisor_preflight>` 包裝的 developer 訊息注入。思維鏈不會被轉移，加密的 provider 內容不會被解密。代理不會注入自己的憑證，但任務內容本身按原樣轉送（見上方跨 provider 提示）。

## 成本與記帳

每次諮詢都是真實的額外模型呼叫。它以 **advisor 模型**計入用量 —— 絕不併入 Worker 的 token 計數 —— 並且每次諮詢會寫一條帶觸發方式、時長、狀態和用量的 `[advisor]` 日誌行，因此 advisor 呼叫永遠可以從日誌中證明。

## 失敗行為

Advisor 失敗是 fail-open 的：已經發出的諮詢若失敗（模型不可用、設定錯誤、逾時），Worker 會收到簡短、無誤導性的「advisor 不可用」通知（preflight 為 `<opencodex_advisor_unavailable>` 訊息，manual 為錯誤工具結果）並繼續任務；只有諮詢被取消時才什麼都不注入，而計畫根本未發起諮詢（未啟用或未設定模型）時也不會送出通知。Advisor 失敗不會讓編碼請求失敗，諮詢也不會切換會話的主模型。

## PR1 限制

- 原生 OpenAI passthrough 回合（ChatGPT 池 Worker）不會獲得合成工具；advisor 支援覆蓋路由（translated）provider。preflight 諮詢適用於 run-turn 介面卡；工具不適用。
- preflight 去重帳本是行程內的；代理重啟後，進行中的任務可能再收到一次 preflight 諮詢。

## Adaptive

Adaptive 包含首次 preflight 諮詢和後續確定性升級；不偵測語義上的困惑或根因不確定性。

```sh
ocx advisor set --policy adaptive
```

The internal defaults are two consecutive explicit validation failures, three successful edits
without successful validation, or three equivalent failed validation actions. The three-edit
threshold tolerates a patch split across two tools. These are completed-tool observations, not
an analysis of hidden reasoning or test-log prose. Different diagnostics do not count as failed
validation; unknown command outcomes are not failures. A successful validation resets the signals.

Adaptive performs the same first preflight consultation as `preflight`, with no duplicate first
call. Afterwards, `edit → test PASS` and different diagnostic experiments cause **zero extra
escalations**. `edit → test FAIL → edit → test FAIL` triggers consultation and advice reaches the
same worker. After manual advice or an adaptive escalation, another escalation requires a fresh
mutation followed by validation and the rule threshold. This cooldown does not block the first
escalation after the preflight baseline. Advisor provider failures retain the existing one-minute
failure cooldown; cancellation releases the claim without a failure cooldown.

Only clients with stable task identity accumulate adaptive observations. State is process-local,
bounded to 512 tasks for 24 hours, with 2,048 result IDs and 128 pending calls per task. Saturation
skips escalation; restart, expiry or compaction can reset evidence. At most 128 recent messages
are projected per request. Provider-private tool activity and complex shell expressions are not
classified. No semantic stuck detection, multiple advisors, voting or model switching is added.
Automatic advice uses the existing developer-role injection, with the same trust limitation and
cross-provider task-content disclosure as preflight.
