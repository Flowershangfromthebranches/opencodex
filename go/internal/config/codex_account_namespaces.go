package config

import (
	"regexp"
	"strings"
)

var codexAccountNamespaceKeyPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

var reservedNamespaceObjectKeys = map[string]bool{"__proto__": true, "prototype": true, "constructor": true}

const mainCodexAccountNamespaceTarget = "@main"

// validateCodexAccountNamespaces refuses every way a selector could become
// ambiguous.
//
// A selector shares its space with provider ids and combo aliases, because
// `alpha/model` has to resolve to exactly one thing. Four collisions are
// possible and all four are rejected: with a provider or combo namespace, with
// a configured pool-account id, with another selector's target, and with the
// reserved object keys that have special meaning in the JSON maps these are
// persisted in.
func validateCodexAccountNamespaces(c Config) error {
	if len(c.CodexAccountNamespaces) == 0 {
		return nil
	}
	accountIDs := map[string]bool{}
	for _, account := range c.CodexAccounts {
		if !account.IsMain && strings.TrimSpace(account.ID) != "" {
			accountIDs[account.ID] = true
		}
	}
	providerNamespaces := map[string]bool{"combo": true, "openai": true}
	for name := range c.Providers {
		providerNamespaces[strings.ToLower(name)] = true
	}
	targets := map[string]bool{}
	for _, accountID := range c.CodexAccountNamespaces {
		if accountID != mainCodexAccountNamespaceTarget {
			targets[accountID] = true
		}
	}

	for namespace, accountID := range c.CodexAccountNamespaces {
		field := "codexAccountNamespaces." + namespace
		if !codexAccountNamespaceKeyPattern.MatchString(namespace) || reservedNamespaceObjectKeys[strings.ToLower(namespace)] {
			return &ConfigError{Field: field, Message: "account selectors must use 1-64 letters, numbers, dots, underscores, or hyphens and cannot be reserved object keys"}
		}
		if providerNamespaces[strings.ToLower(namespace)] {
			return &ConfigError{Field: field, Message: "account selectors must not collide with configured provider or combo namespaces"}
		}
		if accountIDs[namespace] || targets[namespace] {
			return &ConfigError{Field: field, Message: "account selectors must not collide with configured Codex pool-account ids or account selector targets"}
		}
		if accountID == mainCodexAccountNamespaceTarget {
			continue
		}
		if !codexAccountNamespaceKeyPattern.MatchString(accountID) || reservedNamespaceObjectKeys[strings.ToLower(accountID)] || accountID == "__main__" {
			return &ConfigError{Field: field, Message: "account selector targets must be @main or valid Codex pool-account ids"}
		}
	}
	return nil
}
