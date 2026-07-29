package oauth

import (
	"errors"
	"net/http"
	"strings"
)

// TerminalRefreshError marks a refresh failure that retrying cannot fix: the
// grant was revoked, reused, or has expired beyond recovery. Only a refresh can
// produce one -- an authorization-code exchange failing says nothing about a
// stored grant, because there is no stored grant yet.
type TerminalRefreshError struct {
	Provider   string
	HTTPStatus int
	OAuthError string
	Err        error
}

func (e *TerminalRefreshError) Error() string { return e.Err.Error() }

func (e *TerminalRefreshError) Unwrap() error { return e.Err }

// NewRefreshHTTPError classifies a provider refresh failure. It returns err
// unchanged unless the upstream both named an allowlisted OAuth error AND
// answered with 400 or 401.
//
// The status gate is the whole point. An upstream can return 500 with
// `{"error":"invalid_grant"}` in the body during an outage, and treating that as
// terminal would lock a user out of an account that still works. The oracle
// gates the same way (src/oauth/index.ts:307-310).
func NewRefreshHTTPError(provider string, status int, oauthError string, err error) error {
	code := allowlistedOAuthErrorCode(oauthError)
	if code == "" || (status != http.StatusBadRequest && status != http.StatusUnauthorized) {
		return err
	}
	return &TerminalRefreshError{Provider: provider, HTTPStatus: status, OAuthError: code, Err: err}
}

// allowlistedOAuthErrorCode normalizes the codes that mean a grant is gone.
// An unrecognized code returns empty rather than being echoed, so an upstream
// body can never smuggle text into a decision we make about a user's account.
func allowlistedOAuthErrorCode(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "invalid_grant", "refresh_token_reused", "revoked", "revoked_token",
		"refresh_token_revoked", "access_denied", "expired_token":
		return normalized
	}
	return ""
}

// IsTerminalRefreshError reports whether a refresh failure is durable, meaning
// the account needs a human to log in again. Only the typed error counts: a
// message-substring guess cannot see the HTTP status, and without the status the
// gate above is lost.
func IsTerminalRefreshError(err error) bool {
	var terminal *TerminalRefreshError
	return errors.As(err, &terminal)
}
