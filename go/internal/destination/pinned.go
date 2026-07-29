package destination

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"syscall"
	"time"
)

// ErrBlockedDestination reports a connection refused by policy rather than by
// the network.
var ErrBlockedDestination = errors.New("destination blocked by policy")

// PinnedClient returns an HTTP client that re-checks the address at CONNECT
// time, closing the gap between a DNS pre-check and the socket it was meant to
// protect.
//
// Go's http client resolves the hostname again when it dials, so a pre-flight
// lookup guarantees nothing: a name can answer with a public address for the
// check and a loopback or metadata address for the connection (DNS rebinding),
// and the request goes wherever the second answer points. The oracle closes
// this by pinning the validated address into the connection itself
// (src/lib/pinned-http.ts); Go's Dialer exposes a Control hook that runs after
// resolution with the concrete address, which enforces the same guarantee at
// the last possible moment.
//
// allowPrivateNetwork keeps the deliberate local-runtime case working; a
// metadata endpoint is refused regardless, matching the ordering everywhere
// else in this package.
func PinnedClient(base *http.Client, allowPrivateNetwork bool) *http.Client {
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			return CheckDialAddress(address, allowPrivateNetwork)
		},
	}
	var transport *http.Transport
	if base != nil {
		if existing, ok := base.Transport.(*http.Transport); ok && existing != nil {
			transport = existing.Clone()
		}
	}
	if transport == nil {
		transport = http.DefaultTransport.(*http.Transport).Clone()
	}
	transport.DialContext = dialer.DialContext
	client := &http.Client{Transport: transport}
	if base != nil {
		client.Timeout = base.Timeout
		// Redirects are refused rather than followed: a 302 is a second
		// destination the policy never saw, and following it would reopen the
		// hole this client exists to close. The oracle uses redirect:"manual"
		// for the same reason (src/lib/provider-outbound.ts:126).
		client.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
	} else {
		client.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
	return client
}

// CheckDialAddress judges a concrete "host:port" the dialer is about to
// connect to.
func CheckDialAddress(address string, allowPrivateNetwork bool) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return nil
	}
	kind, detail := ClassifyIP(ip)
	switch kind {
	case "metadata":
		// Never permitted, whatever the provider opted into.
		return fmt.Errorf("%w: %s (%s)", ErrBlockedDestination, detail, ip)
	case "public":
		return nil
	}
	if allowPrivateNetwork {
		return nil
	}
	return fmt.Errorf("%w: %s (%s)", ErrBlockedDestination, detail, ip)
}
