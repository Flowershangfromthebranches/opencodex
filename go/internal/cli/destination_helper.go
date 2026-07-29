package cli

import "github.com/lidge-jun/opencodex-go/internal/registry"

// registryAllowsPrivateNetwork reports the built-in opt-in for local runtimes,
// so ollama and friends keep resolving without the user setting the flag.
func registryAllowsPrivateNetwork(name string) bool {
	entry, found := registry.New().Lookup(name)
	return found && (entry.AllowPrivateNetworkByDefault || entry.AllowPrivateNetworkDefault)
}
