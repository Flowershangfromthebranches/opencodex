package destination

import (
	"context"
	"net"
	"strings"
	"testing"
)

type fixedResolver []net.IPAddr

func (r fixedResolver) LookupIPAddr(context.Context, string) ([]net.IPAddr, error) { return r, nil }

func TestDestinationPolicyLiteralAndResolved(t *testing.T) {
	if got := ConfigError("http://169.254.169.254/latest", Options{AllowPrivateNetwork: true}); !strings.Contains(got, "metadata") {
		t.Fatalf("metadata error = %q", got)
	}
	if got := ConfigError("http://127.0.0.1:11434", Options{}); !strings.Contains(got, "loopback") {
		t.Fatalf("loopback error = %q", got)
	}
	if got := ConfigError("http://127.0.0.1:11434", Options{AllowPrivateNetwork: true}); got != "" {
		t.Fatalf("allowed local error = %q", got)
	}
	got := ResolvedError(context.Background(), "https://provider.example/v1", Options{}, fixedResolver{{IP: net.ParseIP("10.0.0.4")}})
	if !strings.Contains(got, "private-network") {
		t.Fatalf("resolved error = %q", got)
	}
}
