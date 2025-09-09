package main

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "os"
    "os/signal"
    "strings"
    "syscall"
    "time"
    "compress/gzip"
    "io"
    "sync"

    "priceprovider/internal/config"
    "priceprovider/internal/httpx"
    "priceprovider/internal/provider"
    "priceprovider/internal/provider/ratelimit"
    "priceprovider/internal/provider/cache"
    "priceprovider/internal/provider/steamdt"
    pricempirepkg "priceprovider/internal/provider/pricempire"
    "priceprovider/internal/provider/pricempireadapter"
    "priceprovider/internal/provider/skinstablexyz"
)

type quotesResponse struct {
    Quotes []provider.Quote `json:"quotes"`
}

func main() {
    // Config
    cfgPath := os.Getenv("CONFIG_FILE")
    cfg, err := config.Load(cfgPath)
    if err != nil { log.Fatalf("config: %v", err) }
    port := cfg.Server.Port
    timeoutSec := cfg.Server.RequestTimeoutSec

    if cfg.SteamDT.Enabled && cfg.SteamDT.APIKey == "" {
        log.Println("warning: steamdt.enabled=true but STEAMDT_API_KEY not set")
    }

    httpClient := httpx.New(time.Duration(timeoutSec) * time.Second)
    httpClient.UserAgent = "price-provider/1.0"

    var providers []provider.Provider
    if cfg.SteamDT.Enabled {
        steam := steamdt.New(steamdt.Config{
            Name:        "SteamDT",
            URL:         cfg.SteamDT.Endpoint,
            Method:      http.MethodPost,
            Headers:     map[string]string{"Authorization": "Bearer " + cfg.SteamDT.APIKey},
            Currency:    cfg.SteamDT.Currency,
            SymbolMap:   map[string]string{},
            IncludeBids: cfg.SteamDT.IncludeBids,
            MaxItemsPerRequest: cfg.SteamDT.MaxItemsPerRequest,
            MaxConcurrency:     cfg.SteamDT.MaxConcurrency,
        }, httpClient)
        var p provider.Provider = steam
        // Prefer token bucket with burst if RPM is set, otherwise use min-interval
        if cfg.SteamDT.MaxRequestsPerMinute > 0 {
            rate := float64(cfg.SteamDT.MaxRequestsPerMinute) / 60.0
            burst := cfg.SteamDT.Burst
            if burst <= 0 { burst = 1 }
            p = &ratelimit.TokenBucketProvider{P: p, TB: ratelimit.NewTokenBucket(rate, burst)}
        } else if cfg.SteamDT.MinRequestIntervalSec > 0 {
            interval := time.Duration(cfg.SteamDT.MinRequestIntervalSec) * time.Second
            p = &ratelimit.MinInterval{P: p, Interval: interval}
        }
        // Wrap with per-symbol cache if configured
        if cfg.SteamDT.CacheTTLSeconds > 0 {
            p = &cache.Provider{P: p, TTL: time.Duration(cfg.SteamDT.CacheTTLSeconds) * time.Second, MaxItems: cfg.SteamDT.CacheMaxItems}
        }
        providers = append(providers, p)
    }
    if cfg.Pricempire.Enabled {
        if cfg.Pricempire.APIKey == "" {
            log.Println("warning: pricempire.enabled=true but PRICEMPIRE_API_KEY not set; skipping")
        } else {
            peClient, err := pricempirepkg.NewPricempireAPIClient(
                cfg.Pricempire.APIKey,
                pricempirepkg.WithHTTPClient(httpClient.HTTP),
                pricempirepkg.WithHeader(http.Header{
                    "User-Agent": []string{"price-provider/1.0"},
                }),
            )
            if err != nil {
                log.Printf("pricempire client error: %v", err)
            } else {
                pe := pricempireadapter.New(pricempireadapter.Config{
                    Name:     "Pricempire",
                    AppID:    cfg.Pricempire.AppID,
                    Currency: cfg.Pricempire.Currency,
                    Sources:  cfg.Pricempire.Sources,
                    ItemsCacheTTLSeconds: cfg.Pricempire.CacheTTLSeconds,
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
        }
    }
    if cfg.Skinstable.Enabled {
        if cfg.Skinstable.Endpoint == "" {
            log.Println("warning: skinstable.enabled=true but endpoint not set; skipping")
        } else {
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
    }

    mux := http.NewServeMux()
    mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
        _, _ = w.Write([]byte("ok"))
    })
    mux.HandleFunc("/api/quotes", func(w http.ResponseWriter, r *http.Request) {
        switch r.Method {
        case http.MethodGet:
            handleGetQuotes(w, r, providers)
        case http.MethodPost:
            handlePostQuotes(w, r, providers)
        default:
            http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
        }
    })

    srv := &http.Server{
        Addr:              ":" + port,
        Handler:           withJSONHeaders(withGzip(recoverPanic(limitBody(mux)))),
        ReadHeaderTimeout: 5 * time.Second,
        ReadTimeout:       15 * time.Second,
        WriteTimeout:      20 * time.Second,
        IdleTimeout:       60 * time.Second,
    }

    go func() {
        log.Printf("server listening on :%s", port)
        if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
            log.Fatalf("server: %v", err)
        }
    }()

    // graceful shutdown
    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer stop()
    <-ctx.Done()
    shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    _ = srv.Shutdown(shutdownCtx)
}

func handleGetQuotes(w http.ResponseWriter, r *http.Request, providers []provider.Provider) {
    q := r.URL.Query().Get("symbols")
    if strings.TrimSpace(q) == "" {
        http.Error(w, "missing symbols query param", http.StatusBadRequest)
        return
    }
    symbols := splitCSV(q)
    if len(symbols) > 1000 {
        http.Error(w, "too many symbols (max 1000)", http.StatusBadRequest)
        return
    }
    writeQuotes(w, r.Context(), providers, symbols)
}

type postBody struct {
    Symbols []string `json:"symbols"`
}

func handlePostQuotes(w http.ResponseWriter, r *http.Request, providers []provider.Provider) {
    var b postBody
    dec := json.NewDecoder(r.Body)
    dec.DisallowUnknownFields()
    if err := dec.Decode(&b); err != nil {
        http.Error(w, "invalid JSON body", http.StatusBadRequest)
        return
    }
    if len(b.Symbols) == 0 {
        http.Error(w, "symbols cannot be empty", http.StatusBadRequest)
        return
    }
    if len(b.Symbols) > 1000 {
        http.Error(w, "too many symbols (max 1000)", http.StatusBadRequest)
        return
    }
    writeQuotes(w, r.Context(), providers, b.Symbols)
}

func writeQuotes(w http.ResponseWriter, rctx context.Context, providers []provider.Provider, symbols []string) {
    ctx, cancel := context.WithTimeout(rctx, 15*time.Second)
    defer cancel()
    // fan-out to providers concurrently; collect partial results
    type result struct { quotes []provider.Quote; err error }
    ch := make(chan result, len(providers))
    for _, p := range providers {
        p := p
        go func() {
            qs, err := p.Fetch(ctx, symbols)
            ch <- result{qs, err}
        }()
    }
    var all []provider.Quote
    var errs []string
    for i := 0; i < len(providers); i++ {
        r := <-ch
        if r.err != nil { errs = append(errs, r.err.Error()); continue }
        all = append(all, r.quotes...)
    }
    if len(all) == 0 && len(errs) > 0 {
        http.Error(w, strings.Join(errs, "; "), http.StatusBadGateway)
        return
    }
    resp := quotesResponse{Quotes: all}
    w.WriteHeader(http.StatusOK)
    enc := json.NewEncoder(w)
    enc.SetEscapeHTML(false)
    enc.Encode(resp)
}

func withJSONHeaders(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json; charset=utf-8")
        // Basic CORS for browser usage; adjust as needed.
        w.Header().Set("Access-Control-Allow-Origin", "*")
        w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type,Authorization")
        if r.Method == http.MethodOptions {
            w.WriteHeader(http.StatusNoContent)
            return
        }
        next.ServeHTTP(w, r)
    })
}

// withGzip compresses response when client supports gzip.
func withGzip(next http.Handler) http.Handler {
    var gzPool = sync.Pool{New: func() any {
        // Prefer best speed to reduce CPU usage since payloads are JSON
        w, _ := gzip.NewWriterLevel(io.Discard, gzip.BestSpeed)
        return w
    }}
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
            next.ServeHTTP(w, r)
            return
        }
        gz := gzPool.Get().(*gzip.Writer)
        gz.Reset(w)
        defer func() {
            _ = gz.Close()
            gz.Reset(io.Discard)
            gzPool.Put(gz)
        }()
        w.Header().Set("Content-Encoding", "gzip")
        w.Header().Add("Vary", "Accept-Encoding")
        gw := gzipResponseWriter{ResponseWriter: w, Writer: gz}
        next.ServeHTTP(gw, r)
    })
}

type gzipResponseWriter struct {
    http.ResponseWriter
    Writer io.Writer
}

func (g gzipResponseWriter) Write(b []byte) (int, error) {
    return g.Writer.Write(b)
}

// limitBody caps request body size to avoid memory abuse.
func limitBody(next http.Handler) http.Handler {
    const maxBody = 1 << 20 // 1MB
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.Method == http.MethodPost && r.Body != nil {
            r.Body = http.MaxBytesReader(w, r.Body, maxBody)
        }
        next.ServeHTTP(w, r)
    })
}

// recoverPanic protects handlers from panics.
func recoverPanic(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        defer func() {
            if rec := recover(); rec != nil {
                http.Error(w, "internal server error", http.StatusInternalServerError)
            }
        }()
        next.ServeHTTP(w, r)
    })
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

// no extra helpers
