package egress

import (
	"context"
	"net"
	"strings"

	xproxy "golang.org/x/net/proxy"
)

// ipv6PreferredDialer makes WARP-backed SOCKS nodes use their distinct IPv6
// exits when the target has an AAAA record. The original hostname remains the
// final fallback for IPv4-only targets and proxies.
type ipv6PreferredDialer struct {
	dialer xproxy.Dialer
	lookup func(context.Context, string) ([]net.IP, error)
}

func newIPv6PreferredDialer(dialer xproxy.Dialer) *ipv6PreferredDialer {
	return &ipv6PreferredDialer{
		dialer: dialer,
		lookup: func(ctx context.Context, host string) ([]net.IP, error) {
			return net.DefaultResolver.LookupIP(ctx, "ip6", host)
		},
	}
}

func (d *ipv6PreferredDialer) Dial(network, address string) (net.Conn, error) {
	return d.DialContext(context.Background(), network, address)
}

func (d *ipv6PreferredDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if d == nil || d.dialer == nil {
		return nil, net.ErrClosed
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil || strings.TrimSpace(host) == "" || net.ParseIP(host) != nil {
		return d.dialOriginal(ctx, network, address)
	}
	if d.lookup != nil {
		if addresses, lookupErr := d.lookup(ctx, host); lookupErr == nil {
			for _, ip := range addresses {
				if ip == nil || ip.To4() != nil {
					continue
				}
				connection, dialErr := d.dialOriginal(ctx, network, net.JoinHostPort(ip.String(), port))
				if dialErr == nil {
					return connection, nil
				}
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
			}
		}
	}
	return d.dialOriginal(ctx, network, address)
}

func (d *ipv6PreferredDialer) dialOriginal(ctx context.Context, network, address string) (net.Conn, error) {
	if contextual, ok := d.dialer.(xproxy.ContextDialer); ok {
		return contextual.DialContext(ctx, network, address)
	}
	return d.dialer.Dial(network, address)
}
