---
title: Danışman
description: OpenCodex'in sahibi olduğu uzman danışma sidecar'ı — yapılandırılan uzman model yönlendirilen worker'lara tavsiye döndürür; manual ve preflight politikaları.
---

Danışman, worker'ın görevini inceleyen ve tavsiye döndüren bağımsız bir uzman modeldir. Danışmayı uçtan uca OpenCodex sahiplenir: proxy, worker'ın turuna sentetik `advisor` aracını enjekte eder, danışmayı normal yönlendirme otoritesi aracılığıyla kendisi yürütür ve tavsiyeyi geri enjekte ederek özgün worker'ın devam etmesini sağlar. Worker'ın bir şey devretmesine, spawn etmesine veya sağlayıcı kimlik bilgisi taşımasına gerek yoktur.

Bu, alt ajan yüzeyinden farklıdır (bkz. [Ajan yapılandırması](/tr/reference/configuration/agents/)): alt ajanlar, Codex'in işbirliği araçları üzerinden worker tarafından başlatılan delegasyondur. Danışman, istemcinin hiç görmediği proxy tarafı bir sidecar'dır — hiçbir şey spawn etmeyen bir worker bile tavsiye alabilir.

## Yapılandırma

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

| Alan | Tür | Varsayılan | Anlam |
| --- | --- | --- | --- |
| `enabled?` | `boolean` | `false` | Ana anahtar. Kapalıyken istek yolunda hiçbir danışman davranışı olmaz. |
| `model?` | `string` | — | Uzman model. Yönlendiricinin kabul ettiği herhangi bir model dizisi: çıplak yerel model (`gpt-6-astra`), açık `provider/model` (`anthropic/claude-sonnet-4-6`, `xai/grok-...`) veya hesap nitelemeli yerel model. Sağlayıcılar arası tam desteklenir. |
| `effort?` | `string` | `"max"` | Danışman çağrısının muhakeme düzeyi (`low`–`ultra`). |
| `policy?` | `"manual" \| "preflight"` | `"manual"` | Ne zaman danışılır. |
| `timeoutMs?` | `number` | `120000` | Loopback danışma zaman aşımı. |

Panodaki **Advisor** sayfası veya `ocx advisor status|on|off|set --model <model> --effort <effort> --policy <manual|preflight>` ile yönetin.

## Politikalar

- **`manual`** — yalnızca worker sentetik `advisor` aracını açıkça çağırdığında danışılır. Çağrı proxy tarafından yakalanır, istemciye hiç gösterilmez ve yerel araç olarak yürütülmez.
- **`preflight`** — OpenCodex ayrıca görev başına bir danışmayı otomatik olarak dener. Worker ilk yönelim kanıtını (son kullanıcı mesajından sonra bir asistan araç çağrısı VEYA araç sonucu) ürettikten sonra, worker aracı hiç çağırmasa da proxy uzmana danışır ve worker'ın bir sonraki turundan önce tavsiyeyi enjekte eder. Tetikleyici deterministik, belgelenmiş bir yaklaşımdır; anlamsal bir "takıldı" dedektörü değildir. BAŞARISIZ olan bir danışma denemesi sessizce tavsiye sayılmaz: görev, başarısızlık defteri kaydının süresi dolduğunda yeniden dener, böylece geçici bir danışman kesintisi politikayı kalıcı olarak susturmaz.

## Danışmanın gördüğü şey

**Sağlayıcılar arası veri aktarımı:** danışman sağlayıcısı worker'ın sağlayıcısından farklıysa, danışma yükü görev konuşmasını ve araç sonuçlarını ikinci bir model sağlayıcısına gönderir. Bu görev içeriğine güvenmediğiniz bir sağlayıcıda danışmanı etkinleştirmeyin.

OpenCodex kendi kimlik bilgilerini yüke asla enjekte etmez (sağlayıcı API anahtarları, Authorization/OAuth bilgileri, arka uç sırları ve ortam değişkenleri dahil değildir). Düşünce zinciri aktarılmaz, şifreli sağlayıcıya özel içerik çözülmez veya iletilmez. **Görev içeriği genellikle sırlardan arındırılmaz**: göreve yapıştırılan bir kimlik bilgisi veya bir aracın yazdırdığı token olduğu gibi iletilir — OpenCodex konuşma üzerinde DLP çalıştırmaz.

Danışma yükü, yalnızca worker modelinin zaten görmesine izin verilen ayrıştırılmış konuşmadan oluşur: kullanıcı görevi, konuşma, araç çağrıları ve sonuçları, worker'ın araç kataloğu ve iki tarafın model kimliği. Danışman düzyazı tavsiye döndürür; tanınabilir bir sarmalayıcıyla geri enjekte edilir ve sistem yetkisi yoktur: manual tavsiye `<opencodex_advisor>` sarmalayıcılı bir araç sonucu olarak, otomatik preflight tavsiyesi ise `<opencodex_advisor_preflight>` sarmalayıcılı bir developer mesajı olarak gelir. Düşünce zinciri aktarılmaz ve şifreli sağlayıcı içeriği çözülmez. Proxy kendi kimlik bilgilerini enjekte etmez, ancak görev içeriği olduğu gibi iletilir (yukarıdaki sağlayıcılar arası uyarıya bakın).

## Maliyet ve hesap

Her danışma gerçek bir ek model çağrısıdır. Worker'ın token sayılarına asla katılmaz; **danışman modeli** altında kullanımda görünür ve her danışma tetikleyici, süre, durum ve kullanımı içeren bir `[advisor]` günlük satırı yazar — böylece bir danışman çağrısı her zaman günlüklerden kanıtlanabilir.

## Hata davranışı

Danışman fail-open davranır: gönderilmiş bir danışma başarısız olursa (model kullanılamıyor, yapılandırma hatası, zaman aşımı) worker kısa ve yanıltıcı olmayan bir "danışman kullanılamıyor" bildirimi alır (preflight için `<opencodex_advisor_unavailable>` mesajı, manual için hata araç sonucu) ve göreve devam eder. Hiçbir şey yalnızca danışma iptal edildiğinde enjekte edilmez; hiç başlatılmayan yapılandırmalarda (kapalı veya model yok) bildirim de gönderilmez. Danışma hatası kodlama isteğini asla başarısız kılmaz ve oturumun ana modelini asla değiştirmez.

## PR1 sınırlamaları

- Yerel OpenAI passthrough turları (ChatGPT havuzu worker'ları) sentetik aracı almaz; danışman desteği yönlendirilen (çevrilen) sağlayıcıları kapsar. Preflight danışması run-turn bağdaştırıcılarına uygulanır; araç uygulanmaz.
- Uyarlanabilir tetikleyici yok: takılma algılama, tekrarlayan başarısızlık analizi, yükseltme katmanları, çoklu danışman veya oylama yok. Yalnızca `manual` ve `preflight`.
- Preflight tekilleştirme defteri süreç içindedir; proxy yeniden başlatıldıktan sonra devam eden bir görev bir preflight danışması daha alabilir.
