package kiro

import (
	"regexp"
	"strings"
)

// retryableTransportFailure matches failures that mean the connection died
// before the response was complete. A retry replays the same request, so only
// these are replay-safe.
var retryableTransportFailure = regexp.MustCompile(`(?i)socket connection was closed|connection(?: was)? closed unexpectedly|econnreset|epipe|und_err_|fetch failed|decoder failed|premature close|other side closed|unexpected eof|network connection lost|terminated|truncated message at end of stream|eventstream:\s*truncated`)

// invalidKiroPayload marks a malformed event the upstream actually sent. The
// connection worked; replaying it produces the same malformed payload.
var invalidKiroPayload = regexp.MustCompile(`(?i)^invalid kiro\b`)

// IsRetryableStreamCatchError reports whether a mid-stream failure may be
// retried, given whether output has already reached the client.
//
// The emittedOutput gate is the important half. Once the client has seen text,
// a retry replays the turn from the beginning and the client receives that
// content twice — so a failure after output is terminal no matter how
// transport-like it looks. The oracle applies the same rule
// (src/adapters/kiro.ts:656-663), and Cursor's adapter has the same gate.
func IsRetryableStreamCatchError(message string, emittedOutput bool) bool {
	if emittedOutput {
		return false
	}
	trimmed := strings.TrimSpace(message)
	if invalidKiroPayload.MatchString(trimmed) {
		return false
	}
	return retryableTransportFailure.MatchString(trimmed)
}
