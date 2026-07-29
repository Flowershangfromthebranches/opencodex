package server

import "strings"

// isTransportLevelPreflightError reports a failure that never produced a valid
// HTTP response body.
//
// Bun rejects these inside fetch(), so the oracle answers 502 before any stream
// exists. Go's http client accepts the response and only fails on the first
// read, which is why they surface here as an adapter error instead. Matching
// the oracle's status for the same upstream condition means catching them at
// the first event rather than letting them travel down a stream the oracle
// would never have opened.
//
// Kept deliberately narrow: everything not listed here is an ordinary provider
// failure and belongs IN the stream, as a response.failed event.
func isTransportLevelPreflightError(message string) bool {
	lower := strings.ToLower(message)
	for _, marker := range []string{"invalid byte in chunk length", "invalid chunk"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}
