package server

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lidge-jun/opencodex-go/internal/adapter/google"
	"github.com/lidge-jun/opencodex-go/internal/images"
	"github.com/lidge-jun/opencodex-go/internal/types"
)

// A 1x1 PNG. The decoder reads the real bytes to pick an extension, so this has
// to be a genuine image rather than arbitrary base64.
const onePixelPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

func artifactServer(t *testing.T, home string) http.Handler {
	t.Helper()
	proxy := New(Config{ArtifactsHome: home})
	return proxy.Handler()
}

// artifactRequest builds a request the loopback origin gate accepts. That gate
// is the shared middleware, and it covering this route is exactly why the
// handler does not implement its own.
func artifactRequest(url string) *http.Request {
	request := httptest.NewRequest(http.MethodGet, url, nil)
	request.Host = "127.0.0.1:10100"
	return request
}

func TestArtifactRouteServesAMaterializedImage(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, images.ArtifactsDirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path, err := images.MaterializeInlineImage(dir, onePixelPNG, &images.ImageBudget{})
	if err != nil {
		t.Fatal(err)
	}
	url, err := images.ArtifactHTTPURL(path)
	if err != nil {
		t.Fatal(err)
	}

	response := httptest.NewRecorder()
	artifactServer(t, home).ServeHTTP(response, artifactRequest(url))

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("content-type = %q", got)
	}
	if got := response.Header().Get("Cache-Control"); got != "private, max-age=3600" {
		t.Fatalf("cache-control = %q", got)
	}
	if got := response.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("nosniff header = %q", got)
	}
}

func TestArtifactRouteRefusesUnknownAndEscapingIDs(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, images.ArtifactsDirName), 0o700); err != nil {
		t.Fatal(err)
	}
	// A file the route must never reach, one level above the artifacts dir.
	if err := os.WriteFile(filepath.Join(home, "secret.png"), []byte("not yours"), 0o600); err != nil {
		t.Fatal(err)
	}
	handler := artifactServer(t, home)

	for _, id := range []string{
		"missing.png",
		"..%2Fsecret.png",
		"%2Fetc%2Fpasswd",
		".hidden.png",
	} {
		t.Run(id, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, artifactRequest(images.ArtifactHTTPPrefix+"/"+id))
			if response.Code == http.StatusOK {
				t.Fatalf("id %q was served: %s", id, response.Body.String())
			}
		})
	}
}

// Without a configured home there is nothing to serve, and falling back to the
// Codex home would read a directory nothing writes to.
func TestArtifactRouteWithoutAHomeAnswersNotFound(t *testing.T) {
	response := httptest.NewRecorder()
	artifactServer(t, "").ServeHTTP(response, artifactRequest(images.ArtifactHTTPPrefix+"/whatever.png"))
	if response.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", response.Code)
	}
}

// The handler deliberately has no auth check of its own. This proves the shared
// middleware really covers the route -- if it ever stopped, the route would
// quietly become an unauthenticated file server.
func TestArtifactRouteSitsBehindTheSharedOriginGate(t *testing.T) {
	response := httptest.NewRecorder()
	// A non-loopback Host is what a cross-origin caller cannot forge.
	request := httptest.NewRequest(http.MethodGet, images.ArtifactHTTPPrefix+"/whatever.png", nil)
	request.Host = "attacker.test"
	artifactServer(t, t.TempDir()).ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want 403 from the shared origin gate", response.Code)
	}
}

// The two halves have to meet: the URL the Gemini adapter hands to the model
// must be fetchable through the route. Testing them apart would pass even if
// each used a different directory.
func TestGeminiInlineImageIsReachableThroughTheArtifactRoute(t *testing.T) {
	home := t.TempDir()
	adapter := &google.Adapter{ArtifactsHome: home}

	events, err := adapter.ParseUnary(t.Context(), []byte(`{"candidates":[{"content":{"parts":[
		{"inlineData":{"mimeType":"image/png","data":"`+onePixelPNG+`"}}
	]},"finishReason":"STOP"}]}`))
	if err != nil {
		t.Fatal(err)
	}

	var markdown string
	for _, event := range events {
		if event.Type == types.EventError {
			t.Fatalf("adapter reported %q", event.Error)
		}
		if event.Type == types.EventTextDelta && strings.Contains(event.Text, images.ArtifactHTTPPrefix) {
			markdown = event.Text
		}
	}
	if markdown == "" {
		t.Fatalf("no artifact markdown in %#v", events)
	}

	start := strings.Index(markdown, images.ArtifactHTTPPrefix)
	url := strings.TrimSuffix(markdown[start:], ")\n")
	response := httptest.NewRecorder()
	artifactServer(t, home).ServeHTTP(response, artifactRequest(url))
	if response.Code != http.StatusOK {
		t.Fatalf("the URL handed to the model 404s: %s -> %d", url, response.Code)
	}
	if _, err := base64.StdEncoding.DecodeString(onePixelPNG); err != nil {
		t.Fatal(err)
	}
}
