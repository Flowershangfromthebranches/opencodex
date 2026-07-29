package management

import (
	"context"
	"strings"
	"testing"

	"github.com/lidge-jun/opencodex-go/internal/config"
)

// `allowPrivateNetwork` exists so someone can point a provider at Ollama on
// their own machine. It must not also unblock the cloud metadata endpoint:
// that address hands out instance credentials, and a provider aimed at it
// turns the proxy into a credential exfiltration path.
//
// The oracle settles this by ORDER — destination-policy.ts:134-141 returns the
// metadata verdict before it ever looks at the allow switch.
func TestProviderDestinationBlocksMetadataDespiteAllowPrivateNetwork(t *testing.T) {
	api := &API{}
	for _, destination := range []string{
		"http://169.254.169.254/latest/meta-data",
		"http://100.100.100.200/",
		"http://metadata.google.internal/computeMetadata/v1/",
	} {
		provider := config.ProviderConfig{BaseURL: destination, AllowPrivateNetwork: true}
		err := api.providerDestinationResolvedError(context.Background(), "custom", provider, false)
		if err == nil {
			t.Fatalf("allowPrivateNetwork opened the metadata endpoint %s", destination)
		}
		if !strings.Contains(err.Error(), "metadata") {
			t.Fatalf("%s rejected for the wrong reason: %v", destination, err)
		}
	}
}

// A provider whose registry entry is local by default (ollama) gets the same
// treatment: local yes, metadata no.
func TestProviderDestinationBlocksMetadataForRegistryLocalProviders(t *testing.T) {
	api := &API{}
	provider := config.ProviderConfig{BaseURL: "http://169.254.169.254/"}
	if err := api.providerDestinationResolvedError(context.Background(), "ollama", provider, false); err == nil {
		t.Fatal("a registry-local provider reached the metadata endpoint")
	}
}

// The negative case that keeps the fix honest: the allow switch must still do
// its job for ordinary private addresses.
func TestProviderDestinationStillAllowsIntentionalPrivateNetworks(t *testing.T) {
	api := &API{}
	provider := config.ProviderConfig{BaseURL: "http://192.168.1.50:11434/v1", AllowPrivateNetwork: true}
	if err := api.providerDestinationResolvedError(context.Background(), "custom", provider, false); err != nil {
		t.Fatalf("allowPrivateNetwork no longer permits a LAN endpoint: %v", err)
	}
	if err := api.providerDestinationResolvedError(context.Background(), "ollama", config.ProviderConfig{BaseURL: "http://localhost:11434/v1"}, false); err != nil {
		t.Fatalf("registry-local provider lost its localhost endpoint: %v", err)
	}
}
