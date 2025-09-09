package pricempire

import (
    "context"
    "encoding/json"
    "fmt"
    "maps"
    "net/http"
    "net/http/httputil"
    "strconv"
    "time"
)

// Item represents an item from the Pricempire API.
type Item struct {
	Name      string
	Liquidity *float64
	Prices    map[string]Price
}

// Price represents the price of an item from a source.
type Price struct {
	Price     *float64
	Count     *float64
	Avg30     *float64
	Inflated  *bool
	CreatedAt *time.Time
}

// GetAllItemsV3 retrieves all items from the Pricempire API.
func (c *PricempireAPIClient) GetAllItemsV3(ctx context.Context, appID int, currency string, sources []string, opts ...PricempireAPIClientOption) ([]Item, error) {
	var override = &PricempireAPIClient{
		baseURL:    c.baseURL,
		httpClient: c.httpClient,
		header:     c.header.Clone(),
		query:      c.query,
	}
	for _, opt := range opts {
		opt(override)
	}

	query := maps.Clone(override.query)
	query.Add("appId", strconv.Itoa(appID))
	query.Add("currency", currency)
	for _, source := range sources {
		query.Add("sources", source)
	}

	url := fmt.Sprintf("%s/v3/items/prices?%s", override.baseURL, query.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header = override.header

	res, err := override.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("performing request: %w", err)
	}
	defer res.Body.Close()

	switch res.StatusCode {
	case http.StatusOK:
		break

	case http.StatusBadRequest:
		b, err := json.Marshal(sources)
		if err != nil {
			return nil, fmt.Errorf("bad request with sources=%v", sources)
		}
		return nil, fmt.Errorf("bad request with sources=%s", string(b))

	case http.StatusForbidden:
		return nil, fmt.Errorf("unauthorized")

	case http.StatusTooManyRequests:
		return nil, fmt.Errorf("rate limited")

	default:
		return nil, fmt.Errorf("unexpected status code: %d", res.StatusCode)
	}

    var body map[string]any
    dec := json.NewDecoder(res.Body)
    if err := dec.Decode(&body); err != nil {
        b2, _ := httputil.DumpResponse(res, true)
        return nil, fmt.Errorf("decoding listings response: %w with %s", err, string(b2))

        // return nil, fmt.Errorf("decoding listings response: %w with %s", err, string(b))
    }

	var items = []Item{}
	for name, raw := range body {
		// {
		//   "liquidity": 53.555,
		//   "buff": {
		//     "isInflated": false,
		//     "price": 32,
		//     "count": 43,
		//     "avg30": 28,
		//     "createdAt": "2023-02-02T12:13:07.393Z"
		//   }
		// }
		item, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("decoding item: %w", err)
		}

		liquidity, err := parseNullableValue[float64](item, "liquidity")
		if err != nil {
			return nil, fmt.Errorf("decoding liquidity: %w", err)
		}

		var prices = map[string]Price{}
		for _, source := range sources {
			// {
			//   "price": 32,
			//   "count": 43,
			//   "avg30": 28,
			//   "isInflated": false,
			//   "createdAt": "2023-02-02T12:13:07.393Z"
			// }
			dataVal, ok := item[source]
			if !ok || dataVal == nil {
				// The source is not present in the response.
				continue
			}

			data, ok := dataVal.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("decoding %s: %w", source, err)
			}

			price, err := parseNullableValue[float64](data, "price")
			if err != nil {
				return nil, fmt.Errorf("decoding price: %w", err)
			}

			count, err := parseNullableValue[float64](data, "count")
			if err != nil {
				return nil, fmt.Errorf("decoding count: %w", err)
			}

			avg30, err := parseNullableValue[float64](data, "avg30")
			if err != nil {
				return nil, fmt.Errorf("decoding avg30: %w", err)
			}

			inflated, err := parseNullableValue[bool](data, "isInflated")
			if err != nil {
				return nil, fmt.Errorf("decoding inflated: %w", err)
			}

			createdAtStr, err := parseNullableValue[string](data, "createdAt")
			if err != nil {
				return nil, fmt.Errorf("decoding createdAt: %w", err)
			}
			var createdAt *time.Time
			if createdAtStr != nil {
				t, err := time.Parse(time.RFC3339, *createdAtStr)
				if err != nil {
					return nil, fmt.Errorf("decoding createdAt: %w", err)
				}
				createdAt = &t
			}

			prices[source] = Price{
				Price:     price,
				Count:     count,
				Avg30:     avg30,
				Inflated:  inflated,
				CreatedAt: createdAt,
			}
		}

		items = append(items, Item{
			Name:      name,
			Liquidity: liquidity,
			Prices:    prices,
		})
	}

	return items, nil
}

// parseNullableValue is a helper function to parse a nullable value.
func parseNullableValue[T any](data map[string]any, key string) (*T, error) {
	v, ok := data[key]
	if !ok || v == nil {
		return nil, nil
	}
	if v, ok := v.(T); ok {
		return &v, nil
	}
	return nil, fmt.Errorf("unexpected type: %T", v)
}
