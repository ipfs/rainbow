package main

import (
	"context"
	"crypto/rand"
	"io"
	"net/http"
	"testing"
	"time"

	bsnet "github.com/ipfs/boxo/bitswap/network"
	bsserver "github.com/ipfs/boxo/bitswap/server"
	"github.com/ipfs/go-metrics-interface"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTrustless(t *testing.T) {
	t.Parallel()

	ts, gnd := mustTestServer(t, Config{
		Bitswap:                 true,
		TrustlessGatewayDomains: []string{"trustless.com"},
		disableMetrics:          true,
	})

	content := "hello world"
	cid := mustAddFile(t, gnd, []byte(content))
	url := ts.URL + "/ipfs/" + cid.String()

	t.Run("Non-trustless request returns 406", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		require.NoError(t, err)
		req.Host = "trustless.com"

		res, err := http.DefaultClient.Do(req)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusNotAcceptable, res.StatusCode)
	})

	t.Run("Trustless request with query parameter returns 200", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, url+"?format=raw", nil)
		require.NoError(t, err)
		req.Host = "trustless.com"

		res, err := http.DefaultClient.Do(req)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, res.StatusCode)
	})

	t.Run("Trustless request with accept header returns 200", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		require.NoError(t, err)
		req.Host = "trustless.com"
		req.Header.Set("Accept", "application/vnd.ipld.raw")

		res, err := http.DefaultClient.Do(req)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, res.StatusCode)
	})
}

func TestNoBlockcacheHeader(t *testing.T) {
	ts, gnd := mustTestServer(t, Config{
		Bitswap: true,
	})

	content := make([]byte, 1024)
	_, err := rand.Read(content)
	require.NoError(t, err)
	cid := mustAddFile(t, gnd, content)
	url := ts.URL + "/ipfs/" + cid.String()

	t.Run("Successful download of data with standard already cached in the node", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		require.NoError(t, err)

		res, err := http.DefaultClient.Do(req)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, res.StatusCode)
		responseBody, err := io.ReadAll(res.Body)
		assert.NoError(t, err)
		assert.Equal(t, content, responseBody)
	})

	t.Run("When caching is explicitly skipped the data is not found", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		require.NoError(t, err)

		req.Header.Set(NoBlockcacheHeader, "true")
		_, err = http.DefaultClient.Do(req)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})

	t.Run("When caching is explicitly skipped the data is found if a peer has it", func(t *testing.T) {
		newHost, err := libp2p.New()
		require.NoError(t, err)

		ctx := context.Background()
		// pacify metrics reporting code
		ctx = metrics.CtxScope(ctx, "test.bsserver.host")
		n := bsnet.NewFromIpfsHost(newHost, nil)
		bs := bsserver.New(ctx, n, gnd.blockstore)
		n.Start(bs)
		defer bs.Close()

		require.NoError(t, newHost.Connect(context.Background(), peer.AddrInfo{
			ID:    gnd.host.ID(),
			Addrs: gnd.host.Addrs(),
		}))

		ctx, cancel := context.WithTimeout(ctx, time.Second*1)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		require.NoError(t, err)

		req.Header.Set(NoBlockcacheHeader, "true")
		res, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, res.StatusCode)
		responseBody, err := io.ReadAll(res.Body)
		assert.NoError(t, err)
		assert.Equal(t, content, responseBody)
	})
}
