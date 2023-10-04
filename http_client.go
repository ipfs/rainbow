package main

import (
	"fmt"
	"net/http"
)

type customTransport struct {
	http.RoundTripper
	AuthorizationBearerToken string
}

func (adt *customTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if adt.AuthorizationBearerToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", adt.AuthorizationBearerToken))
	}
	req.Header.Add("User-Agent", userAgent)
	return adt.RoundTripper.RoundTrip(req)
}
