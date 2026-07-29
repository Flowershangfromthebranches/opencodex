package kiro

import "testing"

// Once output has reached the client, a retry replays the turn from the
// beginning and the client sees that content twice. So a failure after output
// is terminal no matter how transport-like it looks -- which is the half Go was
// missing, since it marked every fallback failure retryable.
func TestOutputAlreadySentMakesEveryFailureTerminal(t *testing.T) {
	for _, message := range []string{
		"socket connection was closed unexpectedly",
		"ECONNRESET",
		"eventstream: truncated message at end of stream",
		"fetch failed",
	} {
		if IsRetryableStreamCatchError(message, true) {
			t.Fatalf("%q stayed retryable after output reached the client", message)
		}
		if !IsRetryableStreamCatchError(message, false) {
			t.Fatalf("%q is a transport failure and should be replay-safe before any output", message)
		}
	}
}

// A malformed event is something the upstream actually sent: the connection
// worked, so replaying produces the same malformed payload.
func TestMalformedPayloadsAreNeverRetried(t *testing.T) {
	for _, message := range []string{
		"invalid Kiro event payload",
		"Invalid Kiro tool call",
	} {
		if IsRetryableStreamCatchError(message, false) {
			t.Fatalf("%q was treated as replay-safe", message)
		}
	}
}

// An unrecognised failure is not assumed replay-safe. Retrying something we
// cannot classify risks duplicate work against the upstream.
func TestUnknownFailuresAreNotRetried(t *testing.T) {
	for _, message := range []string{
		"Kiro rejected the request",
		"quota exhausted",
		"",
	} {
		if IsRetryableStreamCatchError(message, false) {
			t.Fatalf("%q was treated as replay-safe", message)
		}
	}
}

// Truncation is a partial frame plus a clean EOF: same replay-safe class as a
// socket close, and specifically called out in the oracle because it is easy to
// mistake for a malformed payload.
func TestEventstreamTruncationIsReplaySafe(t *testing.T) {
	if !IsRetryableStreamCatchError("eventstream: truncated message at end of stream", false) {
		t.Fatal("eventstream truncation was treated as terminal")
	}
}
