package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// A mid-stream upstream failure interrupts whatever SSE block was being
// written, so the failure frame has to open with a blank line or it lands
// glued to a half-written event and the client's parser drops both. The oracle
// enqueues "\n\nevent: response.failed..." for exactly that reason
// (src/server/relay.ts:78).
func TestFailureTailTerminatesAPartialSSEBlock(t *testing.T) {
	var out strings.Builder
	out.WriteString("event: response.output_text.delta\ndata: {\"delta\":\"hal")
	if err := WriteFailureTail(&out, http.StatusBadGateway, "upstream reset"); err != nil {
		t.Fatal(err)
	}
	written := out.String()
	if !strings.Contains(written, "hal\n\nevent: response.failed") {
		t.Fatalf("failure frame did not close the partial block:\n%q", written)
	}
	if !strings.HasSuffix(written, "data: [DONE]\n\n") {
		t.Fatalf("stream did not end with the DONE sentinel:\n%q", written)
	}
}

// The error object is the client's only description of what went wrong, and
// the oracle emits a string code plus a `last_error` mirror. A numeric HTTP
// status in the `code` slot does not match the Responses error schema, and a
// missing `last_error` hides the failure from clients that read only that.
func TestFailureTailCarriesTheResponsesErrorSchema(t *testing.T) {
	var out strings.Builder
	if err := WriteFailureTail(&out, http.StatusBadGateway, "Upstream stream terminated unexpectedly: reset"); err != nil {
		t.Fatal(err)
	}
	payload := failureTailPayload(t, out.String())
	response, ok := payload["response"].(map[string]any)
	if !ok {
		t.Fatalf("no response object: %#v", payload)
	}
	failure, ok := response["error"].(map[string]any)
	if !ok {
		t.Fatalf("no error object: %#v", response)
	}
	if failure["type"] != "upstream_error" {
		t.Fatalf("error type = %v, oracle sends upstream_error", failure["type"])
	}
	if failure["code"] != "upstream_reset" {
		t.Fatalf("error code = %v, oracle sends the string upstream_reset", failure["code"])
	}
	last, ok := response["last_error"].(map[string]any)
	if !ok {
		t.Fatalf("last_error is missing; clients reading it see no failure: %#v", response)
	}
	if last["code"] != failure["code"] || last["message"] != failure["message"] {
		t.Fatalf("last_error does not mirror error: %#v vs %#v", last, failure)
	}
}

func failureTailPayload(t *testing.T, written string) map[string]any {
	t.Helper()
	for _, line := range strings.Split(written, "\n") {
		if !strings.HasPrefix(line, "data: ") || strings.Contains(line, "[DONE]") {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &payload); err != nil {
			t.Fatalf("failure frame is not JSON: %v", err)
		}
		return payload
	}
	t.Fatalf("no data frame in %q", written)
	return nil
}
