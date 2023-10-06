package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWithUserAgent(t *testing.T) {
	client := &http.Client{
		Transport: &customTransport{
			RoundTripper: http.DefaultTransport,
		},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.EqualValues(t, r.UserAgent(), "rainbow/"+buildVersion())
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	_, err := client.Get(ts.URL)
	assert.Nil(t, err)
}

func TestWithAuthorizationBearerToken(t *testing.T) {
	secret := "secret"

	client := &http.Client{
		Transport: &customTransport{
			AuthorizationBearerToken: secret,
			RoundTripper:             http.DefaultTransport,
		},
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		assert.EqualValues(t, auth, "Bearer "+secret)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	_, err := client.Get(ts.URL)
	assert.Nil(t, err)
}
