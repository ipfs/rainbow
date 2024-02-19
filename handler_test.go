package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	chunker "github.com/ipfs/boxo/chunker"
	"github.com/ipfs/boxo/ipld/merkledag"
	"github.com/ipfs/boxo/ipld/unixfs/importer/balanced"
	uih "github.com/ipfs/boxo/ipld/unixfs/importer/helpers"
	util "github.com/ipfs/boxo/util"
	"github.com/ipfs/go-cid"
	ic "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/multiformats/go-multicodec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type rpcRedirectTest struct {
	url      string
	location string
	status   int
}

func TestRPCRedirectsToGateway(t *testing.T) {
	rpcHandler := newKuboRPCHandler([]string{"http://example.com"})

	tests := []rpcRedirectTest{
		{"/api/v0/cat?arg=bafkreifzjut3te2nhyekklss27nh3k72ysco7y32koao5eei66wof36n5e", "/ipfs/bafkreifzjut3te2nhyekklss27nh3k72ysco7y32koao5eei66wof36n5e", http.StatusSeeOther},
		{"/api/v0/cat?arg=/ipfs/bafkreifzjut3te2nhyekklss27nh3k72ysco7y32koao5eei66wof36n5e", "/ipfs/bafkreifzjut3te2nhyekklss27nh3k72ysco7y32koao5eei66wof36n5e", http.StatusSeeOther},
		{"/api/v0/cat?arg=/ipns/k51qzi5uqu5dgutdk6i1ynyzgkqngpha5xpgia3a5qqp4jsh0u4csozksxel2r", "/ipns/k51qzi5uqu5dgutdk6i1ynyzgkqngpha5xpgia3a5qqp4jsh0u4csozksxel2r", http.StatusSeeOther},

		{"/api/v0/dag/get?arg=bafkreifzjut3te2nhyekklss27nh3k72ysco7y32koao5eei66wof36n5e", "/ipfs/bafkreifzjut3te2nhyekklss27nh3k72ysco7y32koao5eei66wof36n5e?format=dag-json", http.StatusSeeOther},
		{"/api/v0/dag/get?arg=/ipfs/bafkreifzjut3te2nhyekklss27nh3k72ysco7y32koao5eei66wof36n5e", "/ipfs/bafkreifzjut3te2nhyekklss27nh3k72ysco7y32koao5eei66wof36n5e?format=dag-json", http.StatusSeeOther},
		{"/api/v0/dag/get?arg=/ipns/k51qzi5uqu5dgutdk6i1ynyzgkqngpha5xpgia3a5qqp4jsh0u4csozksxel2r", "/ipns/k51qzi5uqu5dgutdk6i1ynyzgkqngpha5xpgia3a5qqp4jsh0u4csozksxel2r?format=dag-json", http.StatusSeeOther},
		{"/api/v0/dag/get?arg=bafkreifzjut3te2nhyekklss27nh3k72ysco7y32koao5eei66wof36n5e&output-codec=dag-cbor", "/ipfs/bafkreifzjut3te2nhyekklss27nh3k72ysco7y32koao5eei66wof36n5e?format=dag-cbor", http.StatusSeeOther},

		{"/api/v0/dag/export?arg=bafkreifzjut3te2nhyekklss27nh3k72ysco7y32koao5eei66wof36n5e", "/ipfs/bafkreifzjut3te2nhyekklss27nh3k72ysco7y32koao5eei66wof36n5e?format=car", http.StatusSeeOther},
		{"/api/v0/dag/export?arg=/ipfs/bafkreifzjut3te2nhyekklss27nh3k72ysco7y32koao5eei66wof36n5e", "/ipfs/bafkreifzjut3te2nhyekklss27nh3k72ysco7y32koao5eei66wof36n5e?format=car", http.StatusSeeOther},
		{"/api/v0/dag/export?arg=/ipns/k51qzi5uqu5dgutdk6i1ynyzgkqngpha5xpgia3a5qqp4jsh0u4csozksxel2r", "/ipns/k51qzi5uqu5dgutdk6i1ynyzgkqngpha5xpgia3a5qqp4jsh0u4csozksxel2r?format=car", http.StatusSeeOther},

		{"/api/v0/block/get?arg=bafkreifzjut3te2nhyekklss27nh3k72ysco7y32koao5eei66wof36n5e", "/ipfs/bafkreifzjut3te2nhyekklss27nh3k72ysco7y32koao5eei66wof36n5e?format=raw", http.StatusSeeOther},
		{"/api/v0/block/get?arg=/ipfs/bafkreifzjut3te2nhyekklss27nh3k72ysco7y32koao5eei66wof36n5e", "/ipfs/bafkreifzjut3te2nhyekklss27nh3k72ysco7y32koao5eei66wof36n5e?format=raw", http.StatusSeeOther},
		{"/api/v0/block/get?arg=/ipns/k51qzi5uqu5dgutdk6i1ynyzgkqngpha5xpgia3a5qqp4jsh0u4csozksxel2r", "/ipns/k51qzi5uqu5dgutdk6i1ynyzgkqngpha5xpgia3a5qqp4jsh0u4csozksxel2r?format=raw", http.StatusSeeOther},
	}

	for _, test := range tests {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			req, err := http.NewRequest(method, "http://127.0.0.1"+test.url, nil)
			assert.Nil(t, err)
			resp := httptest.NewRecorder()
			rpcHandler.ServeHTTP(resp, req)

			assert.Equal(t, test.status, resp.Code)
			assert.Equal(t, test.location, resp.Header().Get("Location"))
		}
	}
}

func TestRPCRedirectsToKubo(t *testing.T) {
	rpcHandler := newKuboRPCHandler([]string{"http://example.com"})

	tests := []rpcRedirectTest{
		{"/api/v0/name/resolve?arg=some-arg", "http://example.com/api/v0/name/resolve?arg=some-arg", http.StatusTemporaryRedirect},
		{"/api/v0/name/resolve/some-arg", "http://example.com/api/v0/name/resolve/some-arg", http.StatusTemporaryRedirect},
		{"/api/v0/resolve?arg=some-arg", "http://example.com/api/v0/resolve?arg=some-arg", http.StatusTemporaryRedirect},
		{"/api/v0/dag/resolve?arg=some-arg", "http://example.com/api/v0/dag/resolve?arg=some-arg", http.StatusTemporaryRedirect},
		{"/api/v0/dns?arg=some-arg", "http://example.com/api/v0/dns?arg=some-arg", http.StatusTemporaryRedirect},
	}

	for _, test := range tests {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			req, err := http.NewRequest(method, "http://127.0.0.1"+test.url, nil)
			assert.Nil(t, err)
			resp := httptest.NewRecorder()
			rpcHandler.ServeHTTP(resp, req)

			assert.Equal(t, test.status, resp.Code)
			assert.Equal(t, test.location, resp.Header().Get("Location"))
		}
	}
}

func TestRPCNotImplemented(t *testing.T) {
	rpcHandler := newKuboRPCHandler([]string{"http://example.com"})

	for _, method := range []string{http.MethodGet, http.MethodPost} {
		req, err := http.NewRequest(method, "http://127.0.0.1/api/v0/ping", nil)
		assert.Nil(t, err)
		resp := httptest.NewRecorder()
		rpcHandler.ServeHTTP(resp, req)

		assert.Equal(t, http.StatusNotImplemented, resp.Code)
	}
}

func mustTestNode(t *testing.T, cfg Config) *Node {
	cfg.DataDir = t.TempDir()
	cfg.BlockstoreType = "flatfs"

	ctx := context.Background()

	sr := util.NewTimeSeededRand()
	sk, _, err := ic.GenerateKeyPairWithReader(ic.Ed25519, 2048, sr)
	require.NoError(t, err)

	cdns := newCachedDNS(dnsCacheRefreshInterval)

	t.Cleanup(func() {
		_ = cdns.Close()
	})

	gnd, err := Setup(ctx, cfg, sk, cdns)
	require.NoError(t, err)
	return gnd
}

func mustTestServer(t *testing.T, cfg Config) (*httptest.Server, *Node) {
	gnd := mustTestNode(t, cfg)

	handler, err := setupGatewayHandler(cfg, gnd)
	if err != nil {
		require.NoError(t, err)
	}

	ts := httptest.NewServer(handler)

	return ts, gnd
}

func mustAddFile(t *testing.T, gnd *Node, content []byte) cid.Cid {
	dsrv := merkledag.NewDAGService(gnd.bsrv)

	// Create a UnixFS graph from our file, parameters described here but can be visualized at https://dag.ipfs.tech/
	ufsImportParams := uih.DagBuilderParams{
		Maxlinks:  uih.DefaultLinksPerBlock, // Default max of 174 links per block
		RawLeaves: true,                     // Leave the actual file bytes untouched instead of wrapping them in a dag-pb protobuf wrapper
		CidBuilder: cid.V1Builder{ // Use CIDv1 for all links
			Codec:    uint64(multicodec.DagPb),
			MhType:   uint64(multicodec.Sha2_256), // Use SHA2-256 as the hash function
			MhLength: -1,                          // Use the default hash length for the given hash function (in this case 256 bits)
		},
		Dagserv: dsrv,
		NoCopy:  false,
	}
	ufsBuilder, err := ufsImportParams.New(chunker.NewSizeSplitter(bytes.NewReader(content), chunker.DefaultBlockSize)) // Split the file up into fixed sized 256KiB chunks
	require.NoError(t, err)

	nd, err := balanced.Layout(ufsBuilder) // Arrange the graph with a balanced layout
	require.NoError(t, err)

	return nd.Cid()
}

func TestTrustless(t *testing.T) {
	t.Parallel()

	ts, gnd := mustTestServer(t, Config{
		TrustlessGatewayDomains: []string{"trustless.com"},
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
