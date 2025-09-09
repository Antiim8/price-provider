package pricempire_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	pricempire "priceprovider/internal/provider/pricempire"
)

func TestNewPricempireAPIClient(t *testing.T) {
	t.Parallel()

	// Assert: a valid key should return a client.
	client, err := pricempire.NewPricempireAPIClient("test")
	require.NoErrorf(t, err, "unexpected error: %v", err)
	require.NotNilf(t, client, "unexpected nil client")
}

func TestWithHTTPClient(t *testing.T) {
	t.Parallel()

	// Arrange: create a mock controller
	ctrl := gomock.NewController(t)

	// Arrange: create a mock http client
	httpClient := NewMockHTTPClient(ctrl)

	// Assert: stub the Do method to set called to true
	httpClient.EXPECT().
		Do(gomock.Any()).
		DoAndReturn(func(req *http.Request) (*http.Response, error) {
			buffer := &bytes.Buffer{}
			require.NoError(t, json.NewEncoder(buffer).Encode(map[string]any{}))

			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(buffer),
			}, nil
		}).
		Times(1)

	// Arrange: create a new client with a custom HTTP client.
	client, err := pricempire.NewPricempireAPIClient("test", pricempire.WithHTTPClient(httpClient))
	require.NoError(t, err)
	require.NotNil(t, client)

	// Act: call GetAllItemsV3 with the custom HTTP client.
	client.GetAllItemsV3(t.Context(), 730, "USD", []string{"csgofloat"})
}

func TestWithBaseURL(t *testing.T) {
	t.Parallel()

	// Arrange: create a mock controller
	ctrl := gomock.NewController(t)

	// Arrange: create a mock http client
	httpClient := NewMockHTTPClient(ctrl)

	// Arrange: define a base url
	baseURL := "http://localhost:8080"

	// Assert: stub the Do method
	httpClient.EXPECT().
		Do(gomock.Any()).
		DoAndReturn(func(req *http.Request) (*http.Response, error) {
			require.Truef(t, strings.HasPrefix(req.URL.String(), baseURL), "expected url to start with base url, received: %s", req.URL.String())

			buffer := &bytes.Buffer{}
			require.NoError(t, json.NewEncoder(buffer).Encode(map[string]any{}))

			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(buffer),
			}, nil
		}).
		Times(1)

	// Arrange: create a new client.
	client, err := pricempire.NewPricempireAPIClient("test", pricempire.WithHTTPClient(httpClient), pricempire.WithBaseURL(baseURL))
	require.NoError(t, err)
	require.NotNil(t, client)

	// Act: call GetAllItemsV3 with the overridden base URL.
	client.GetAllItemsV3(t.Context(), 730, "USD", []string{
		"csgofloat",
	})
}

func TestWithHeader(t *testing.T) {
	t.Parallel()

	// Arrange: create a mock controller
	ctrl := gomock.NewController(t)

	// Arrange: create a mock http client
	httpClient := NewMockHTTPClient(ctrl)

	// Assert: stub the Do method to set called to true
	httpClient.EXPECT().
		Do(gomock.Any()).
		DoAndReturn(func(req *http.Request) (*http.Response, error) {
			require.Equal(t, "bar", req.Header.Get("foo"))

			buffer := &bytes.Buffer{}
			require.NoError(t, json.NewEncoder(buffer).Encode(map[string]any{}))
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(buffer),
			}, nil
		}).
		Times(1)

	// Arrange: create a new client with a custom header.
	client, err := pricempire.NewPricempireAPIClient("test", pricempire.WithHTTPClient(httpClient), pricempire.WithHeader(http.Header{
		"foo": []string{"bar"},
	}))
	require.NoError(t, err)
	require.NotNil(t, client)

	// Act: call GetAllItemsV3 with the custom header.
	client.GetAllItemsV3(t.Context(), 730, "USD", []string{
		"csgofloat",
	})
}
