package oauth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// stubDoer answers every request with one canned response, which is enough to
// drive a provider flow's failure branch without touching the network.
type stubDoer struct {
	status int
	body   string
}

func (d stubDoer) Do(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: d.status,
		Body:       io.NopCloser(strings.NewReader(d.body)),
		Header:     make(http.Header),
	}, nil
}

func TestNewRefreshHTTPErrorGatesOnStatusAndAllowlist(t *testing.T) {
	base := errors.New("refresh failed")
	for _, testCase := range []struct {
		name     string
		status   int
		code     string
		terminal bool
	}{
		{"400 with a revoked grant is terminal", http.StatusBadRequest, "invalid_grant", true},
		{"401 with a revoked grant is terminal", http.StatusUnauthorized, "revoked_token", true},
		{"the code is matched case-insensitively", http.StatusBadRequest, "  Invalid_Grant ", true},
		// The gate exists for this row: an upstream outage can echo the same code
		// in a 5xx body, and marking that terminal locks a working account.
		{"500 carrying the same code is transient", http.StatusInternalServerError, "invalid_grant", false},
		{"503 carrying the same code is transient", http.StatusServiceUnavailable, "invalid_grant", false},
		{"429 carrying the same code is transient", http.StatusTooManyRequests, "invalid_grant", false},
		{"an unrecognized code on 400 is not terminal", http.StatusBadRequest, "server_error", false},
		{"no code at all is not terminal", http.StatusBadRequest, "", false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			err := NewRefreshHTTPError("test", testCase.status, testCase.code, base)
			if got := IsTerminalRefreshError(err); got != testCase.terminal {
				t.Fatalf("IsTerminalRefreshError = %t, want %t", got, testCase.terminal)
			}
			if !errors.Is(err, base) {
				t.Fatalf("wrapped error lost its cause: %v", err)
			}
		})
	}
}

func TestIsTerminalRefreshErrorIgnoresMessageText(t *testing.T) {
	// A plain error whose text happens to contain a marker must not be terminal:
	// without a status there is no way to tell a revoked grant from an outage.
	if IsTerminalRefreshError(errors.New("upstream reported the certificate was revoked")) {
		t.Fatal("message text alone must not mark an account for re-auth")
	}
	if IsTerminalRefreshError(nil) {
		t.Fatal("nil is not a terminal failure")
	}
}

func TestProviderRefreshFailuresClassifyByStatus(t *testing.T) {
	const revoked = `{"error":"invalid_grant","error_description":"grant revoked"}`
	for _, testCase := range []struct {
		name     string
		status   int
		body     string
		refresh  func(HTTPDoer) (OAuthCredentials, error)
		terminal bool
	}{
		{
			name: "Kiro 400 invalid_grant", status: http.StatusBadRequest, body: revoked, terminal: true,
			refresh: func(c HTTPDoer) (OAuthCredentials, error) {
				return NewKiroFlow(c).Refresh(context.Background(), KiroImportedCredential{OAuthCredentials: OAuthCredentials{Refresh: "grant"}})
			},
		},
		{
			name: "Kiro 500 invalid_grant", status: http.StatusInternalServerError, body: revoked, terminal: false,
			refresh: func(c HTTPDoer) (OAuthCredentials, error) {
				return NewKiroFlow(c).Refresh(context.Background(), KiroImportedCredential{OAuthCredentials: OAuthCredentials{Refresh: "grant"}})
			},
		},
		{
			name: "Kiro 400 unrecognized code", status: http.StatusBadRequest, body: `{"error":"server_error"}`, terminal: false,
			refresh: func(c HTTPDoer) (OAuthCredentials, error) {
				return NewKiroFlow(c).Refresh(context.Background(), KiroImportedCredential{OAuthCredentials: OAuthCredentials{Refresh: "grant"}})
			},
		},
		{
			name: "Anthropic 400 invalid_grant", status: http.StatusBadRequest, body: revoked, terminal: true,
			refresh: func(c HTTPDoer) (OAuthCredentials, error) {
				return (&AnthropicFlow{Client: c}).Refresh(context.Background(), "grant")
			},
		},
		{
			name: "Anthropic 500 invalid_grant", status: http.StatusInternalServerError, body: revoked, terminal: false,
			refresh: func(c HTTPDoer) (OAuthCredentials, error) {
				return (&AnthropicFlow{Client: c}).Refresh(context.Background(), "grant")
			},
		},
		{
			name: "ChatGPT 400 invalid_grant", status: http.StatusBadRequest, body: revoked, terminal: true,
			refresh: func(c HTTPDoer) (OAuthCredentials, error) {
				return (&ChatGPTFlow{Client: c}).Refresh(context.Background(), "grant")
			},
		},
		{
			name: "GitHub Copilot 400 invalid_grant", status: http.StatusBadRequest, body: revoked, terminal: true,
			refresh: func(c HTTPDoer) (OAuthCredentials, error) {
				return NewGithubCopilotFlow(c).Refresh(context.Background(), "ghr_grant")
			},
		},
		{
			name: "GitHub Copilot 500 invalid_grant", status: http.StatusInternalServerError, body: revoked, terminal: false,
			refresh: func(c HTTPDoer) (OAuthCredentials, error) {
				return NewGithubCopilotFlow(c).Refresh(context.Background(), "ghr_grant")
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := testCase.refresh(stubDoer{status: testCase.status, body: testCase.body})
			if err == nil {
				t.Fatal("expected the refresh to fail")
			}
			if got := IsTerminalRefreshError(err); got != testCase.terminal {
				t.Fatalf("IsTerminalRefreshError(%v) = %t, want %t", err, got, testCase.terminal)
			}
		})
	}
}

func TestAuthorizationExchangeFailuresAreNeverTerminal(t *testing.T) {
	// An exchange failing says nothing about a stored grant, because there is no
	// stored grant yet. Only a refresh can retire an account.
	const revoked = `{"error":"invalid_grant"}`
	client := stubDoer{status: http.StatusBadRequest, body: revoked}

	chatgpt := &ChatGPTFlow{Client: client}
	if _, err := chatgpt.postToken(context.Background(), url.Values{}, "ChatGPT token exchange", ""); IsTerminalRefreshError(err) {
		t.Fatalf("ChatGPT exchange must not be terminal: %v", err)
	}
	anthropic := &AnthropicFlow{Client: client}
	if _, err := anthropic.postToken(context.Background(), map[string]any{}, "Anthropic token exchange", "", ""); IsTerminalRefreshError(err) {
		t.Fatalf("Anthropic exchange must not be terminal: %v", err)
	}
	xai := &XAIFlow{Client: client, sleep: func(context.Context, time.Duration) error { return nil }}
	if _, err := xai.postToken(context.Background(), "https://example.invalid/token", url.Values{}, "", ""); IsTerminalRefreshError(err) {
		t.Fatalf("xAI exchange must not be terminal: %v", err)
	}
}

func TestTerminalRefreshMarksAccountForReauth(t *testing.T) {
	store := NewCredentialStore(filepath.Join(t.TempDir(), "auth.json"))
	credential := OAuthCredentials{Access: "old", Refresh: "grant", Expires: time.Now().Add(time.Hour).UnixMilli()}
	if err := store.SaveCredential(context.Background(), "anthropic", credential); err != nil {
		t.Fatal(err)
	}
	set, _, _ := store.GetAccountSet("anthropic")
	accountID := set.ActiveAccountID

	terminal := NewRefreshHTTPError("anthropic", http.StatusBadRequest, "invalid_grant", fmt.Errorf("Anthropic refresh failed: HTTP 400 invalid_grant"))
	_, err := store.RefreshAccountIfGeneration(context.Background(), "anthropic", accountID, CredentialGeneration(credential),
		func(context.Context, string) (OAuthCredentials, error) { return OAuthCredentials{}, terminal })
	if !errors.Is(err, ErrLoginRequired) {
		t.Fatalf("terminal refresh error = %v, want ErrLoginRequired", err)
	}
	set, _, _ = store.GetAccountSet("anthropic")
	for _, account := range set.Accounts {
		if account.ID == accountID && !account.NeedsReauth {
			t.Fatal("a revoked grant must leave the account marked for re-auth")
		}
	}
}

func TestTransientRefreshLeavesAccountUsable(t *testing.T) {
	store := NewCredentialStore(filepath.Join(t.TempDir(), "auth.json"))
	credential := OAuthCredentials{Access: "old", Refresh: "grant", Expires: time.Now().Add(time.Hour).UnixMilli()}
	if err := store.SaveCredential(context.Background(), "anthropic", credential); err != nil {
		t.Fatal(err)
	}
	set, _, _ := store.GetAccountSet("anthropic")
	accountID := set.ActiveAccountID

	transient := NewRefreshHTTPError("anthropic", http.StatusInternalServerError, "invalid_grant", errors.New("Anthropic refresh failed: HTTP 500 invalid_grant"))
	_, err := store.RefreshAccountIfGeneration(context.Background(), "anthropic", accountID, CredentialGeneration(credential),
		func(context.Context, string) (OAuthCredentials, error) { return OAuthCredentials{}, transient })
	if errors.Is(err, ErrLoginRequired) {
		t.Fatalf("an upstream outage must not require a new login: %v", err)
	}
	set, _, _ = store.GetAccountSet("anthropic")
	for _, account := range set.Accounts {
		if account.ID == accountID && account.NeedsReauth {
			t.Fatal("an upstream outage must not retire the account")
		}
	}
}
