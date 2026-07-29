package management

import (
	"net/http"

	"github.com/lidge-jun/opencodex-go/internal/providers"
)

// patchCodexAccountMode owns the account-mode switch and its side effects.
//
// The mode decides which credential every subsequent turn uses, but threads
// already bound to an account keep that binding until the map is cleared — so
// accepting the field without clearing leaves the user watching the account
// they just switched away from keep serving traffic. That is why the oracle
// gives this field a dedicated path rather than folding it into the field-mask
// editor (src/server/management/provider-routes.ts:144-175), and why the field
// is mutually exclusive with every other patch key: the shape validation and
// the side-effect path would otherwise run against each other.
func (a *API) patchCodexAccountMode(w http.ResponseWriter, name string, patch map[string]any) bool {
	if len(patch) != 1 {
		writeError(w, http.StatusBadRequest, "codexAccountMode cannot be combined with other patch fields")
		return true
	}
	if name != "openai" {
		writeError(w, http.StatusBadRequest, "codexAccountMode is valid only for provider openai")
		return true
	}
	mode, _ := patch["codexAccountMode"].(string)
	if mode != "pool" && mode != "direct" {
		writeError(w, http.StatusBadRequest, "codexAccountMode must be pool or direct")
		return true
	}

	a.mu.Lock()
	provider, found := a.config.Providers[name]
	if !found {
		a.mu.Unlock()
		writeError(w, http.StatusNotFound, "unknown provider")
		return true
	}
	if !providers.IsCanonicalOpenAiForwardProvider(provider.Adapter, provider.AuthMode, provider.BaseURL) {
		a.mu.Unlock()
		writeError(w, http.StatusBadRequest, "provider openai must be the canonical built-in provider")
		return true
	}
	provider.CodexAccountMode = mode
	a.config.Providers[name] = provider
	err := a.saveLocked()
	a.mu.Unlock()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "save provider failed")
		return true
	}

	// Persisted first, then the live state that would otherwise contradict it.
	if a.codexRouter != nil {
		a.codexRouter.ClearThreadAccountMap()
	}
	if a.modelCache != nil {
		a.modelCache.Clear(name)
	}

	writeJSON(w, http.StatusOK, orderedJSONObject{
		{name: "success", value: true},
		{name: "name", value: name},
		{name: "codexAccountMode", value: mode},
	})
	return true
}
