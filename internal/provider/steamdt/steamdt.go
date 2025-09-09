package steamdt

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "sort"
    "strings"
    "time"

    "priceprovider/internal/httpx"
    "priceprovider/internal/provider"
    "sync"
)

type Config struct {
    Name        string
    URL         string
    Method      string
    Headers     map[string]string
    Currency    string
    SymbolMap   map[string]string
    IncludeBids bool
    // MaxItemsPerRequest splits large symbol lists into smaller batch API requests.
    // 0 or negative means no limit (single request).
    MaxItemsPerRequest int
    // MaxConcurrency limits concurrent batch requests when splitting.
    // Defaults to 1 when <= 0.
    MaxConcurrency int
}

type Provider struct {
    cfg    Config
    client *httpx.Client
}

func New(cfg Config, hc *httpx.Client) *Provider {
    if cfg.Name == "" { cfg.Name = "SteamDT" }
    if cfg.URL == "" { cfg.URL = "https://open.steamdt.com/open/cs2/v1/price/batch" }
    if cfg.Method == "" { cfg.Method = http.MethodPost }
    if cfg.Currency == "" { cfg.Currency = "CNY" }
    return &Provider{cfg: cfg, client: hc}
}

func (p *Provider) Name() string { return p.cfg.Name }

func (p *Provider) Fetch(ctx context.Context, symbols []string) ([]provider.Quote, error) {
    // map requested symbols -> provider keys, keep unique provider keys for batching
    keyByAgg := make(map[string]string, len(symbols))
    uniqSet := make(map[string]struct{}, len(symbols))
    uniqKeys := make([]string, 0, len(symbols))
    for _, s := range symbols {
        key := s
        if v := p.cfg.SymbolMap[s]; v != "" { key = v }
        keyByAgg[s] = key
        if _, ok := uniqSet[key]; !ok {
            uniqSet[key] = struct{}{}
            uniqKeys = append(uniqKeys, key)
        }
    }

    // perform one or more batch requests as needed
    byMarketAll := make(map[string]entry, len(uniqKeys))
    var firstErr error

    doBatch := func(ctx context.Context, keys []string) error {
        payload := map[string]any{"marketHashNames": keys}
        body, _ := json.Marshal(payload)
        req, err := http.NewRequestWithContext(ctx, p.cfg.Method, p.cfg.URL, bytes.NewReader(body))
        if err != nil { return err }
        req.Header.Set("Content-Type", "application/json")
        for k, v := range p.cfg.Headers { req.Header.Set(k, v) }
        resp, err := p.client.Do(ctx, req)
        if err != nil { return err }
        defer resp.Body.Close()
        if resp.StatusCode < 200 || resp.StatusCode >= 300 {
            b, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<10))
            return fmt.Errorf("%s %s -> %d: %s", p.cfg.Method, p.cfg.URL, resp.StatusCode, string(b))
        }
        dec := json.NewDecoder(resp.Body)
        dec.UseNumber()
        var api apiResponse
        if err := dec.Decode(&api); err != nil { return fmt.Errorf("decode: %w", err) }
        if !api.Success && (api.ErrorCode != 0 || strings.TrimSpace(api.ErrorMsg) != "") && len(api.Data) == 0 {
            return fmt.Errorf("provider error: code=%d msg=%q", api.ErrorCode, api.ErrorMsg)
        }
        for _, e := range api.Data { byMarketAll[e.MarketHashName] = e }
        return nil
    }

    batchSize := p.cfg.MaxItemsPerRequest
    if batchSize <= 0 || len(uniqKeys) <= batchSize {
        // single request path
        if err := doBatch(ctx, uniqKeys); err != nil { firstErr = err }
    } else {
        // concurrent batched requests with a limit
        batches := chunkStrings(uniqKeys, batchSize)
        maxConc := p.cfg.MaxConcurrency
        if maxConc <= 0 { maxConc = 1 }
        sem := make(chan struct{}, maxConc)
        var wg sync.WaitGroup
        var mu sync.Mutex
        for _, b := range batches {
            b := b
            wg.Add(1)
            go func() {
                defer wg.Done()
                select {
                case sem <- struct{}{}:
                    defer func() { <-sem }()
                case <-ctx.Done():
                    return
                }
                if err := doBatch(ctx, b); err != nil {
                    mu.Lock()
                    if firstErr == nil { firstErr = err }
                    mu.Unlock()
                }
            }()
        }
        wg.Wait()
    }

    now := time.Now().UTC()
    out := make([]provider.Quote, 0, len(symbols)*4)
    for _, aggSym := range symbols {
        provKey := keyByAgg[aggSym]
        if e, ok := byMarketAll[provKey]; ok {
            cs := collectCandidates(e.DataList, now)
            for _, c := range cs {
                out = append(out, provider.Quote{
                    Symbol:     aggSym,
                    Price:      c.sell,
                    Currency:   p.cfg.Currency,
                    Source:     fmt.Sprintf("%s:%s:sell", p.cfg.Name, c.platform),
                    ReceivedAt: c.ts,
                })
                if p.cfg.IncludeBids && c.bid != "" && c.bid != "0" && c.bid != "0.0" {
                    out = append(out, provider.Quote{
                        Symbol:     aggSym,
                        Price:      c.bid,
                        Currency:   p.cfg.Currency,
                        Source:     fmt.Sprintf("%s:%s:bid", p.cfg.Name, c.platform),
                        ReceivedAt: c.ts,
                    })
                }
            }
        }
    }
    if len(out) == 0 && firstErr != nil {
        return nil, firstErr
    }
    return out, nil
}

type entry struct {
    MarketHashName string    `json:"marketHashName"`
    DataList       []listing `json:"dataList"`
}

type listing struct {
    Platform       string      `json:"platform"`
    PlatformItemID string      `json:"platformItemId"`
    SellPrice      json.Number `json:"sellPrice"`
    SellCount      int         `json:"sellCount"`
    BiddingPrice   json.Number `json:"biddingPrice"`
    BiddingCount   int         `json:"biddingCount"`
    UpdateTime     int64       `json:"updateTime"`
}

type apiResponse struct {
    Success      bool    `json:"success"`
    Data         []entry `json:"data"`
    ErrorCode    int     `json:"errorCode"`
    ErrorMsg     string  `json:"errorMsg"`
    ErrorData    any     `json:"errorData"`
    ErrorCodeStr string  `json:"errorCodeStr"`
}

type candidate struct {
    platform string
    sell     string
    bid      string
    ts       time.Time
}

func collectCandidates(list []listing, now time.Time) []candidate {
    cs := make([]candidate, 0, len(list))
    for _, d := range list {
        sel := numToString(d.SellPrice)
        bid := numToString(d.BiddingPrice)
        if (sel == "" || sel == "0" || sel == "0.0") && (bid == "" || bid == "0" || bid == "0.0") {
            continue
        }
        ts := parseEpochMaybeMillis(d.UpdateTime, now)
        cs = append(cs, candidate{platform: d.Platform, sell: sel, bid: bid, ts: ts})
    }
    sort.Slice(cs, func(i, j int) bool {
        if cs[i].platform == cs[j].platform {
            return cs[i].ts.Before(cs[j].ts)
        }
        return cs[i].platform < cs[j].platform
    })
    return cs
}

func numToString(n json.Number) string {
    s := strings.TrimSpace(n.String())
    return s
}

func parseEpochMaybeMillis(v int64, fallback time.Time) time.Time {
    if v <= 0 { return fallback }
    if v > 1_000_000_000_000 { // ms
        return time.UnixMilli(v).UTC()
    }
    return time.Unix(v, 0).UTC()
}

func chunkStrings(in []string, size int) [][]string {
    if size <= 0 || len(in) == 0 { return [][]string{in} }
    out := make([][]string, 0, (len(in)+size-1)/size)
    for i := 0; i < len(in); i += size {
        j := i + size
        if j > len(in) { j = len(in) }
        out = append(out, in[i:j])
    }
    return out
}
