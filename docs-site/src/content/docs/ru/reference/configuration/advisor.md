---
title: Консультант
description: Принадлежащий OpenCodex sidecar экспертных консультаций — настроенная экспертная модель консультирует маршрутизируемых воркеров; политики manual и preflight.
---

Консультант — независимая экспертная модель, которая анализирует задачу воркера и возвращает рекомендации. Консультацией владеет OpenCodex от начала до конца: прокси внедряет синтетический инструмент `advisor` в ход воркера, сам выполняет консультацию через штатный маршрутизирующий механизм и возвращает рекомендацию, чтобы исходный воркер продолжил работу. Воркеру не нужно делегировать, spawn-ить что-либо или держать провайдерские учётные данные.

Это отличается от поверхности сабагентов (см. [Конфигурацию агентов](/ru/reference/configuration/agents/)): сабагенты — это инициированная воркером делегация через инструменты совместной работы Codex. Консультант — прокси-sidecar, невидимый для клиента: даже воркер, который никогда ничего не spawn-ит, может получить совет.

## Конфигурация

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

| Поле | Тип | По умолчанию | Значение |
| --- | --- | --- | --- |
| `enabled?` | `boolean` | `false` | Главный выключатель. Выключено — ноль поведения консультанта на пути запроса. |
| `model?` | `string` | — | Экспертная модель. Любая строка модели, которую принимает роутер: «голая» нативная модель (`gpt-6-astra`), явный `provider/model` (`anthropic/claude-sonnet-4-6`, `xai/grok-...`) или модель с квалификацией аккаунта. Полная поддержка межпровайдерных сценариев. |
| `effort?` | `string` | `"max"` | Интенсивность рассуждений вызова консультанта (`low`–`ultra`). |
| `policy?` | `"manual" \| "preflight" \| "adaptive"` | `"manual"` | Когда консультировать. |
| `timeoutMs?` | `number` | `120000` | Тайм-аут loopback-консультации. |

Управляйте через страницу **Advisor** на дашборде или `ocx advisor status|on|off|set --model <model> --effort <effort> --policy <manual|preflight|adaptive>`.

## Политики

- **`manual`** — консультация только при явном вызове воркером синтетического инструмента `advisor`. Вызов перехватывается прокси, никогда не показывается клиенту и не исполняется как локальный инструмент.
- **`preflight`** — OpenCodex дополнительно автоматически пытается провести одну консультацию на задачу. Неудавшаяся консультация не считается советом: попытка повторяется после истечения записи о неудаче в реестре. После того как воркер получил первые свидетельства ориентации (вызов инструмента ассистентом ИЛИ результат инструмента после последнего сообщения пользователя), прокси консультируется с экспертом и вводит рекомендацию до следующего хода воркера — даже если воркер никогда не вызывает инструмент. Триггер — детерминированное, документированное приближение, а не семантический детектор «модель застряла».

## Что видит консультант

**Межпровайдерная передача данных:** если провайдер консультанта отличается от провайдера воркера, нагрузка консультации отправляет беседу задачи и результаты инструментов второму провайдеру модели. Не включайте консультанта у провайдера, которому не доверяете этот контент.

OpenCodex никогда не внедряет в нагрузку свои собственные учётные данные (без ключей API провайдеров, данных Authorization/OAuth, бэкенд-секретов и переменных окружения). Цепочка рассуждений не передаётся, зашифрованный контент провайдера не расшифровывается и не пересылается. **Содержимое задачи обычно не очищается от секретов**: учётные данные, вставленные в задачу, или токен, напечатанный инструментом, передаются как есть — OpenCodex не применяет DLP к беседе.

Полезная нагрузка строится исключительно из разобранной беседы, которую модель воркера и так имеет право видеть: задача пользователя, беседа, вызовы инструментов и их результаты, каталог инструментов воркера и идентификация обеих моделей. Консультант возвращает прозу-совет, вводимую как распознаваемая обёртка без системных полномочий: совет manual приходит как результат инструмента с обёрткой `<opencodex_advisor>`, а автоматический preflight — как сообщение developer с обёрткой `<opencodex_advisor_preflight>`. Цепочка рассуждений не передаётся, зашифрованный контент провайдера не расшифровывается. Прокси не внедряет свои собственные учётные данные, но содержимое задачи передаётся как есть (см. уведомление о межпровайдерной передаче выше).

## Стоимость и учёт

Каждая консультация — реальный дополнительный вызов модели. Она учитывается в использовании под **моделью консультанта** — никогда не сливается с токенами воркера — и пишет строку лога `[advisor]` с триггером, длительностью, статусом и использованием, так что вызов консультанта всегда можно доказать по логам.

## Поведение при сбоях

Консультант отказывает открыто: при сбое уже отправленной консультации (модель недоступна, ошибка настройки, тайм-аут) воркер получает короткое, не вводящее в заблуждение уведомление «консультант недоступен» (сообщение `<opencodex_advisor_unavailable>` для preflight, ошибочный результат инструмента для manual) и продолжает задачу. Ничего не внедряется только при отмене консультации; при конфигурации без запуска (отключено или нет модели) уведомление тоже не отправляется. Сбой консультанта никогда не проваливает кодинг-запрос, а консультация никогда не переключает основную модель сессии.

## Ограничения PR1

- Нативные passthrough-ходы OpenAI (воркеры пула ChatGPT) не получают синтетический инструмент; поддержка консультанта покрывает маршрутизируемых (переведённых) провайдеров. Preflight-консультация применяется к run-turn-адаптерам; инструмент — нет.
- Учётная книга дедупликации preflight живёт в процессе; после перезапуска прокси задача в работе может получить ещё одну preflight-консультацию.

## Adaptive

Adaptive включает начальную preflight-консультацию и последующую детерминированную эскалацию, без семантического определения затруднений.

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
