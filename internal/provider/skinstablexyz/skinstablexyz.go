package skinstablexyz

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "net/url"
    "time"

    "priceprovider/internal/httpx"
    "priceprovider/internal/provider"
)

// Config controls the SkinstableXYZ provider behavior.
type Config struct {
    Name                 string
    URL                  string
    Currency             string
    APIKey               string            // optional; if set, sent as Bearer token
    Headers              map[string]string // optional extra headers
    ItemsCacheTTLSeconds int               // cache the full items payload for this long
    AppID                int               // required app/game id (e.g., 730)
    Sites                []string          // list of sites to query (e.g., ["CS.MONEY","BUFF.163"]) 
}

// Provider fetches price data from SkinstableXYZ.
// It pulls the aggregated items payload and filters by requested symbols.
type Provider struct {
    cfg    Config
    client *httpx.Client

    // cached full items payload
    cache map[string]siteCache // key: site -> items + expiry
}

func New(cfg Config, hc *httpx.Client) *Provider {
    if cfg.Name == "" { cfg.Name = "SkinstableXYZ" }
    if cfg.Currency == "" { cfg.Currency = "USD" }
    return &Provider{cfg: cfg, client: hc}
}

func (p *Provider) Name() string { return p.cfg.Name }

func (p *Provider) Fetch(ctx context.Context, symbols []string) ([]provider.Quote, error) {
    if p.cfg.URL == "" {
        return nil, fmt.Errorf("skinstable: missing URL")
    }
    if p.cfg.AppID == 0 { p.cfg.AppID = 730 }
    if len(p.cfg.Sites) == 0 { p.cfg.Sites = []string{"CS.MONEY"} }

    now := time.Now()
    if p.cache == nil { p.cache = make(map[string]siteCache, len(p.cfg.Sites)) }
    // Ensure cache for each site
    for _, site := range p.cfg.Sites {
        sc, ok := p.cache[site]
        if !ok || now.After(sc.until) {
            items, until, err := p.fetchSite(ctx, site)
            if err != nil { return nil, err }
            p.cache[site] = siteCache{items: items, until: until}
        }
    }

    out := make([]provider.Quote, 0, len(symbols))
    for _, s := range symbols {
        for _, site := range p.cfg.Sites {
            sc := p.cache[site]
            it, ok := sc.items[s]
            if !ok || it.P == nil { continue }
            ts := parseEpochMaybeMillis(it.T, now)
            out = append(out, provider.Quote{
                Symbol:     s,
                Price:      formatFloat(*it.P),
                Currency:   p.cfg.Currency,
                Source:     fmt.Sprintf("%s:%s", p.cfg.Name, site),
                ReceivedAt: ts,
            })
        }
    }
    return out, nil
}

type siteCache struct {
    items map[string]item
    until time.Time
}

func (p *Provider) fetchSite(ctx context.Context, site string) (map[string]item, time.Time, error) {
    u, err := url.Parse(p.cfg.URL)
    if err != nil { return nil, time.Time{}, err }
    q := u.Query()
    if p.cfg.APIKey != "" { q.Set("apikey", p.cfg.APIKey) }
    if p.cfg.AppID > 0 { q.Set("app", fmt.Sprintf("%d", p.cfg.AppID)) }
    if site != "" { q.Set("site", site) }
    u.RawQuery = q.Encode()

    req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody)
    if err != nil { return nil, time.Time{}, err }
    for k, v := range p.cfg.Headers { req.Header.Set(k, v) }
    req.Header.Set("Accept", "application/json")
    resp, err := p.client.Do(ctx, req)
    if err != nil { return nil, time.Time{}, err }
    defer resp.Body.Close()
    if resp.StatusCode < 200 || resp.StatusCode >= 300 {
        return nil, time.Time{}, fmt.Errorf("GET %s -> %d", u.String(), resp.StatusCode)
    }
    var body apiResponse
    dec := json.NewDecoder(resp.Body)
    if err := dec.Decode(&body); err != nil { return nil, time.Time{}, fmt.Errorf("decode: %w", err) }
    ttl := time.Duration(p.cfg.ItemsCacheTTLSeconds) * time.Second
    if ttl <= 0 { ttl = 10 * time.Second }
    return body.Items, time.Now().Add(ttl), nil
}

// Response model based on the provided sample.
type apiResponse struct {
    Items     map[string]item `json:"items"`
    Time      int             `json:"time"`
    Requests  int             `json:"requests"`
}

type item struct {
    N string   `json:"n"`
    P *float64 `json:"p"`
    C *int     `json:"c"`
    M *int     `json:"m"`
    B *int     `json:"b"`
    O *int     `json:"o"`
    F json.RawMessage `json:"f"`
    T int64    `json:"t"` // epoch millis
}

func formatFloat(v float64) string { return fmt.Sprintf("%g", v) }

func parseEpochMaybeMillis(v int64, fallback time.Time) time.Time {
    if v <= 0 { return fallback }
    if v > 1_000_000_000_000 { // ms
        return time.UnixMilli(v).UTC()
    }
    return time.Unix(v, 0).UTC()
}
