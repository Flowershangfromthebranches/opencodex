package registry

import (
	"testing"

	"github.com/lidge-jun/opencodex-go/internal/providers"
)

// Go carries the provider catalog twice: this table drives routing and
// transport, while internal/providers drives CLI `provider add` and the
// dashboard preset list. They describe the SAME providers, so any field that
// disagrees is a provider that behaves differently depending on which surface
// the user came through — which is how opencode-free lost its
// `x-opencode-client` header on one path but not the other, and how ten
// providers ended up with a default model on one side and none on the other.
//
// Merging the two tables is a larger refactor. Until then this test is the
// thing that keeps them honest: a new provider added to one side without the
// other fails here rather than in a user's routing.
func TestBothProviderTablesAgree(t *testing.T) {
	runtime := make(map[string]Provider, len(Providers))
	for _, entry := range Providers {
		runtime[entry.ID] = entry
	}
	metadata := providers.ListRegistryEntries()
	if len(metadata) != len(runtime) {
		t.Fatalf("provider counts differ: runtime=%d metadata=%d", len(runtime), len(metadata))
	}

	for _, meta := range metadata {
		entry, present := runtime[meta.ID]
		if !present {
			t.Errorf("%s exists only in internal/providers", meta.ID)
			continue
		}
		if entry.BaseURL != meta.BaseURL {
			t.Errorf("%s baseURL: runtime=%q metadata=%q", meta.ID, entry.BaseURL, meta.BaseURL)
		}
		if entry.Adapter != meta.Adapter {
			t.Errorf("%s adapter: runtime=%q metadata=%q", meta.ID, entry.Adapter, meta.Adapter)
		}
		if entry.DefaultModel != meta.DefaultModel {
			t.Errorf("%s defaultModel: runtime=%q metadata=%q", meta.ID, entry.DefaultModel, meta.DefaultModel)
		}
		if entry.KeyOptional != meta.KeyOptional {
			t.Errorf("%s keyOptional: runtime=%v metadata=%v", meta.ID, entry.KeyOptional, meta.KeyOptional)
		}
		if entry.AllowBaseURLOverride != meta.AllowBaseURLOverride {
			t.Errorf("%s allowBaseUrlOverride: runtime=%v metadata=%v", meta.ID, entry.AllowBaseURLOverride, meta.AllowBaseURLOverride)
		}
		if entry.AllowPrivateNetworkByDefault != meta.AllowPrivateNetworkByDefault {
			t.Errorf("%s allowPrivateNetworkByDefault: runtime=%v metadata=%v", meta.ID, entry.AllowPrivateNetworkByDefault, meta.AllowPrivateNetworkByDefault)
		}
		for header, value := range meta.StaticHeaders {
			if entry.StaticHeaders[header] != value {
				t.Errorf("%s header %s: runtime=%q metadata=%q", meta.ID, header, entry.StaticHeaders[header], value)
			}
		}
		for header, value := range entry.StaticHeaders {
			if meta.StaticHeaders[header] != value {
				t.Errorf("%s header %s: runtime=%q metadata=%q", meta.ID, header, value, meta.StaticHeaders[header])
			}
		}
	}

	seen := make(map[string]bool, len(metadata))
	for _, meta := range metadata {
		seen[meta.ID] = true
	}
	for id := range runtime {
		if !seen[id] {
			t.Errorf("%s exists only in internal/registry", id)
		}
	}
}
