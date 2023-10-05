package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/ipfs/boxo/ipns"
	"github.com/ipfs/go-cid"
	routinghelpers "github.com/libp2p/go-libp2p-routing-helpers"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/routing"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const DefaultKuboRPC = "http://127.0.0.1:5001"

type proxyRouting struct {
	kuboRPC    []string
	httpClient *http.Client
	rand       *rand.Rand
}

func newProxyRouting(kuboRPC []string, cdns *cachedDNS) routing.Routing {
	s := rand.NewSource(time.Now().Unix())
	rand := rand.New(s)

	return &proxyRouting{
		kuboRPC: kuboRPC,
		httpClient: &http.Client{
			Transport: otelhttp.NewTransport(&customTransport{
				// Roundtripper with increased defaults than http.Transport such that retrieving
				// multiple lookups concurrently is fast.
				RoundTripper: &http.Transport{
					MaxIdleConns:        1000,
					MaxConnsPerHost:     100,
					MaxIdleConnsPerHost: 100,
					IdleConnTimeout:     90 * time.Second,
					DialContext:         cdns.dialWithCachedDNS,
					ForceAttemptHTTP2:   true,
				},
			}),
		},
		rand: rand,
	}
}

func (ps *proxyRouting) Bootstrap(context.Context) error {
	return nil
}

func (ps *proxyRouting) PutValue(context.Context, string, []byte, ...routing.Option) error {
	return routing.ErrNotSupported
}

func (ps *proxyRouting) GetValue(ctx context.Context, k string, opts ...routing.Option) ([]byte, error) {
	return ps.fetch(ctx, k)
}

func (ps *proxyRouting) FindPeer(context.Context, peer.ID) (peer.AddrInfo, error) {
	return peer.AddrInfo{}, routing.ErrNotSupported
}

func (ps *proxyRouting) FindProvidersAsync(context.Context, cid.Cid, int) <-chan peer.AddrInfo {
	ch := make(chan peer.AddrInfo)
	close(ch)
	return ch
}

func (ps *proxyRouting) Provide(context.Context, cid.Cid, bool) error {
	return routing.ErrNotSupported
}

func (ps *proxyRouting) SearchValue(ctx context.Context, k string, opts ...routing.Option) (<-chan []byte, error) {
	if !strings.HasPrefix(k, "/ipns/") {
		return nil, routing.ErrNotSupported
	}

	ch := make(chan []byte)

	go func() {
		v, err := ps.fetch(ctx, k)
		if err != nil {
			close(ch)
		} else {
			ch <- v
			close(ch)
		}
	}()

	return ch, nil
}

func (ps *proxyRouting) fetch(ctx context.Context, key string) (rb []byte, err error) {
	name, err := ipns.NameFromRoutingKey([]byte(key))
	if err != nil {
		return nil, err
	}

	key = "/ipns/" + name.String()

	urlStr := fmt.Sprintf("%s/api/v0/dht/get?arg=%s", ps.getRandomKuboURL(), key)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, urlStr, nil)
	if err != nil {
		return nil, err
	}

	goLog.Debugw("routing proxy fetch", "key", key, "from", req.URL.String())
	defer func() {
		if err != nil {
			goLog.Debugw("routing proxy fetch error", "key", key, "from", req.URL.String(), "error", err.Error())
		}
	}()

	resp, err := ps.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Read at most 10 KiB (max size of IPNS record).
	rb, err = io.ReadAll(io.LimitReader(resp.Body, 10240))
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("routing/get RPC returned unexpected status: %s, body: %q", resp.Status, string(rb))
	}

	parts := bytes.Split(bytes.TrimSpace(rb), []byte("\n"))
	var b64 string

	for _, part := range parts {
		var evt routing.QueryEvent
		err = json.Unmarshal(part, &evt)
		if err != nil {
			return nil, fmt.Errorf("routing/get RPC response cannot be parsed: %w", err)
		}

		if evt.Type == routing.Value {
			b64 = evt.Extra
			break
		}
	}

	if b64 == "" {
		return nil, errors.New("routing/get RPC returned no value")
	}

	rb, err = base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, err
	}

	entry, err := ipns.UnmarshalRecord(rb)
	if err != nil {
		return nil, err
	}

	err = ipns.ValidateWithName(entry, name)
	if err != nil {
		return nil, err
	}

	return rb, nil
}

func (ps *proxyRouting) getRandomKuboURL() string {
	return ps.kuboRPC[ps.rand.Intn(len(ps.kuboRPC))]
}

// httpRoutingWrapper is a wrapper needed to construct the routing.Routing interface from
// http delegated routing.
type httpRoutingWrapper struct {
	routing.ContentRouting
	routing.PeerRouting
	routing.ValueStore
	routinghelpers.ProvideManyRouter
}

func (c *httpRoutingWrapper) Bootstrap(ctx context.Context) error {
	return nil
}
