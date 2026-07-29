package oauth

// ClearCooldown lifts a rate-limit cooldown so the account can serve again
// immediately, and reports whether one was actually lifted.
//
// The dashboard offers this because a cooldown outlives the condition that
// caused it: an upstream can rescind a rate limit, or the user can upgrade a
// plan, and without a way to clear it the account sits idle for the full
// Retry-After the provider named. The oracle exposes the same operation
// (src/server/management/oauth-account-routes.ts:319).
func (p *AnthropicPool) ClearCooldown(accountID string) bool {
	if accountID == "" {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, found := p.cooldowns[accountID]; !found {
		return false
	}
	delete(p.cooldowns, accountID)
	return true
}
