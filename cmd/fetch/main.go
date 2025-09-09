package main

import (
    "context"
    "encoding/json"
    "flag"
    "fmt"
    "log"
    "net/http"
    "os"
    "strings"
    "time"

    "priceprovider/internal/config"
    "priceprovider/internal/httpx"
    "priceprovider/internal/provider"
    "priceprovider/internal/provider/ratelimit"
    "priceprovider/internal/provider/cache"
    pricempirepkg "priceprovider/internal/provider/pricempire"
    "priceprovider/internal/provider/pricempireadapter"
    "priceprovider/internal/provider/steamdt"
    "priceprovider/internal/provider/skinstablexyz"
)

func main() {
    var symbolsCSV string
    var includeBids bool
    var steamCurrency string
    var peCurrency string
    var peSourcesCSV string
    var peAppID int
    var timeout int
    var configPath string

    flag.StringVar(&symbolsCSV, "symbols", getenv("SYMBOLS", "AK-47 | Redline (Field-Tested)"), "comma-separated marketHashNames")
    flag.BoolVar(&includeBids, "include-bids", getenvBool("INCLUDE_BIDS", true), "include Pricempire bids where available (N/A) and SteamDT bids")
    flag.StringVar(&steamCurrency, "steam-currency", getenv("STEAM_CURRENCY", "CNY"), "SteamDT currency tag")
    flag.StringVar(&peCurrency, "pe-currency", getenv("PRICEMPIRE_CURRENCY", "USD"), "Pricempire currency")
    flag.StringVar(&peSourcesCSV, "pe-sources", getenv("PRICEMPIRE_SOURCES", "buff"), "Pricempire sources CSV (e.g., buff,steam,skinport)")
    flag.IntVar(&peAppID, "pe-appid", getenvInt("PRICEMPIRE_APP_ID", 730), "Pricempire app id")
    flag.IntVar(&timeout, "timeout", getenvInt("REQUEST_TIMEOUT_SEC", 15), "request timeout seconds")
    flag.StringVar(&configPath, "config", getenv("CONFIG_FILE", ""), "path to config.json (optional)")
    flag.Parse()

    // Load config (optional) and merge with flags/env
    cfg, err := config.Load(configPath)
    if err != nil { log.Fatalf("config: %v", err) }
    // Override select fields from flags where provided
    if steamCurrency != "" { cfg.SteamDT.Currency = steamCurrency }
    cfg.SteamDT.IncludeBids = includeBids
    if peCurrency != "" { cfg.Pricempire.Currency = peCurrency }
    if peSourcesCSV != "" { cfg.Pricempire.Sources = splitCSV(peSourcesCSV) }
    if peAppID != 0 { cfg.Pricempire.AppID = peAppID }
    if timeout != 0 { cfg.Server.RequestTimeoutSec = timeout }

    httpClient := httpx.New(time.Duration(cfg.Server.RequestTimeoutSec) * time.Second)

    providers := make([]provider.Provider, 0, 2)
    if cfg.SteamDT.Enabled && cfg.SteamDT.APIKey != "" {
        st := steamdt.New(steamdt.Config{
            Name:        "SteamDT",
            URL:         cfg.SteamDT.Endpoint,
            Method:      http.MethodPost,
            Headers:     map[string]string{"Authorization": "Bearer " + cfg.SteamDT.APIKey},
            Currency:    cfg.SteamDT.Currency,
            IncludeBids: cfg.SteamDT.IncludeBids,
        }, httpClient)
        var p provider.Provider = st
        if cfg.SteamDT.MaxRequestsPerMinute > 0 {
            rate := float64(cfg.SteamDT.MaxRequestsPerMinute) / 60.0
            burst := cfg.SteamDT.Burst
            if burst <= 0 { burst = 1 }
            p = &ratelimit.TokenBucketProvider{P: p, TB: ratelimit.NewTokenBucket(rate, burst)}
        } else if cfg.SteamDT.MinRequestIntervalSec > 0 {
            interval := time.Duration(cfg.SteamDT.MinRequestIntervalSec) * time.Second
            p = &ratelimit.MinInterval{P: p, Interval: interval}
        }
        if cfg.SteamDT.CacheTTLSeconds > 0 {
            p = &cache.Provider{P: p, TTL: time.Duration(cfg.SteamDT.CacheTTLSeconds) * time.Second, MaxItems: cfg.SteamDT.CacheMaxItems}
        }
        providers = append(providers, p)
    }
    if cfg.Pricempire.Enabled && cfg.Pricempire.APIKey != "" {
        peClient, err := pricempirepkg.NewPricempireAPIClient(
            cfg.Pricempire.APIKey,
            pricempirepkg.WithHTTPClient(httpClient.HTTP),
            pricempirepkg.WithHeader(http.Header{
                "User-Agent": []string{"price-provider/1.0"},
            }),
        )
        if err != nil { log.Fatalf("pricempire client: %v", err) }
        pe := pricempireadapter.New(pricempireadapter.Config{
            Name:     "Pricempire",
            AppID:    cfg.Pricempire.AppID,
            Currency: cfg.Pricempire.Currency,
            Sources:  cfg.Pricempire.Sources,
        }, peClient)
        var p provider.Provider = pe
        if cfg.Pricempire.MaxRequestsPerMinute > 0 {
            rate := float64(cfg.Pricempire.MaxRequestsPerMinute) / 60.0
            burst := cfg.Pricempire.Burst
            if burst <= 0 { burst = 1 }
            p = &ratelimit.TokenBucketProvider{P: p, TB: ratelimit.NewTokenBucket(rate, burst)}
        } else if cfg.Pricempire.MinRequestIntervalSec > 0 {
            interval := time.Duration(cfg.Pricempire.MinRequestIntervalSec) * time.Second
            p = &ratelimit.MinInterval{P: p, Interval: interval}
        }
        if cfg.Pricempire.CacheTTLSeconds > 0 {
            p = &cache.Provider{P: p, TTL: time.Duration(cfg.Pricempire.CacheTTLSeconds) * time.Second, MaxItems: cfg.Pricempire.CacheMaxItems}
        }
        providers = append(providers, p)
    }
    if cfg.Skinstable.Enabled && cfg.Skinstable.Endpoint != "" {
        stx := skinstablexyz.New(skinstablexyz.Config{
            Name:                "SkinstableXYZ",
            URL:                 cfg.Skinstable.Endpoint,
            Currency:            cfg.Skinstable.Currency,
            APIKey:              cfg.Skinstable.APIKey,
            AppID:               cfg.Skinstable.AppID,
            Sites:               cfg.Skinstable.Sites,
            ItemsCacheTTLSeconds: cfg.Skinstable.ItemsCacheTTLSeconds,
        }, httpClient)
        var p provider.Provider = stx
        if cfg.Skinstable.MaxRequestsPerMinute > 0 {
            rate := float64(cfg.Skinstable.MaxRequestsPerMinute) / 60.0
            burst := cfg.Skinstable.Burst
            if burst <= 0 { burst = 1 }
            p = &ratelimit.TokenBucketProvider{P: p, TB: ratelimit.NewTokenBucket(rate, burst)}
        } else if cfg.Skinstable.MinRequestIntervalSec > 0 {
            interval := time.Duration(cfg.Skinstable.MinRequestIntervalSec) * time.Second
            p = &ratelimit.MinInterval{P: p, Interval: interval}
        }
        if cfg.Skinstable.CacheTTLSeconds > 0 {
            p = &cache.Provider{P: p, TTL: time.Duration(cfg.Skinstable.CacheTTLSeconds) * time.Second, MaxItems: cfg.Skinstable.CacheMaxItems}
        }
        providers = append(providers, p)
    }
    if len(providers) == 0 {
        log.Fatal("no providers configured; set config.json API keys or env overrides")
    }

    symbols := splitCSV(symbolsCSV)
    if len(symbols) == 0 { log.Fatal("no symbols provided") }

    ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
    defer cancel()

    type result struct {
        name   string
        quotes []provider.Quote
        err    error
    }
    ch := make(chan result, len(providers))
    for _, p := range providers {
        p := p
        go func() {
            qs, err := p.Fetch(ctx, symbols)
            ch <- result{name: p.Name(), quotes: qs, err: err}
        }()
    }

    var all []provider.Quote
    for i := 0; i < len(providers); i++ {
        r := <-ch
        if r.err != nil {
            log.Printf("%s error: %v", r.name, r.err)
            continue
        }
        log.Printf("%s: %d quotes", r.name, len(r.quotes))
        all = append(all, r.quotes...)
    }

    if len(all) == 0 {
        log.Fatal("no quotes received")
    }

    // Print up to 10 quotes as JSON for inspection
    n := len(all)
    if n > 10 { n = 10 }
    sample := struct{ Quotes []provider.Quote `json:"quotes"` }{Quotes: all[:n]}
    b, _ := json.MarshalIndent(sample, "", "  ")
    fmt.Println(string(b))
}

func splitCSV(s string) []string {
    parts := strings.Split(s, ",")
    out := make([]string, 0, len(parts))
    for _, p := range parts {
        p = strings.TrimSpace(p)
        if p != "" { out = append(out, p) }
    }
    return out
}

func getenv(key, def string) string { if v := os.Getenv(key); v != "" { return v }; return def }
func getenvInt(key string, def int) int {
    if v := os.Getenv(key); v != "" {
        var x int
        _, _ = fmt.Sscanf(v, "%d", &x)
        if x != 0 { return x }
    }
    return def
}
func getenvBool(key string, def bool) bool {
    if v := os.Getenv(key); v != "" {
        switch strings.ToLower(v) {
        case "1","true","yes","y": return true
        case "0","false","no","n": return false
        }
    }
    return def
}
