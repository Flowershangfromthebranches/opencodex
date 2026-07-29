package oauth

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type antigravityStubDoer struct {
	status int
	body   string
}

func (d antigravityStubDoer) Do(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: d.status,
		Body:       io.NopCloser(strings.NewReader(d.body)),
		Header:     http.Header{"Content-Type": {"application/json"}},
	}, nil
}

func antigravityFlow(status int, body string) *AntigravityFlow {
	return &AntigravityFlow{
		Client:   antigravityStubDoer{status: status, body: body},
		TokenURL: "https://oauth2.example.test/token",
		ClientID: "id", ClientSecret: "secret",
		Now: func() time.Time { return time.Unix(1_700_000_000, 0) },
	}
}

// A revoked Antigravity grant used to surface as a bare "HTTP 400", which no
// caller could distinguish from a transient failure — so the guardian retried a
// refresh that can never succeed and the account was never marked for
// re-login. The user just sees requests failing with no instruction.
func TestAntigravityRefreshMarksRevokedGrantTerminal(t *testing.T) {
	_, err := antigravityFlow(http.StatusBadRequest, `{"error":"invalid_grant"}`).Refresh(context.Background(), "stale-refresh")
	if err == nil {
		t.Fatal("a revoked grant refreshed successfully")
	}
	if !IsTerminalRefreshError(err) {
		t.Fatalf("revoked grant was not terminal: %v", err)
	}
}

// The status gate is the point. An upstream outage answering 500 with the same
// body must NOT lock the user out of an account that still works.
func TestAntigravityRefreshTreatsServerErrorsAsTransient(t *testing.T) {
	for _, status := range []int{http.StatusInternalServerError, http.StatusBadGateway, http.StatusTooManyRequests} {
		_, err := antigravityFlow(status, `{"error":"invalid_grant"}`).Refresh(context.Background(), "good-refresh")
		if err == nil {
			t.Fatalf("HTTP %d refreshed successfully", status)
		}
		if IsTerminalRefreshError(err) {
			t.Fatalf("HTTP %d with invalid_grant was treated as terminal, which would lock out a working account", status)
		}
	}
}

// An unrecognized code is not evidence a grant is gone, so it stays retryable
// rather than being echoed into a decision about the user's account.
func TestAntigravityRefreshIgnoresUnknownErrorCodes(t *testing.T) {
	_, err := antigravityFlow(http.StatusBadRequest, `{"error":"something_else"}`).Refresh(context.Background(), "refresh")
	if err == nil {
		t.Fatal("expected an error")
	}
	if IsTerminalRefreshError(err) {
		t.Fatalf("an unknown code was treated as terminal: %v", err)
	}
}

// The authorization-code exchange must never mark an account revoked: a stale
// one-time code says nothing about the stored credential, and terminating there
// would lock a user out over a mistyped login.
func TestAntigravityExchangeNeverMarksTerminal(t *testing.T) {
	flow := antigravityFlow(http.StatusBadRequest, `{"error":"invalid_grant"}`)
	_, err := flow.Exchange(context.Background(), "code", "verifier", "https://localhost/callback")
	if err == nil {
		t.Fatal("expected an error")
	}
	if IsTerminalRefreshError(err) {
		t.Fatalf("a failed code exchange marked the account revoked: %v", err)
	}
}
