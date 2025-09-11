package skinstablexyz

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "net/url"
    "strconv"
    "sync"
    "time"

    "priceprovider/internal/httpx"
    "priceprovider/internal/provider"
    "golang.org/x/sync/singleflight"
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
    cache   map[string]siteCache // key: site -> items + expiry
    cacheMu sync.RWMutex

    // coalesce concurrent refreshes per-site
    sf singleflight.Group
}

func New(cfg Config, hc *httpx.Client) *Provider {
    if cfg.Name == "" { cfg.Name = "SkinstableXYZ" }
    if cfg.Currency == "" { cfg.Currency = "USD" }
    return &Provider{cfg: cfg, client: hc}
}

func (p *Provider) Name() string { return p.cfg.Name }

func (p *Provider) Fetch(ctx context.Context, symbols []string) ([]provider.Quote, error) {
    // a) Config guards
    if p.cfg.URL == "" {
        return nil, fmt.Errorf("skinstable: missing URL")
    }
    if p.cfg.AppID == 0 {
        p.cfg.AppID = 730
    }
    if len(p.cfg.Sites) == 0 {
        p.cfg.Sites = []string{"CS.MONEY"}
    }

    now := time.Now()

    // b) Lazy init cache under write lock
    p.cacheMu.Lock()
    if p.cache == nil {
        p.cache = make(map[string]siteCache, len(p.cfg.Sites))
    }
    p.cacheMu.Unlock()

    // c) Double-checked refresh per site
    var anyValid bool
    var lastErr error
    for _, site := range p.cfg.Sites {
        // Read snapshot of current entry
        p.cacheMu.RLock()
        sc, ok := p.cache[site]
        expired := !ok || now.After(sc.until)
        if ok && !expired {
            anyValid = true
        }
        p.cacheMu.RUnlock()

        if expired {
            // Coalesce refreshes per-site to avoid duplicate upstream calls
            type result struct {
                items map[string]item
                until time.Time
            }
            v, err, _ := p.sf.Do(site, func() (any, error) {
                perSiteCtx, cancel := context.WithTimeout(ctx, 7*time.Second)
                defer cancel()
                items, until, err := p.fetchSite(perSiteCtx, site)
                if err != nil { return nil, err }
                return result{items: items, until: until}, nil
            })
            if err != nil {
                // Record last error; continue to check other sites
                lastErr = err
            } else {
                res := v.(result)
                // Write new snapshot if still expired/missing (use fresh time)
                p.cacheMu.Lock()
                sc2, ok2 := p.cache[site]
                if !ok2 || time.Now().After(sc2.until) {
                    p.cache[site] = siteCache{items: res.items, until: res.until}
                }
                p.cacheMu.Unlock()
                anyValid = true
            }
        }
    }

    if !anyValid {
        if lastErr != nil {
            return nil, lastErr
        }
        return nil, fmt.Errorf("skinstable: no data from any site")
    }

    // d) Snapshot caches for lock-free reads
    type siteSnapshot struct {
        site string
        sc   siteCache
    }
    snaps := make([]siteSnapshot, 0, len(p.cfg.Sites))
    p.cacheMu.RLock()
    for _, site := range p.cfg.Sites {
        if sc, ok := p.cache[site]; ok {
            snaps = append(snaps, siteSnapshot{site: site, sc: sc})
        }
    }
    p.cacheMu.RUnlock()

    out := make([]provider.Quote, 0, len(symbols))
    for _, snap := range snaps {
        for _, s := range symbols {
            it, ok := snap.sc.items[s]
            if !ok || it.P == nil {
                continue
            }
            ts := parseEpochMaybeMillis(it.T, now)
            out = append(out, provider.Quote{
                Symbol:     s,
                Price:      formatFloat(*it.P),
                Currency:   p.cfg.Currency,
                Source:     fmt.Sprintf("%s:%s", p.cfg.Name, snap.site),
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
    P *float64 `json:"p"`
    T int64    `json:"t"`
}

func formatFloat(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

func parseEpochMaybeMillis(v int64, fallback time.Time) time.Time {
    if v <= 0 { return fallback }
    if v > 1_000_000_000_000 { // ms
        return time.UnixMilli(v).UTC()
    }
    return time.Unix(v, 0).UTC()
}
