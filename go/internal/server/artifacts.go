package server

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/lidge-jun/opencodex-go/internal/images"
)

// handleArtifact serves an image the proxy materialized for a model -- today
// that means a Gemini response part carrying inlineData. The model is handed an
// opaque URL instead of a host path, so without this route that URL points at
// nothing.
//
// Auth and origin are enforced by the middleware wrapping this mux, the same
// boundary every other data-plane route sits behind. A second check here would
// be a second boundary to keep in sync.
func (s *Server) handleArtifact(w http.ResponseWriter, r *http.Request) {
	home := strings.TrimSpace(s.config.ArtifactsHome)
	if home == "" {
		// No configured home means nothing wrote artifacts here. Falling back to
		// StorageHome would point at the Codex home, which is the wrong directory.
		writeJSONError(w, http.StatusNotFound, "not_found", "artifact not found")
		return
	}
	path := images.ResolveArtifactPath(filepath.Join(home, images.ArtifactsDirName), r.PathValue("id"))
	if path == "" {
		writeJSONError(w, http.StatusNotFound, "not_found", "artifact not found")
		return
	}
	w.Header().Set("Content-Type", images.ArtifactContentType(path))
	// Private: an artifact belongs to one turn, not to a shared cache.
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, path)
}
