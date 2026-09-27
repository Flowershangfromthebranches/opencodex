---
title: Conseiller
description: Le sidecar de consultation experte d'OpenCodex — un modèle expert configuré conseille les workers routés, avec les politiques manual et preflight.
---

Le conseiller est un modèle expert indépendant qui examine la tâche du worker et renvoie des
conseils. La consultation appartient à OpenCodex de bout en bout : le proxy injecte un outil
synthétique `advisor` dans le tour du worker, exécute lui-même la consultation via l'autorité de
routage normale, et réinjecte les conseils pour que le worker d'origine continue. Le worker n'a
rien à déléguer, ne spawn rien et ne porte aucun identifiant de fournisseur.

Cela se distingue de la surface des sous-agents (voir
[Configuration des agents](/fr/reference/configuration/agents/)) : les sous-agents sont une
délégation initiée par le worker via les outils de collaboration de Codex. Le conseiller est un
sidecar côté proxy invisible du client — même un worker qui ne spawn jamais peut être conseillé.

## Configuration

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

| Champ | Type | Défaut | Signification |
| --- | --- | --- | --- |
| `enabled?` | `boolean` | `false` | Interrupteur principal. Désactivé : aucun comportement conseiller sur le chemin de requête. |
| `model?` | `string` | — | Le modèle expert. Toute chaîne de modèle acceptée par le routeur : modèle natif seul (`gpt-6-astra`), `provider/model` explicite (`anthropic/claude-sonnet-4-6`, `xai/grok-...`) ou modèle natif qualifié par compte. Inter-fournisseurs entièrement pris en charge. |
| `effort?` | `string` | `"max"` | Intensité de raisonnement de l'appel conseiller (`low`–`ultra`). |
| `policy?` | `"manual" \| "preflight"` | `"manual"` | Quand consulter le conseiller. |
| `timeoutMs?` | `number` | `120000` | Délai de la consultation en boucle locale. |

Gérez-le via la page **Advisor** du tableau de bord ou
`ocx advisor status|on|off|set --model <model> --effort <effort> --policy <manual|preflight>`.

## Politiques

- **`manual`** — consultation uniquement sur un appel explicite de l'outil synthétique `advisor`
  par le worker. L'appel est intercepté par le proxy, jamais montré au client, et jamais exécuté
  comme un outil local.
- **`preflight`** — OpenCodex tente en plus une consultation par tâche automatiquement. Quand le
  worker a produit sa première preuve d'orientation (un appel d'outil de l'assistant OU un
  résultat d'outil après le dernier message utilisateur), le proxy consulte l'expert et injecte
  les conseils avant le prochain tour du worker — même si le worker n'appelle jamais l'outil. Le
  déclencheur est une approximation déterministe et documentée, pas un détecteur sémantique de
  « modèle bloqué ». Une consultation tentée qui ÉCHOUE n'est pas traitée silencieusement comme
  un conseil : la tâche réessaie après l'expiration de l'entrée d'échec du registre, afin qu'une
  panne temporaire du conseiller ne rende pas la politique muette pour toujours.

## Ce que voit le conseiller

**Transfert de données entre fournisseurs :** lorsque le fournisseur du conseiller diffère de celui du worker, la charge utile de consultation envoie la conversation de tâche et les résultats d'outils à un second fournisseur de modèle. N'activez pas le conseiller avec un fournisseur auquel vous ne confiez pas ce contenu.

OpenCodex n'injecte jamais ses propres identifiants dans la charge utile (aucune clé d'API de fournisseur, aucun élément Authorization/OAuth, aucun secret backend, aucune variable d'environnement). La chaîne de raisonnement n'est jamais transférée, et le contenu chiffré propre au fournisseur n'est jamais déchiffré ni transmis. **Le contenu de tâche n'est pas généralement expurgé de secrets** : un identifiant collé dans la tâche, ou un jeton imprimé par un outil, est transmis tel quel — OpenCodex n'exécute pas de DLP sur la conversation.

La charge utile de consultation est construite exclusivement à partir de la conversation analysée
que le modèle du worker a déjà le droit de voir : la tâche utilisateur, la conversation, les
appels d'outils et leurs résultats, le catalogue d'outils du worker et l'identité des deux
modèles. Le conseiller renvoie des conseils en prose, réinjectés dans une enveloppe identifiable
`<opencodex_advisor>` sans autorité système. La chaîne de raisonnement n'est jamais transférée,
le contenu chiffré du fournisseur n'est jamais déchiffré, et aucun identifiant ni secret
d'environnement ne voyage dans la charge utile.

## Coût et comptabilité

Chaque consultation est un véritable appel de modèle supplémentaire. Elle apparaît dans
l'utilisation sous le **modèle conseiller** — jamais fusionnée avec les tokens du worker — et
chaque consultation écrit une ligne de journal `[advisor]` avec déclencheur, durée, statut et
utilisation : un appel conseiller est toujours prouvable depuis les journaux.

## Comportement en cas d'échec

Le conseiller échoue ouvertement : si le modèle expert est indisponible, mal configuré ou expire,
le worker reçoit un court contexte « conseiller indisponible », non trompeur (ou rien, pour
preflight), et poursuit la tâche. Un échec de consultation ne fait jamais échouer la requête de
codage, et une consultation ne change jamais le modèle principal de la session.

## Limitations PR1

- Les tours natifs OpenAI en passthrough (workers du pool ChatGPT) ne reçoivent pas l'outil
  synthétique ; le conseiller couvre les fournisseurs routés (traduits). La consultation
  preflight s'applique aux adaptateurs run-turn ; l'outil non.
- Pas de déclencheur adaptatif : pas de détection de blocage, d'analyse d'échecs répétés, de
  niveaux d'escalade, de conseillers multiples ni de vote. `manual` et `preflight` seulement.
- Le registre de déduplication preflight vit dans le processus ; après un redémarrage du proxy,
  une tâche en cours peut recevoir une tentative preflight de plus.
