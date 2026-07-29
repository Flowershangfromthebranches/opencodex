package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/lidge-jun/opencodex-go/internal/config"
)

func destinationRegistry(t *testing.T, provider string, entry config.ProviderConfig) *configBackedRegistry {
	t.Helper()
	cfg := config.Default()
	cfg.Providers[provider] = entry
	path := filepath.Join(t.TempDir(), "config.json")
	return &configBackedRegistry{config: &cfg, persistence: config.NewLivePersistence(path, &cfg)}
}

// Config load and the management API both judge the destination, but neither
// sees the URL this layer finally resolves, and neither runs at all for a
// config an older build already wrote. Resolving a transport is the last point
// where the request can still be stopped before the credential is attached, so
// the oracle asserts there too (src/router.ts:242).
func TestResolveTransportRejectsMetadataAndPrivateDestinations(t *testing.T) {
	for _, testCase := range []struct{ name, baseURL, want string }{
		{"metadata", "http://169.254.169.254/v1", "metadata"},
		{"loopback", "http://127.0.0.1:11434/v1", "loopback"},
		{"private", "http://10.0.0.4/v1", "private-network"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			reg := destinationRegistry(t, "custom", config.ProviderConfig{Adapter: "openai-chat", BaseURL: testCase.baseURL, DefaultModel: "m"})
			_, err := reg.ResolveTransport("custom", nil)
			if err == nil {
				t.Fatalf("%s resolved a transport", testCase.baseURL)
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("%s rejected for the wrong reason: %v", testCase.baseURL, err)
			}
		})
	}
}

// A metadata endpoint stays blocked even with the opt-in, at this layer too.
func TestResolveTransportBlocksMetadataDespiteAllowPrivateNetwork(t *testing.T) {
	reg := destinationRegistry(t, "custom", config.ProviderConfig{
		Adapter: "openai-chat", BaseURL: "http://169.254.169.254/v1", DefaultModel: "m", AllowPrivateNetwork: true,
	})
	if _, err := reg.ResolveTransport("custom", nil); err == nil {
		t.Fatal("allowPrivateNetwork opened the metadata endpoint at transport resolution")
	}
}

// The opt-ins keep working, or this gate would break every local runtime.
func TestResolveTransportAllowsIntentionalLocalDestinations(t *testing.T) {
	opted := destinationRegistry(t, "custom", config.ProviderConfig{
		Adapter: "openai-chat", BaseURL: "http://127.0.0.1:11434/v1", DefaultModel: "m", AllowPrivateNetwork: true,
	})
	if _, err := opted.ResolveTransport("custom", nil); err != nil {
		t.Fatalf("allowPrivateNetwork no longer reaches a local runtime: %v", err)
	}

	local := destinationRegistry(t, "ollama", config.ProviderConfig{Adapter: "openai-chat", BaseURL: "http://localhost:11434/v1", DefaultModel: "m"})
	if _, err := local.ResolveTransport("ollama", nil); err != nil {
		t.Fatalf("registry-local provider lost its endpoint: %v", err)
	}

	public := destinationRegistry(t, "openrouter", config.ProviderConfig{Adapter: "openai-chat", BaseURL: "https://openrouter.ai/api/v1", DefaultModel: "m"})
	if _, err := public.ResolveTransport("openrouter", nil); err != nil {
		t.Fatalf("a public endpoint was rejected: %v", err)
	}
}
