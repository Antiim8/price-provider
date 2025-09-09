package pricempire

import (
	"net/http"
	"net/url"
)

// HTTPClient describes an HTTP client.
//
//go:generate mockgen -package=pricempire_test -destination=mock_http_client_test.go -source=client.go HTTPClient
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// PricempireAPIClient is a client for the Pricempire API.
type PricempireAPIClient struct {
	// baseURL is the base URL for the API.
	baseURL string
	// httpClient is the HTTP httpClient.
	httpClient HTTPClient
	// header contains additional headers to be sent with each request.
	header http.Header
	// query contains additional query parameters to be sent with each request.
	query url.Values
}

// PricempireAPIClientOption is a configuration option for the Pricempire API client.
type PricempireAPIClientOption func(*PricempireAPIClient)

// WithBaseURL sets the base URL for the API.
func WithBaseURL(baseURL string) PricempireAPIClientOption {
	return func(c *PricempireAPIClient) {
		c.baseURL = baseURL
	}
}

// WithHTTPClient sets the HTTP client for the API.
func WithHTTPClient(httpClient HTTPClient) PricempireAPIClientOption {
	return func(c *PricempireAPIClient) {
		c.httpClient = httpClient
	}
}

// WithHeader sets additional headers to be sent with each request.
func WithHeader(header http.Header) PricempireAPIClientOption {
	return func(c *PricempireAPIClient) {
		for key, values := range header {
			for _, value := range values {
				c.header.Add(key, value)
			}
		}
	}
}

// NewPricempireAPIClient creates a new Pricempire API client.
func NewPricempireAPIClient(key string, options ...PricempireAPIClientOption) (*PricempireAPIClient, error) {
	var pricempireAPIClient = &PricempireAPIClient{
		baseURL:    baseURL,
		httpClient: http.DefaultClient,
		header:     http.Header{},
		query:      url.Values{},
	}
	if key != "" {
		// This is the header that is used to authenticate the client.
		// https://developers.pricempire.com/
		pricempireAPIClient.query.Add("api_key", key)
	}
	for _, option := range options {
		option(pricempireAPIClient)
	}
	return pricempireAPIClient, nil
}
