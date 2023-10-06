package main

import (
	"context"
	"net"
	"time"

	"github.com/rs/dnscache"
)

// How often should we check for successful updates to cached entries
const dnsCacheRefreshInterval = 5 * time.Minute

// Local DNS cache because in this world things are ephemeral
type cachedDNS struct {
	resolver  *dnscache.Resolver
	refresher *time.Ticker
}

func newCachedDNS(refreshInterval time.Duration) *cachedDNS {
	cache := &cachedDNS{
		resolver:  &dnscache.Resolver{},
		refresher: time.NewTicker(refreshInterval),
	}

	// Configure DNS cache to not remove stale records to protect gateway from
	// catastrophic failures like https://github.com/ipfs/bifrost-gateway/issues/34
	options := dnscache.ResolverRefreshOptions{}
	options.ClearUnused = false
	options.PersistOnFailure = true

	// Every refreshInterval we check for updates, but if there is
	// none, or if domain disappears, we keep the last cached version
	go func(cdns *cachedDNS) {
		defer cdns.refresher.Stop()
		for range cdns.refresher.C {
			cdns.resolver.RefreshWithOptions(options)
		}
	}(cache)

	return cache
}

// dialWithCachedDNS implements DialContext that uses cachedDNS
func (cdns *cachedDNS) dialWithCachedDNS(ctx context.Context, network string, addr string) (conn net.Conn, err error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ips, err := cdns.resolver.LookupHost(ctx, host)
	if err != nil {
		return nil, err
	}
	// Try all IPs returned by DNS
	for _, ip := range ips {
		var dialer net.Dialer
		conn, err = dialer.DialContext(ctx, network, net.JoinHostPort(ip, port))
		if err == nil {
			break
		}
	}
	return
}

func (cdns *cachedDNS) Close() error {
	cdns.refresher.Stop()
	return nil
}
