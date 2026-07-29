package lib

import (
	"context"
	"net"

	"github.com/lidge-jun/opencodex-go/internal/destination"
)

// The destination classifier moved to internal/destination so config and
// routing can reach it too: internal/lib sits above internal/config in the
// import graph, which left the two layers that most need this policy unable to
// call it. These aliases keep existing callers working.

type DestinationOptions = destination.Options

type IPResolver = destination.IPResolver

func ProviderDestinationConfigError(baseURL string, options DestinationOptions) string {
	return destination.ConfigError(baseURL, options)
}

func ProviderDestinationResolvedError(ctx context.Context, baseURL string, options DestinationOptions, resolver IPResolver) string {
	return destination.ResolvedError(ctx, baseURL, options, resolver)
}

func ClassifyDestinationIP(ip net.IP) (string, string) { return destination.ClassifyIP(ip) }

func ClassifyDestinationHost(host string) (string, string) { return destination.ClassifyHost(host) }
