package destination

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A DNS pre-check protects nothing on its own: Go's client resolves the name
// again when it dials, so a name that answers public for the check and
// loopback for the connection sends the request — with its credentials —
// wherever the second answer points. This asserts the policy runs against the
// address actually being connected to.
func TestPinnedClientRefusesALoopbackConnection(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	client := PinnedClient(nil, false)
	_, err := client.Get(upstream.URL)
	if err == nil {
		t.Fatal("a loopback destination was connected to")
	}
	if !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("refused for the wrong reason: %v", err)
	}
}

// The deliberate local-runtime case still works, which is what the opt-in is
// for.
func TestPinnedClientHonorsAllowPrivateNetwork(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer upstream.Close()

	response, err := PinnedClient(nil, true).Get(upstream.URL)
	if err != nil {
		t.Fatalf("allowPrivateNetwork no longer reaches a local runtime: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", response.StatusCode)
	}
}

// The metadata endpoint is refused whatever the provider opted into, matching
// the ordering used everywhere else in this package.
func TestCheckDialAddressAlwaysBlocksMetadata(t *testing.T) {
	for _, allowPrivate := range []bool{false, true} {
		err := CheckDialAddress("169.254.169.254:80", allowPrivate)
		if err == nil {
			t.Fatalf("metadata reached with allowPrivateNetwork=%v", allowPrivate)
		}
		if !errors.Is(err, ErrBlockedDestination) {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}

// Public destinations are unaffected, and a private one is judged by the opt-in.
func TestCheckDialAddressVerdicts(t *testing.T) {
	if err := CheckDialAddress("93.184.216.34:443", false); err != nil {
		t.Fatalf("a public address was refused: %v", err)
	}
	if err := CheckDialAddress("10.0.0.4:443", false); err == nil {
		t.Fatal("a private address was allowed without the opt-in")
	}
	if err := CheckDialAddress("10.0.0.4:443", true); err != nil {
		t.Fatalf("allowPrivateNetwork did not permit a LAN address: %v", err)
	}
}

// A redirect is a second destination the policy never judged, so it is returned
// to the caller rather than followed.
func TestPinnedClientDoesNotFollowRedirects(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/moved", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte("followed"))
	}))
	defer upstream.Close()

	response, err := PinnedClient(nil, true).Get(upstream.URL + "/start")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusFound {
		t.Fatalf("status = %d, the redirect was followed instead of reported", response.StatusCode)
	}
}
