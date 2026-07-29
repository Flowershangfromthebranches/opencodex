package management

import (
	"net/http"
	"strings"

	"github.com/lidge-jun/opencodex-go/internal/config"
)

// A cooldown outlives the condition that caused it: an upstream can rescind a
// rate limit and a user can upgrade a plan, but the account still sits idle for
// the full Retry-After the provider named. The dashboard offers a way to lift
// it, and these are the routes behind those buttons — absent in Go, so the
// buttons 404'd.

func (a *API) postCodexAuthClearCooldown(w http.ResponseWriter, r *http.Request) bool {
	var body struct {
		ID string `json:"id"`
	}
	if !decodeJSON(w, r, &body) {
		return true
	}
	id := strings.TrimSpace(body.ID)
	if id == "" {
		writeError(w, http.StatusBadRequest, "Invalid account id format")
		return true
	}
	cleared := false
	if a.codexRouter != nil {
		if _, active := a.codexRouter.GetCodexAccountCooldownUntil(id, a.now()); active {
			cleared = true
		}
		// Clears the health entry the cooldown lives on. Safe when nothing is
		// cooling: the account simply has no entry to remove.
		a.codexRouter.ClearCodexUpstreamHealth(id)
	}
	writeJSON(w, http.StatusOK, orderedJSONObject{
		{name: "ok", value: true},
		{name: "id", value: id},
		{name: "cleared", value: cleared},
	})
	return true
}

func (a *API) postOAuthClearCooldown(w http.ResponseWriter, r *http.Request) bool {
	var body struct {
		Provider  string `json:"provider"`
		AccountID string `json:"accountId"`
	}
	if !decodeJSON(w, r, &body) {
		return true
	}
	provider := strings.ToLower(strings.TrimSpace(body.Provider))
	accountID := strings.TrimSpace(body.AccountID)
	// Only Anthropic has an account pool with cooldowns; accepting another
	// provider would report success for an operation that did nothing.
	if provider != "anthropic" {
		writeError(w, http.StatusBadRequest, "clear-cooldown is only supported for anthropic")
		return true
	}
	if accountID == "" {
		writeError(w, http.StatusBadRequest, "missing accountId")
		return true
	}
	cleared := false
	if a.anthropicPool != nil {
		cleared = a.anthropicPool.ClearCooldown(accountID)
	}
	writeJSON(w, http.StatusOK, orderedJSONObject{
		{name: "ok", value: true},
		{name: "cleared", value: cleared},
	})
	return true
}

func (a *API) putCodexAuthPoolStrategy(w http.ResponseWriter, r *http.Request) bool {
	var body struct {
		Strategy    *string `json:"strategy"`
		StickyLimit *int    `json:"stickyLimit"`
	}
	if !decodeJSON(w, r, &body) {
		return true
	}
	if body.Strategy == nil && body.StickyLimit == nil {
		writeError(w, http.StatusBadRequest, "strategy or stickyLimit required")
		return true
	}
	var strategy string
	if body.Strategy != nil {
		parsed, ok := config.ParseAccountPoolStrategy(*body.Strategy)
		if !ok {
			writeError(w, http.StatusBadRequest, "strategy must be one of: quota, round-robin, fill-first")
			return true
		}
		strategy = parsed
	}
	var sticky int
	if body.StickyLimit != nil {
		parsed, ok := config.ParseAccountPoolStickyLimit(*body.StickyLimit)
		if !ok {
			writeError(w, http.StatusBadRequest, "stickyLimit must be an integer 1-100")
			return true
		}
		sticky = parsed
	}

	a.mu.Lock()
	if body.Strategy != nil {
		a.config.AccountPoolStrategy = strategy
	}
	if body.StickyLimit != nil {
		value := sticky
		a.config.AccountPoolStickyLimit = &value
	}
	effectiveStrategy := config.NormalizedAccountPoolStrategy(a.config.AccountPoolStrategy)
	effectiveSticky := config.NormalizedAccountPoolStickyLimit(a.config.AccountPoolStickyLimit)
	err := a.saveLocked()
	a.mu.Unlock()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "save pool strategy failed")
		return true
	}
	writeJSON(w, http.StatusOK, orderedJSONObject{
		{name: "ok", value: true},
		{name: "accountPoolStrategy", value: effectiveStrategy},
		{name: "accountPoolStickyLimit", value: effectiveSticky},
	})
	return true
}
