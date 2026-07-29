package config

import "github.com/lidge-jun/opencodex-go/internal/providers"

// registryAllowsPrivateNetwork reports the built-in opt-in for local runtimes
// (ollama, vllm, lm-studio, litellm), so those keep working without every user
// setting allowPrivateNetwork by hand.
func registryAllowsPrivateNetwork(name string) bool {
	entry, found := providers.GetProviderRegistryEntry(name)
	return found && entry.AllowPrivateNetworkByDefault
}
