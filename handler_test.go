package main

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
