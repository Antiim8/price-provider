package httpx

import (
    "context"
    "net"
    "net/http"
    "time"
)

// Client is a small wrapper around http.Client with sane defaults.
type Client struct {
    HTTP      *http.Client
    UserAgent string
    Headers   map[string]string
}

func New(timeout time.Duration) *Client {
    transport := &http.Transport{
        Proxy: http.ProxyFromEnvironment,
        DialContext: (&net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
        MaxIdleConns:          200,
        MaxIdleConnsPerHost:   100,
        MaxConnsPerHost:       100,
        ForceAttemptHTTP2:     true,
        IdleConnTimeout:       90 * time.Second,
        TLSHandshakeTimeout:   3 * time.Second,
        ExpectContinueTimeout: 1 * time.Second,
        ResponseHeaderTimeout: 5 * time.Second,
    }
    return &Client{HTTP: &http.Client{Timeout: timeout, Transport: transport}, UserAgent: "price-provider/1.0"}
}

func (c *Client) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
    if c.UserAgent != "" && req.Header.Get("User-Agent") == "" {
        req.Header.Set("User-Agent", c.UserAgent)
    }
    for k, v := range c.Headers {
        if req.Header.Get(k) == "" {
            req.Header.Set(k, v)
        }
    }
    return c.HTTP.Do(req)
}
