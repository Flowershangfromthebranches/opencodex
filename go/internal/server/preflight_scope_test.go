package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lidge-jun/opencodex-go/internal/types"
)

// An ordinary provider failure belongs INSIDE the stream the client asked for.
// Answering it with a JSON 502 instead made a normal upstream error look like
// the proxy itself had broken, and cost the client the response.failed event
// that explains what happened. The oracle preflights only on a combo attempt
// (src/server/responses/core.ts:1980).
func TestOrdinaryProviderErrorStreamsInsteadOfPreflight502(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {}\n\n")
	}))
	defer upstream.Close()

	core := NewResponsesCore(ResponsesCoreConfig{
		Registry: coreRegistry{endpoint: upstream.URL},
		ResolveAdapter: func(*types.ResolvedModel, *types.Transport, *types.AuthContext, http.Header) (types.Adapter, error) {
			return preflightFailureAdapter{
				coreAdapter: coreAdapter{endpoint: upstream.URL},
				events:      []types.AdapterEvent{{Type: types.EventError, Error: "upstream rejected the request", StatusCode: 429}},
			}, nil
		},
	})

	response := httptest.NewRecorder()
	core.ServeHTTP(response, loopbackRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"provider/wire","input":"x","stream":true}`)))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, an ordinary provider error should still open the stream", response.Code)
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want the SSE stream the client requested", contentType)
	}
	body := response.Body.String()
	if !strings.Contains(body, "event: response.failed") {
		t.Fatalf("the failure never reached the client as an event:\n%s", body)
	}
	if !strings.Contains(body, "upstream rejected the request") {
		t.Fatalf("the upstream message was lost:\n%s", body)
	}
}

// A malformed chunked body never produced a valid HTTP response. Bun rejects it
// inside fetch(), so the oracle answers 502 with no stream at all; Go only
// discovers it on first read, so it is caught at the first event to keep the
// same client-visible outcome.
func TestTransportLevelFailureStillAnswersBeforeTheStream(t *testing.T) {
	if !isTransportLevelPreflightError("read upstream SSE stream: invalid byte in chunk length") {
		t.Fatal("a malformed chunked body must stay a pre-stream failure")
	}
	for _, ordinary := range []string{
		"upstream rejected the request",
		"rate limited",
		"Adapter ended before producing a response",
	} {
		if isTransportLevelPreflightError(ordinary) {
			t.Fatalf("%q was treated as a transport failure and would bypass the stream", ordinary)
		}
	}
}
