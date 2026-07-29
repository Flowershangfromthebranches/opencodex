package codex

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/lidge-jun/opencodex-go/internal/combos"
)

// MainCodexAccountNamespaceTarget is the config-only sentinel for the Desktop
// account. Deliberately outside the pool-account id grammar so it can never be
// confused with a real account id.
const MainCodexAccountNamespaceTarget = "@main"

var codexAccountIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// reservedCodexAccountIDs are names with special meaning on ordinary JavaScript
// objects. Credentials are persisted in JSON maps, so admitting one of these as
// a user-controlled key is a prototype-pollution vector on the oracle side and
// a confusing collision here.
var reservedCodexAccountIDs = map[string]bool{"__proto__": true, "prototype": true, "constructor": true}

// reservedNamespaceKeys can never be claimed as an account selector: two are
// routing namespaces that already mean something, and the rest are the reserved
// object keys above.
var reservedNamespaceKeys = map[string]bool{
	"__proto__": true, "prototype": true, "constructor": true,
	combos.Namespace: true, "openai": true,
}

const publicAccountSelectorMaxAttempts = 16

// IsValidCodexAccountID reports whether an id may be persisted as a pool
// account (oracle: src/codex/account-id.ts:15).
func IsValidCodexAccountID(accountID string) bool {
	return accountID != MainCodexAccountID &&
		codexAccountIDPattern.MatchString(accountID) &&
		!reservedCodexAccountIDs[strings.ToLower(accountID)]
}

// CodexProviderNamespaceKey normalizes a provider id for namespace comparison.
// Case-insensitive because these collide at admission boundaries, where a
// difference in case would let two things claim the same routing prefix.
func CodexProviderNamespaceKey(value string) string { return strings.ToLower(value) }

// HasCodexAccountNamespace reports whether the map defines this namespace.
func HasCodexAccountNamespace(namespaces map[string]string, namespace string) bool {
	_, found := namespaces[namespace]
	return found
}

// CodexAccountNamespaceProviderCollisionError refuses a provider name that
// would shadow a configured account namespace.
//
// Both live in the same selector space: `alpha/model` has to mean one thing, so
// a provider and an account namespace cannot share a name. Admitting the
// collision would make routing depend on lookup order.
func CodexAccountNamespaceProviderCollisionError(namespaces map[string]string, providerName string) string {
	normalized := CodexProviderNamespaceKey(providerName)
	for namespace := range namespaces {
		if CodexProviderNamespaceKey(namespace) == normalized {
			return "provider name must not collide with a configured Codex account namespace"
		}
	}
	return ""
}

// CodexAccountIDNamespaceCollisionError refuses an account id that equals an
// existing namespace key, which would make the selector ambiguous.
func CodexAccountIDNamespaceCollisionError(namespaces map[string]string, accountID string) string {
	if HasCodexAccountNamespace(namespaces, accountID) {
		return "account id must not collide with a configured Codex account namespace"
	}
	return ""
}

// CodexAccountNamespaceForModel returns the namespace a routed model id selects,
// or "" when the prefix is not a configured namespace.
func CodexAccountNamespaceForModel(namespaces map[string]string, modelID string) string {
	slash := strings.Index(modelID, "/")
	if slash <= 0 {
		return ""
	}
	namespace := modelID[:slash]
	if HasCodexAccountNamespace(namespaces, namespace) {
		return namespace
	}
	return ""
}

// IsMainCodexAccountTarget accepts both spellings of the Desktop account: the
// config sentinel and the internal id.
func IsMainCodexAccountTarget(accountID string) bool {
	return accountID == MainCodexAccountNamespaceTarget || accountID == MainCodexAccountID
}

// CodexAccountNamespaceEntries returns the namespace map with Desktop targets
// normalized to the internal id, so callers never have to know both spellings.
func CodexAccountNamespaceEntries(namespaces map[string]string) map[string]string {
	entries := make(map[string]string, len(namespaces))
	for namespace, accountID := range namespaces {
		if IsMainCodexAccountTarget(accountID) {
			entries[namespace] = MainCodexAccountID
			continue
		}
		entries[namespace] = accountID
	}
	return entries
}

// NamespaceConfig is what namespace generation needs from the config, kept
// narrow so this package does not depend on the whole config type.
type NamespaceConfig struct {
	Accounts     []CodexAccount
	ProviderIDs  []string
	ComboAliases []string
}

// DefaultCodexAccountNamespaces builds the initial selector map.
//
// Public selectors are never derived from an account id or alias: those are the
// PRIVATE identifiers, and exposing one as a routing prefix leaks which account
// is which. A row that predates persisted log labels gets a freshly allocated
// selector rather than the id-derived fallback, which exists only to redact old
// logs.
func DefaultCodexAccountNamespaces(config NamespaceConfig) map[string]string {
	namespaces := map[string]string{}
	used := occupiedNamespaces(config)
	privateCandidates := map[string]bool{}
	for _, account := range config.Accounts {
		if account.ID == MainCodexAccountID {
			continue
		}
		privateCandidates[account.ID] = true
		privateCandidates[FallbackCodexAccountLogLabel(account.ID)] = true
	}
	for candidate := range privateCandidates {
		used[candidate] = true
	}

	namespaces[claimNamespace("main", used)] = MainCodexAccountNamespaceTarget
	for _, account := range config.Accounts {
		if account.IsMain || !IsValidCodexAccountID(account.ID) {
			continue
		}
		selector := defaultPublicAccountSelector(account, privateCandidates)
		namespaces[claimNamespace(selector, used)] = account.ID
	}
	return namespaces
}

// AppendDefaultCodexAccountNamespace adds one account to an existing generated
// map, leaving explicit entries untouched. Reports whether the map changed.
func AppendDefaultCodexAccountNamespace(namespaces map[string]string, config NamespaceConfig, account CodexAccount) bool {
	if account.IsMain || !IsValidCodexAccountID(account.ID) || len(namespaces) == 0 {
		return false
	}
	if CodexAccountIDNamespaceCollisionError(namespaces, account.ID) != "" {
		return false
	}
	for _, existing := range namespaces {
		if existing == account.ID {
			return false
		}
	}
	used := occupiedNamespaces(config)
	for namespace := range namespaces {
		used[namespace] = true
	}
	privateCandidates := map[string]bool{}
	addPrivate := func(id string) {
		if id == "" || id == MainCodexAccountID {
			return
		}
		privateCandidates[id] = true
		privateCandidates[FallbackCodexAccountLogLabel(id)] = true
	}
	for _, existing := range config.Accounts {
		addPrivate(existing.ID)
	}
	for _, target := range namespaces {
		addPrivate(target)
	}
	addPrivate(account.ID)
	for candidate := range privateCandidates {
		used[candidate] = true
	}
	namespaces[claimNamespace(defaultPublicAccountSelector(account, privateCandidates), used)] = account.ID
	return true
}

func occupiedNamespaces(config NamespaceConfig) map[string]bool {
	used := map[string]bool{}
	for key := range reservedNamespaceKeys {
		used[key] = true
	}
	for _, provider := range config.ProviderIDs {
		used[CodexProviderNamespaceKey(provider)] = true
	}
	for _, alias := range config.ComboAliases {
		trimmed := strings.TrimSpace(alias)
		if slash := strings.Index(trimmed, "/"); slash > 0 {
			used[trimmed[:slash]] = true
		}
	}
	return used
}

func defaultPublicAccountSelector(account CodexAccount, allPrivate map[string]bool) string {
	private := map[string]bool{account.ID: true, FallbackCodexAccountLogLabel(account.ID): true}
	for candidate := range allPrivate {
		private[candidate] = true
	}
	if label := account.LogLabel; CodexAccountLogLabelPattern.MatchString(label) && !private[label] {
		return label
	}
	existing := make([]string, 0, len(private))
	for candidate := range private {
		existing = append(existing, candidate)
	}
	for attempt := 0; attempt < publicAccountSelectorMaxAttempts; attempt++ {
		selector := CreateCodexAccountLogLabel(existing...)
		if !private[selector] {
			return selector
		}
	}
	// Falling back to the id-derived label would expose a private identifier;
	// an unusable-but-unique selector is the safer end state.
	return FallbackCodexAccountLogLabel(account.ID + "-selector")
}

func claimNamespace(requested string, used map[string]bool) string {
	namespace := requested
	for suffix := 2; used[namespace]; suffix++ {
		namespace = requested + "-" + strconv.Itoa(suffix)
	}
	used[namespace] = true
	return namespace
}

