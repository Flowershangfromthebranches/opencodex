package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// X-Frame-Options is the legacy header, and current browsers increasingly
// honour only the CSP directive. Sending just the old one left the local
// dashboard framable by a page the user happened to visit, which is the whole
// point of the header on a loopback service. The oracle sends both
// (src/server/auth-cors.ts:114-115).
func TestResponsesCarryFrameAncestorsCSP(t *testing.T) {
	handler := corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), MiddlewareConfig{Hostname: "127.0.0.1"})

	// A loopback bind gates on Host before anything else, so the request has to
	// look like one the dashboard would actually make.
	request := httptest.NewRequest(http.MethodGet, "/api/providers", nil)
	request.Host = "127.0.0.1:10100"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if got := response.Header().Get("Content-Security-Policy"); got != "frame-ancestors 'none'" {
		t.Fatalf("Content-Security-Policy = %q", got)
	}
	if got := response.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("X-Frame-Options = %q, the legacy header must stay for older browsers", got)
	}
}
