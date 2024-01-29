package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
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
