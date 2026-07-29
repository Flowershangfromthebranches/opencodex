package config

import (
	"strings"
	"testing"
)

// Config load is the first gate a persisted provider passes. The oracle checks
// the destination there (src/config.ts:781), so a config.json edited by hand to
// aim a provider at the instance-metadata endpoint is rejected before anything
// routes through it. Go only validated URL syntax.
func TestValidateRejectsMetadataAndPrivateProviderDestinations(t *testing.T) {
	for _, testCase := range []struct{ name, baseURL, want string }{
		{"metadata literal", "http://169.254.169.254/latest/meta-data", "metadata"},
		{"metadata host", "http://metadata.google.internal/computeMetadata/v1/", "metadata"},
		{"loopback", "http://127.0.0.1:11434/v1", "loopback"},
		{"private", "http://10.0.0.4/v1", "private-network"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			cfg := Default()
			cfg.Providers["custom"] = ProviderConfig{Adapter: "openai-chat", BaseURL: testCase.baseURL, APIKey: "k", AuthMode: "key"}
			err := cfg.Validate()
			if err == nil {
				t.Fatalf("%s was accepted at config load", testCase.baseURL)
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("%s rejected for the wrong reason: %v", testCase.baseURL, err)
			}
		})
	}
}

// The opt-ins must keep working: an intentional local runtime is the reason
// allowPrivateNetwork exists, and registry-local providers get it by default.
func TestValidateAllowsIntentionalLocalDestinations(t *testing.T) {
	cfg := Default()
	cfg.Providers["custom"] = ProviderConfig{Adapter: "openai-chat", BaseURL: "http://127.0.0.1:11434/v1", APIKey: "k", AuthMode: "key", AllowPrivateNetwork: true}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("allowPrivateNetwork no longer permits a local endpoint: %v", err)
	}

	local := Default()
	local.Providers["ollama"] = ProviderConfig{Adapter: "openai-chat", BaseURL: "http://localhost:11434/v1", APIKey: "k", AuthMode: "key"}
	if err := local.Validate(); err != nil {
		t.Fatalf("registry-local provider was rejected: %v", err)
	}
}

// allowPrivateNetwork is not a metadata switch, at config load either.
func TestValidateBlocksMetadataEvenWithAllowPrivateNetwork(t *testing.T) {
	cfg := Default()
	cfg.Providers["custom"] = ProviderConfig{Adapter: "openai-chat", BaseURL: "http://169.254.169.254/", APIKey: "k", AuthMode: "key", AllowPrivateNetwork: true}
	if err := cfg.Validate(); err == nil {
		t.Fatal("allowPrivateNetwork opened the metadata endpoint at config load")
	}
}
