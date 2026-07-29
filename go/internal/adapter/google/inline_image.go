package google

import (
	"path/filepath"
	"strings"

	"github.com/lidge-jun/opencodex-go/internal/images"
	"github.com/lidge-jun/opencodex-go/internal/types"
)

// materializeInlinePart writes a Gemini response image to disk and returns the
// opaque URL the model should see.
//
// Gemini can answer with an image rather than text. Dropping it loses the
// answer; putting the host path in the transcript hands a filesystem location
// to a model and to any remote client reading the turn. The oracle does neither
// -- it materializes the bytes and substitutes an authenticated opaque URL
// (src/adapters/google.ts:499-508).
//
// The third return reports whether this part carried an image at all, so the
// caller can tell "no image here" from "image that failed".
func (a *Adapter) materializeInlinePart(part map[string]any, budget *images.ImageBudget) (string, *types.AdapterEvent, bool) {
	inline, ok := part["inlineData"].(map[string]any)
	if !ok {
		return "", nil, false
	}
	data := stringValue(inline["data"])
	if data == "" {
		return "", nil, false
	}
	home := strings.TrimSpace(a.ArtifactsHome)
	if home == "" {
		// Without a home there is nowhere to put the bytes, and inventing one
		// would write where the artifact route does not read.
		return "", &types.AdapterEvent{Type: types.EventError, Error: "failed to materialize inline image"}, true
	}
	// Checked here as well as inside the decoder so an oversized image is
	// reported as such rather than as a generic write failure, matching the
	// oracle's two distinct messages (src/adapters/google.ts:501).
	if len(data) > images.MaxEncodedBytesPerImage {
		return "", &types.AdapterEvent{Type: types.EventError, Error: "inline image exceeds per-image size cap"}, true
	}
	path, err := images.MaterializeInlineImage(filepath.Join(home, images.ArtifactsDirName), data, budget)
	if err != nil {
		return "", &types.AdapterEvent{Type: types.EventError, Error: "failed to materialize inline image"}, true
	}
	url, err := images.ArtifactHTTPURL(path)
	if err != nil {
		return "", &types.AdapterEvent{Type: types.EventError, Error: "failed to materialize inline image"}, true
	}
	// Parens would end a markdown link early. The current artifact id pattern
	// cannot produce one, so this is parity insurance rather than a live need.
	return strings.NewReplacer("(", "\\(", ")", "\\)").Replace(url), nil, true
}
