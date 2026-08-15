package egress

import (
	"context"
	"net"
	"testing"

	xproxy "golang.org/x/net/proxy"
)

type recordingDialer struct {
	addresses []string
	err       error
}

func (d *recordingDialer) Dial(_ string, address string) (net.Conn, error) {
	d.addresses = append(d.addresses, address)
	return nil, d.err
}

func TestIPv6PreferredDialerUsesAAAAAddressBeforeHostname(t *testing.T) {
	recorder := &recordingDialer{}
	dialer := &ipv6PreferredDialer{
		dialer: recorder,
		lookup: func(context.Context, string) ([]net.IP, error) {
			return []net.IP{net.ParseIP("2001:db8::10")}, nil
		},
	}

	_, _ = dialer.Dial("tcp", "grok.example:443")

	if len(recorder.addresses) != 1 || recorder.addresses[0] != "[2001:db8::10]:443" {
		t.Fatalf("dial addresses = %#v, want IPv6 destination first", recorder.addresses)
	}
}

func TestIPv6PreferredDialerFallsBackToHostname(t *testing.T) {
	recorder := &recordingDialer{err: net.ErrClosed}
	dialer := &ipv6PreferredDialer{
		dialer: recorder,
		lookup: func(context.Context, string) ([]net.IP, error) {
			return []net.IP{net.ParseIP("2001:db8::10")}, nil
		},
	}

	_, _ = dialer.Dial("tcp", "grok.example:443")

	if len(recorder.addresses) != 2 || recorder.addresses[0] != "[2001:db8::10]:443" || recorder.addresses[1] != "grok.example:443" {
		t.Fatalf("dial addresses = %#v, want IPv6 then hostname fallback", recorder.addresses)
	}
}

var _ xproxy.Dialer = (*recordingDialer)(nil)
