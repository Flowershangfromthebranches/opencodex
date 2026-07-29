package codex

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"
)

// CatalogSupportsReasoningSummaries reports what the ACTIVE catalog says about a
// model's reasoning-summary support, and whether it knew the model at all.
//
// The wire sanitizer consults this per request because the catalog changes
// underneath a long-lived client: a Codex session that started before a refresh
// keeps asking for summaries on a model that no longer supports them, and the
// upstream answers 400. Mirrors the oracle's
// catalogModelSupportsReasoningSummaries (src/codex/catalog/parsing.ts:348).
func CatalogSupportsReasoningSummaries(catalogPath, cachePath, modelID string) (bool, bool) {
	if strings.TrimSpace(modelID) == "" {
		return false, false
	}
	entries := reasoningSummaryCatalog(catalogPath, cachePath)
	if len(entries) == 0 {
		return false, false
	}
	if supported, ok := entries[modelID]; ok {
		return supported, true
	}
	// A routed slug (`provider/model`) answers for its bare id only when every
	// routed entry agrees; a split verdict is no verdict.
	encoded := EncodeRoutedModelIDForCatalog(modelID)
	var seen *bool
	for slug, supported := range entries {
		index := strings.Index(slug, "/")
		if index < 0 || slug[index+1:] != encoded {
			continue
		}
		if seen == nil {
			value := supported
			seen = &value
			continue
		}
		if *seen != supported {
			return false, false
		}
	}
	if seen == nil {
		return false, false
	}
	return *seen, true
}

// EncodeRoutedModelIDForCatalog matches the slug encoding used when routed
// models are written into the catalog.
func EncodeRoutedModelIDForCatalog(id string) string {
	return strings.ReplaceAll(id, "/", SlugAliasSeparatorForCatalog)
}

// SlugAliasSeparatorForCatalog is the separator routed slugs use in place of
// "/", kept here so this file does not import the registry.
const SlugAliasSeparatorForCatalog = "--"

type reasoningSummaryCache struct {
	mu      sync.Mutex
	path    string
	modTime time.Time
	entries map[string]bool
}

var reasoningSummaryEntries reasoningSummaryCache

// reasoningSummaryCatalog reads the catalog at most once per file change. The
// sanitizer runs on every request, so re-reading and re-parsing the file each
// time would put disk IO on the hot path.
func reasoningSummaryCatalog(catalogPath, cachePath string) map[string]bool {
	for _, path := range []string{catalogPath, cachePath} {
		if strings.TrimSpace(path) == "" {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		reasoningSummaryEntries.mu.Lock()
		if reasoningSummaryEntries.path == path && reasoningSummaryEntries.modTime.Equal(info.ModTime()) {
			entries := reasoningSummaryEntries.entries
			reasoningSummaryEntries.mu.Unlock()
			return entries
		}
		reasoningSummaryEntries.mu.Unlock()

		entries := parseReasoningSummarySupport(path)
		if entries == nil {
			continue
		}
		reasoningSummaryEntries.mu.Lock()
		reasoningSummaryEntries.path, reasoningSummaryEntries.modTime, reasoningSummaryEntries.entries = path, info.ModTime(), entries
		reasoningSummaryEntries.mu.Unlock()
		return entries
	}
	return nil
}

func parseReasoningSummarySupport(path string) map[string]bool {
	data, err := os.ReadFile(path)
	if err != nil || len(data) > MaxCatalogBytes {
		return nil
	}
	var document struct {
		Models []struct {
			Slug                       string `json:"slug"`
			ID                         string `json:"id"`
			SupportsReasoningSummaries *bool  `json:"supports_reasoning_summaries"`
		} `json:"models"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return nil
	}
	entries := make(map[string]bool, len(document.Models))
	for _, model := range document.Models {
		if model.SupportsReasoningSummaries == nil {
			continue
		}
		for _, key := range []string{model.Slug, model.ID} {
			if key != "" {
				entries[key] = *model.SupportsReasoningSummaries
			}
		}
	}
	return entries
}
