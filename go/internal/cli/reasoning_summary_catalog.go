package cli

import (
	"os"
	"path/filepath"

	openaiadapter "github.com/lidge-jun/opencodex-go/internal/adapter/openai"
	"github.com/lidge-jun/opencodex-go/internal/codex"
)

// installReasoningSummaryCatalogLookup lets the Responses wire sanitizer consult
// the active Codex catalog. internal/codex imports the adapter package, so the
// dependency has to be injected here rather than called directly.
func installReasoningSummaryCatalogLookup() {
	openaiadapter.CatalogModelSupportsReasoningSummaries = func(modelID string) (bool, bool) {
		home := os.Getenv("CODEX_HOME")
		if home == "" {
			return false, false
		}
		return codex.CatalogSupportsReasoningSummaries(
			filepath.Join(home, "opencodex-catalog.json"),
			filepath.Join(home, "models_cache.json"),
			modelID,
		)
	}
}
