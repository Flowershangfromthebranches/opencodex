---
title: Conseiller
description: Le sidecar de consultation experte d'OpenCodex — un modèle expert configuré conseille les workers routés, avec les politiques manual, preflight et adaptive.
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
| `policy?` | `"manual" \| "preflight" \| "adaptive"` | `"manual"` | Quand consulter le conseiller. |
| `timeoutMs?` | `number` | `120000` | Délai de la consultation en boucle locale. |

Gérez-le via la page **Advisor** du tableau de bord ou
`ocx advisor status|on|off|set --model <model> --effort <effort> --policy <manual|preflight|adaptive>`.

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
sans autorité système : le conseil MANUAL arrive comme un résultat d'outil portant l'enveloppe
`<opencodex_advisor>`, et le conseil preflight AUTOMATIQUE comme un message developer portant
l'enveloppe `<opencodex_advisor_preflight>`. La chaîne de raisonnement n'est jamais transférée,
le contenu chiffré du fournisseur n'est jamais déchiffré, et le proxy n'injecte aucun de ses propres
identifiants, mais le contenu de tâche lui-même est transmis tel quel (voir l'avis
multi-fournisseurs ci-dessus).

## Coût et comptabilité

Chaque consultation est un véritable appel de modèle supplémentaire. Elle apparaît dans
l'utilisation sous le **modèle conseiller** — jamais fusionnée avec les tokens du worker — et
chaque consultation écrit une ligne de journal `[advisor]` avec déclencheur, durée, statut et
utilisation : un appel conseiller est toujours prouvable depuis les journaux.

## Comportement en cas d'échec

Le conseiller échoue ouvertement : une consultation déjà envoyée qui échoue (modèle indisponible, configuration erronée, délai
dépassé) donne au worker un court avis « conseiller indisponible », non trompeur (un message
`<opencodex_advisor_unavailable>` pour preflight, un résultat d'outil en erreur pour manual), et
la tâche continue ; rien n'est injecté uniquement quand la consultation est annulée, et un plan
qui ne démarre aucune consultation (désactivé ou sans modèle) n'envoie aucun avis. Un échec de consultation ne fait jamais échouer la requête de
codage, et une consultation ne change jamais le modèle principal de la session.

## Limitations PR1

- Les tours natifs OpenAI en passthrough (workers du pool ChatGPT) ne reçoivent pas l'outil
  synthétique ; le conseiller couvre les fournisseurs routés (traduits). La consultation
  preflight s'applique aux adaptateurs run-turn ; l'outil non.
- Le registre de déduplication preflight vit dans le processus ; après un redémarrage du proxy,
  une tâche en cours peut recevoir une tentative preflight de plus.

## Adaptive

Adaptive inclut la consultation preflight initiale, puis n'escalade que pour une classe limitée de non-convergence observable : un échec de validation explicite, une modification de réparation, puis un nouvel échec de la même validation. Il ne détecte ni un worker bloqué, ni une confusion sémantique, ni une cause racine incertaine.

```sh
ocx advisor set --policy adaptive
```

La seule raison automatique est `repair_failed`. Une suite de modifications sans échec préalable ne consulte pas. Deux échecs de validation sans modification entre eux ne consultent pas. Un échec de suivi sur une validation différente ne consulte pas et ouvre un nouveau cycle d'échec. Lorsqu'un résultat de validation n'a pas d'empreinte stable, la même forme peut encore consulter, et la preuve indique `same_validation=unknown` au lieu d'affirmer que les deux validations étaient identiques.

Les observations viennent d'outils terminés. Le classifieur ne lit pas la prose des journaux de test. Les commandes de diagnostic (`git diff`, `git status`, recherche, lecture de fichier) ne sont pas des validations, même si le résultat est négatif. Une validation réussie réinitialise le cycle. Les commandes shell composées — affectations d'environnement, `&&`, tubes, redirections et listes — restent non classées : une validation cachée dedans peut ne pas être observée.

Adaptive reprend la même première consultation preflight que `preflight`, sans seconde consultation dans ce tour. Après cette base, des modifications répétées, une modification suivie d'un test réussi, et des diagnostics distincts ne provoquent aucune escalade supplémentaire. Un test en échec, puis une modification, puis le même test en échec consulte, et le conseil revient au même worker. Un conseil manuel et une consultation adaptive réussie réinitialisent tous deux le cycle. Une nouvelle escalade exige un nouvel échec, une nouvelle réparation, puis un nouvel échec. Les pannes du fournisseur conservent le délai d'une minute déjà en place. Une annulation libère la réservation et ne démarre pas ce délai.

Seul un client doté d'une identité de tâche stable accumule les observations adaptive. L'état vit dans le processus, borné à 512 tâches pendant 24 heures, avec 2 048 identifiants de résultat et 128 appels en attente par tâche. La saturation saute l'escalade. Un redémarrage, l'expiration ou une compaction peut effacer les preuves. Au plus 128 messages récents sont lus par requête. Aucune détection sémantique de blocage, aucun multi-conseiller, aucun vote ni changement de modèle n'est ajouté. Le conseil automatique utilise toujours l'injection en rôle developer, avec la même limite de confiance et la même divulgation inter-fournisseurs que preflight.
