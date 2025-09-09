package pricempire_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	pricempire "priceprovider/internal/provider/pricempire"
)

func TestGetAllItemsV3(t *testing.T) {
	t.Parallel()

	// Arrange: create a mock controller
	ctrl := gomock.NewController(t)

	// Arrange: create a mock HTTP client
	httpClient := NewMockHTTPClient(ctrl)

	// Assert: stub the Do method
	httpClient.EXPECT().
		Do(gomock.Any()).
		DoAndReturn(func(req *http.Request) (*http.Response, error) {
			require.Equal(t, http.MethodGet, req.Method)
			require.Equal(t, "test-key", req.URL.Query().Get("api_key"))
			require.Contains(t, req.URL.Path, "/v3/items/prices")
			require.Contains(t, req.URL.RawQuery, "appId=730")
			require.Contains(t, req.URL.RawQuery, "currency=USD")
			require.Contains(t, req.URL.RawQuery, "sources=buff")

			buffer := &bytes.Buffer{}
			require.NoError(t, json.NewEncoder(buffer).Encode(mockItemsResponse))

			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(buffer),
			}, nil
		}).
		Times(1)

	// Arrange: setup a new Pricempire API client
	client, err := pricempire.NewPricempireAPIClient("test-key", pricempire.WithHTTPClient(httpClient))
	require.NoError(t, err)
	require.NotNil(t, client)

	// Act: call GetAllItemsV3
	items, err := client.GetAllItemsV3(t.Context(), 730, "USD", []string{"buff"})
	require.NoError(t, err)
	require.NotNil(t, items)

	// Assert: items should be unmarshalled from the mock response
	require.Len(t, mockItems, len(items))
	require.Equal(t, mockItems[0].Name, items[0].Name)
	require.InEpsilon(t, *mockItems[0].Liquidity, *items[0].Liquidity, 0.0001)
	require.InEpsilon(t, *mockItems[0].Prices["buff"].Price, *items[0].Prices["buff"].Price, 0.0001)
}

func TestGetAllItemsV3_ErrCreatingRequest(t *testing.T) {
	t.Parallel()

	// Arrange: create a mock controller
	ctrl := gomock.NewController(t)

	// Arrange: create a mock HTTP client
	httpClient := NewMockHTTPClient(ctrl)

	// Assert: stub the Do method
	httpClient.EXPECT().
		Do(gomock.Any()).
		Times(0)

	// Arrange: setup a new Pricempire API client
	client, err := pricempire.NewPricempireAPIClient("", pricempire.WithHTTPClient(httpClient))
	require.NoError(t, err)
	require.NotNil(t, client)

	// Act: call GetAllItemsV3 with a nil context
	items, err := client.GetAllItemsV3(t.Context(), 730, "USD", []string{"buff"}, pricempire.WithBaseURL(string([]rune{0x7f})))
	require.Error(t, err)
	require.Nil(t, items)
}

func TestGetAllItemsV3_ErrPerformingRequest(t *testing.T) {
	t.Parallel()

	// Arrange: create a mock controller
	ctrl := gomock.NewController(t)

	// Arrange: create a mock HTTP client
	httpClient := NewMockHTTPClient(ctrl)

	// Assert: stub the Do method
	httpClient.EXPECT().
		Do(gomock.Any()).
		DoAndReturn(func(req *http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("error")
		}).
		Times(1)

	// Arrange: setup a new Pricempire API client
	client, err := pricempire.NewPricempireAPIClient("", pricempire.WithHTTPClient(httpClient))
	require.NoError(t, err)
	require.NotNil(t, client)

	// Act: call GetAllItemsV3
	items, err := client.GetAllItemsV3(t.Context(), 730, "USD", []string{"buff"})
	require.Error(t, err)
	require.Nil(t, items)
}

func TestGetAllItemsV3_ErrUnexpectedStatusCode(t *testing.T) {
	t.Parallel()

	// Arrange: create a mock controller
	ctrl := gomock.NewController(t)

	// Arrange: create a mock HTTP client
	httpClient := NewMockHTTPClient(ctrl)

	// Assert: stub the Do method
	httpClient.EXPECT().
		Do(gomock.Any()).
		DoAndReturn(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(bytes.NewReader([]byte{})),
			}, nil
		}).
		Times(1)

	// Arrange: setup a new Pricempire API client
	client, err := pricempire.NewPricempireAPIClient("", pricempire.WithHTTPClient(httpClient))
	require.NoError(t, err)
	require.NotNil(t, client)

	// Act: call GetAllItemsV3
	items, err := client.GetAllItemsV3(t.Context(), 730, "USD", []string{"buff"})
	require.Error(t, err)
	require.Nil(t, items)
}

func TestGetAllItemsV3_ErrDecodingResponse(t *testing.T) {
	t.Parallel()

	// Arrange: create a mock controller
	ctrl := gomock.NewController(t)

	// Arrange: create a mock HTTP client
	httpClient := NewMockHTTPClient(ctrl)

	// Assert: stub the Do method
	httpClient.EXPECT().
		Do(gomock.Any()).
		DoAndReturn(func(req *http.Request) (*http.Response, error) {
			buffer := &bytes.Buffer{}
			buffer.WriteString("invalid json")

			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(buffer),
			}, nil
		}).
		Times(1)

	// Arrange: setup a new Pricempire API client
	client, err := pricempire.NewPricempireAPIClient("", pricempire.WithHTTPClient(httpClient))
	require.NoError(t, err)
	require.NotNil(t, client)

	// Act: call GetAllItemsV3
	items, err := client.GetAllItemsV3(t.Context(), 730, "USD", []string{"buff"})
	require.Error(t, err)
	require.Nil(t, items)
}

func TestGetAllItemsV3_WithFixture(t *testing.T) {
	t.Parallel()

	// Arrange: create a mock controller
	ctrl := gomock.NewController(t)

	// Load the fixture data
	fixtureData, err := os.OpenFile("fixtures/get_all_items_v3.json", os.O_RDONLY, 0600)
	require.NoError(t, err)

	// Arrange: create a mock HTTP client
	httpClient := NewMockHTTPClient(ctrl)

	// Assert: stub the Do method
	httpClient.EXPECT().
		Do(gomock.Any()).
		DoAndReturn(func(req *http.Request) (*http.Response, error) {
			require.Equal(t, http.MethodGet, req.Method)
			require.Equal(t, "test-key", req.URL.Query().Get("api_key"))
			require.Contains(t, req.URL.Path, "/v3/items/prices")
			require.Contains(t, req.URL.RawQuery, "appId=730")
			require.Contains(t, req.URL.RawQuery, "currency=USD")
			require.Contains(t, req.URL.RawQuery, "sources=buff")

			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       fixtureData,
			}, nil
		}).
		Times(1)

	// Arrange: setup a new Pricempire API client
	client, err := pricempire.NewPricempireAPIClient("test-key", pricempire.WithHTTPClient(httpClient))
	require.NoError(t, err)
	require.NotNil(t, client)

	// Act: call GetAllItemsV3
	items, err := client.GetAllItemsV3(t.Context(), 730, "USD", []string{"buff", "buff_avg30"})
	require.NoError(t, err)
	require.NotNil(t, items)

	// Assert: items should be unmarshalled from the fixture data
	require.Lenf(t, items, 24282, "expected 24282 items, got %d", len(items))

	// Assert: the item should be returned.
	ak47Hydroponic := findItem(items, "AK-47 | Hydroponic (Battle-Scarred)")
	require.NotNilf(t, ak47Hydroponic, "expected AK-47 | Hydroponic (Battle-Scarred) item to be found")

	// Assert: the item should have a liquidity
	require.InEpsilon(t, 55.565525383707204, *ak47Hydroponic.Liquidity, 0.0001)

	// Assert: the item should have a buff price
	buffPrice := ak47Hydroponic.Prices["buff"]
	require.NotNilf(t, buffPrice, "expected buff price to be found: %+v", ak47Hydroponic.Prices)
	require.NotNilf(t, buffPrice.Price, "expected price to be set, got nil")
	require.InEpsilon(t, 48144.0, *buffPrice.Price, 0.0001)
	require.NotNilf(t, buffPrice.Count, "expected count to be set, got nil")
	require.InEpsilon(t, 7.0, *buffPrice.Count, 0.0001)
	require.NotNilf(t, buffPrice.Avg30, "expected avg30 to be set, got nil")
	require.InEpsilon(t, 49143.0, *buffPrice.Avg30, 0.0001)
	require.NotNilf(t, buffPrice.Inflated, "expected inflated to be set, got nil")
	require.False(t, *buffPrice.Inflated)
	require.NotNilf(t, buffPrice.CreatedAt, "expected created_at to be set, got nil")
	require.Equal(t, time.Date(2024, 7, 17, 13, 53, 47, 857000000, time.UTC), *buffPrice.CreatedAt)

	// Assert: the item should have a buff_avg30 price
	buffAvg30Price := ak47Hydroponic.Prices["buff_avg30"]
	require.NotNilf(t, buffAvg30Price, "expected buff_avg30 price to be found: %+v", ak47Hydroponic.Prices)
	require.NotNilf(t, buffAvg30Price.Price, "expected price to be set, got nil")
	require.InEpsilon(t, 49143.0, *buffAvg30Price.Price, 0.0001)
	require.Nil(t, buffAvg30Price.Count)
	require.Nil(t, buffAvg30Price.Avg30)
	require.NotNilf(t, buffAvg30Price.Inflated, "expected inflated to be set, got nil")
	require.Falsef(t, *buffAvg30Price.Inflated, "expected inflated to be false, got %v", *buffAvg30Price.Inflated)
	require.Nil(t, buffAvg30Price.CreatedAt)
}

// findItem is a helper function to find an item by name
func findItem(items []pricempire.Item, name string) *pricempire.Item {
	for _, item := range items {
		if item.Name == name {
			return &item
		}
	}
	return nil
}

// mockItemsResponse is a mock response from the Pricempire API
var mockItemsResponse = map[string]any{
	"item1": map[string]any{
		"liquidity": 53.555,
		"buff": map[string]any{
			"isInflated": false,
			"price":      32.0,
			"count":      43,
			"avg30":      28,
			"createdAt":  "2023-02-02T12:13:07.393Z",
		},
	},
}

// mockItems is a map containing mock items from the Pricempire API
var mockItems = []pricempire.Item{
	{
		Name:      "item1",
		Liquidity: toPtr(53.555),
		Prices: map[string]pricempire.Price{
			"buff": {
				Price:     toPtr(32.0),
				Count:     toPtr(43.0),
				Avg30:     toPtr(28.0),
				Inflated:  toPtr(false),
				CreatedAt: toPtr(time.Date(2023, 2, 2, 12, 13, 7, 393000000, time.UTC)),
			},
		},
	},
}

// toPtr is a small local helper to create pointers to literal values in tests.
func toPtr[T any](v T) *T { return &v }
