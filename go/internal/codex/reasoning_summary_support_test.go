package codex

import (
	"os"
	"path/filepath"
	"testing"
)

func writeCatalog(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "opencodex-catalog.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// The catalog is the live answer to "does this model still support reasoning
// summaries". A Codex session that started before a refresh keeps asking for
// them on a model that no longer does, and the upstream answers 400 -- which is
// exactly the case the oracle's guard exists for
// (src/adapters/openai-responses.ts:61).
func TestCatalogSupportsReasoningSummariesReadsTheActiveCatalog(t *testing.T) {
	path := writeCatalog(t, `{"models":[
		{"slug":"gpt-5.5","supports_reasoning_summaries":false},
		{"slug":"gpt-5.6-sol","supports_reasoning_summaries":true}
	]}`)

	if supported, known := CatalogSupportsReasoningSummaries(path, "", "gpt-5.5"); !known || supported {
		t.Fatalf("gpt-5.5 supported=%v known=%v, catalog says false", supported, known)
	}
	if supported, known := CatalogSupportsReasoningSummaries(path, "", "gpt-5.6-sol"); !known || !supported {
		t.Fatalf("gpt-5.6-sol supported=%v known=%v, catalog says true", supported, known)
	}
}

// A model the catalog never mentions is unknown, not unsupported: guessing
// would strip the field from models that are perfectly capable.
func TestCatalogSupportsReasoningSummariesReportsUnknownModels(t *testing.T) {
	path := writeCatalog(t, `{"models":[{"slug":"gpt-5.5","supports_reasoning_summaries":false}]}`)

	if _, known := CatalogSupportsReasoningSummaries(path, "", "some-other-model"); known {
		t.Fatal("an unlisted model was treated as known")
	}
	if _, known := CatalogSupportsReasoningSummaries("/nonexistent/catalog.json", "", "gpt-5.5"); known {
		t.Fatal("a missing catalog produced a verdict")
	}
}

// A routed slug answers for its bare id only when every routed entry agrees.
func TestCatalogSupportsReasoningSummariesRequiresRoutedAgreement(t *testing.T) {
	agreed := writeCatalog(t, `{"models":[
		{"slug":"provider-a/my--model","supports_reasoning_summaries":false},
		{"slug":"provider-b/my--model","supports_reasoning_summaries":false}
	]}`)
	if supported, known := CatalogSupportsReasoningSummaries(agreed, "", "my/model"); !known || supported {
		t.Fatalf("agreeing routed entries did not answer: supported=%v known=%v", supported, known)
	}

	split := writeCatalog(t, `{"models":[
		{"slug":"provider-a/my--model","supports_reasoning_summaries":false},
		{"slug":"provider-b/my--model","supports_reasoning_summaries":true}
	]}`)
	if _, known := CatalogSupportsReasoningSummaries(split, "", "my/model"); known {
		t.Fatal("a split verdict was treated as an answer")
	}
}
