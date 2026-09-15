package main

import (
	"testing"

	ma "github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
)

func TestIsUndialableMergedAddr(t *testing.T) {
	// The bridge from dhtHost.Peerstore() into the bitswap host must drop
	// IP-rooted addresses that aren't publicly routable, but it must not
	// reject addresses whose first component isn't an IP just because
	// manet.IsPublicAddr returns false for them.
	cases := []struct {
		addr string
		drop bool
	}{
		// IPv4 public.
		{"/ip4/1.2.3.4/tcp/4001", false},
		{"/ip4/89.124.92.77/udp/4001/quic-v1", false},
		// IPv4 non-public.
		{"/ip4/127.0.0.1/tcp/4001", true},
		{"/ip4/10.0.0.1/tcp/4001", true},
		{"/ip4/192.168.1.1/tcp/4001", true},
		{"/ip4/172.16.0.1/tcp/4001", true},
		{"/ip4/169.254.1.1/tcp/4001", true},
		// IPv6 public and non-public.
		{"/ip6/2606:4700::1/tcp/4001", false},
		{"/ip6/::1/tcp/4001", true},
		{"/ip6/fc00::1/tcp/4001", true},
		// IP6ZONE-prefixed (link-local with zone id) must still be evaluated.
		{"/ip6zone/eth0/ip6/fe80::1/tcp/4001", true},
		// Pure relay hop without an IP head. manet.IsPublicAddr returns false
		// here, but we must keep it: the swarm resolves the relay separately.
		{"/p2p/12D3KooWPZgaSPM84PKCb78vEugeJTA7X2k6WYcKV1TGv1M9FJGq/p2p-circuit/p2p/12D3KooWPZgaSPM84PKCb78vEugeJTA7X2k6WYcKV1TGv1M9FJGq", false},
		// DNS-rooted transports with ordinary names: kept; BasicHost resolves
		// them on dial.
		{"/dns4/example.com/tcp/4001", false},
		{"/dns6/example.com/tcp/4001", false},
		{"/dnsaddr/bootstrap.libp2p.io/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN", false},
		// Special-use DNS names that can never resolve to a useful public
		// destination: dropped. Same reasoning as RFC1918/loopback IPs.
		{"/dns4/foo.local/tcp/4001", true},
		{"/dns/printer.home.arpa/tcp/4001", true},
		{"/dns4/myhost.localhost/tcp/4001", true},
		{"/dns/bogus.invalid/tcp/4001", true},
		{"/dns4/widget.test/tcp/4001", true},
		// Relay address with a public IP head is still public.
		{"/ip4/147.75.83.83/tcp/4001/p2p/12D3KooWPZgaSPM84PKCb78vEugeJTA7X2k6WYcKV1TGv1M9FJGq/p2p-circuit", false},
		// Relay address with a private IP head is not.
		{"/ip4/192.168.1.1/tcp/4001/p2p/12D3KooWPZgaSPM84PKCb78vEugeJTA7X2k6WYcKV1TGv1M9FJGq/p2p-circuit", true},
	}

	for _, tc := range cases {
		t.Run(tc.addr, func(t *testing.T) {
			addr, err := ma.NewMultiaddr(tc.addr)
			require.NoError(t, err)
			require.Equal(t, tc.drop, isUndialableMergedAddr(addr))
		})
	}

	t.Run("empty multiaddr", func(t *testing.T) {
		require.True(t, isUndialableMergedAddr(ma.Multiaddr{}))
	})
}
