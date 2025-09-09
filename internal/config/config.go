package config

import (
    "encoding/json"
    "errors"
    "fmt"
    "os"
    "strings"
)

type Server struct {
    Port               string `json:"port"`
    RequestTimeoutSec  int    `json:"request_timeout_sec"`
}

type SteamDT struct {
    Enabled               bool   `json:"enabled"`
    APIKey                string `json:"api_key"`
    Endpoint              string `json:"endpoint"`
    IncludeBids           bool   `json:"include_bids"`
    Currency              string `json:"currency"`
    MaxRequestsPerMinute  int    `json:"max_requests_per_minute"`
    MinRequestIntervalSec int    `json:"min_request_interval_sec"`
    Burst                 int    `json:"burst"`
    MaxItemsPerRequest    int    `json:"max_items_per_request"`
    MaxConcurrency        int    `json:"max_concurrency"`
    CacheTTLSeconds       int    `json:"cache_ttl_sec"`
    CacheMaxItems         int    `json:"cache_max_items"`
}

type Pricempire struct {
    Enabled               bool     `json:"enabled"`
    APIKey                string   `json:"api_key"`
    AppID                 int      `json:"app_id"`
    Currency              string   `json:"currency"`
    Sources               []string `json:"sources"`
    MaxRequestsPerMinute  int      `json:"max_requests_per_minute"`
    MinRequestIntervalSec int      `json:"min_request_interval_sec"`
    Burst                 int      `json:"burst"`
    CacheTTLSeconds       int      `json:"cache_ttl_sec"`
    CacheMaxItems         int      `json:"cache_max_items"`
}

type Skinstable struct {
    Enabled               bool   `json:"enabled"`
    Endpoint              string `json:"endpoint"`
    APIKey                string `json:"api_key"`
    Currency              string `json:"currency"`
    ItemsCacheTTLSeconds  int    `json:"items_cache_ttl_sec"`
    AppID                 int    `json:"app_id"`
    Sites                 []string `json:"sites"`
    MaxRequestsPerMinute  int    `json:"max_requests_per_minute"`
    MinRequestIntervalSec int    `json:"min_request_interval_sec"`
    Burst                 int    `json:"burst"`
    CacheTTLSeconds       int    `json:"cache_ttl_sec"`
    CacheMaxItems         int    `json:"cache_max_items"`
}

type Config struct {
    Server     Server     `json:"server"`
    SteamDT    SteamDT    `json:"steamdt"`
    Pricempire Pricempire `json:"pricempire"`
    Skinstable Skinstable `json:"skinstable"`
}

func Default() Config {
    return Config{
        Server: Server{Port: "8080", RequestTimeoutSec: 10},
        SteamDT: SteamDT{
            Enabled:     true,
            Endpoint:    "https://open.steamdt.com/open/cs2/v1/price/batch",
            IncludeBids: true,
            Currency:    "CNY",
            MaxRequestsPerMinute: 1,
            Burst: 1,
            MaxItemsPerRequest:  200,
            MaxConcurrency:      2,
            CacheTTLSeconds:     3,
            CacheMaxItems:       10000,
        },
        Pricempire: Pricempire{
            Enabled:  false,
            AppID:    730,
            Currency: "USD",
            Sources:  []string{"buff"},
            MaxRequestsPerMinute: 2,
            Burst: 2,
            CacheTTLSeconds: 15,
            CacheMaxItems:   50000,
        },
        Skinstable: Skinstable{
            Enabled:  false,
            Endpoint: "",
            Currency: "USD",
            AppID:    730,
            Sites:    []string{"CS.MONEY"},
            ItemsCacheTTLSeconds: 15,
            MaxRequestsPerMinute: 2,
            Burst: 2,
            CacheTTLSeconds: 15,
            CacheMaxItems:   50000,
        },
    }
}

// Load reads JSON config from path. If path is empty or file does not exist,
// it returns defaults. Environment variables override select fields for secrecy.
func Load(path string) (Config, error) {
    cfg := Default()
    if path == "" {
        if _, err := os.Stat("config.json"); err == nil {
            path = "config.json"
        }
    }
    if path != "" {
        b, err := os.ReadFile(path)
        if err != nil && !errors.Is(err, os.ErrNotExist) {
            return cfg, fmt.Errorf("read config: %w", err)
        }
        if err == nil {
            if err := json.Unmarshal(b, &cfg); err != nil {
                return cfg, fmt.Errorf("parse config: %w", err)
            }
        }
    }
    applyEnv(&cfg)
    return cfg, nil
}

func applyEnv(cfg *Config) {
    if v := os.Getenv("PORT"); v != "" { cfg.Server.Port = v }
    if v := os.Getenv("REQUEST_TIMEOUT_SEC"); v != "" {
        var x int; fmt.Sscanf(v, "%d", &x); if x > 0 { cfg.Server.RequestTimeoutSec = x }
    }
    if v := os.Getenv("STEAMDT_API_KEY"); v != "" { cfg.SteamDT.APIKey = v }
    if v := os.Getenv("STEAMDT_ENDPOINT"); v != "" { cfg.SteamDT.Endpoint = v }
    if v := os.Getenv("INCLUDE_BIDS"); v != "" {
        switch strings.ToLower(v) {
        case "1","true","yes","y": cfg.SteamDT.IncludeBids = true
        case "0","false","no","n": cfg.SteamDT.IncludeBids = false
        }
    }
    if v := os.Getenv("CURRENCY"); v != "" { cfg.SteamDT.Currency = v }
    if v := os.Getenv("STEAMDT_MIN_INTERVAL_SEC"); v != "" {
        var x int; fmt.Sscanf(v, "%d", &x); if x >= 0 { cfg.SteamDT.MinRequestIntervalSec = x }
    }
    if v := os.Getenv("STEAMDT_MAX_RPM"); v != "" {
        var x int; fmt.Sscanf(v, "%d", &x); if x >= 0 { cfg.SteamDT.MaxRequestsPerMinute = x }
    }
    if v := os.Getenv("STEAMDT_BURST"); v != "" {
        var x int; fmt.Sscanf(v, "%d", &x); if x > 0 { cfg.SteamDT.Burst = x }
    }
    if v := os.Getenv("STEAMDT_MAX_ITEMS_PER_REQUEST"); v != "" {
        var x int; fmt.Sscanf(v, "%d", &x); if x > 0 { cfg.SteamDT.MaxItemsPerRequest = x }
    }
    if v := os.Getenv("STEAMDT_MAX_CONCURRENCY"); v != "" {
        var x int; fmt.Sscanf(v, "%d", &x); if x > 0 { cfg.SteamDT.MaxConcurrency = x }
    }
    if v := os.Getenv("STEAMDT_CACHE_TTL_SEC"); v != "" {
        var x int; fmt.Sscanf(v, "%d", &x); if x >= 0 { cfg.SteamDT.CacheTTLSeconds = x }
    }
    if v := os.Getenv("STEAMDT_CACHE_MAX_ITEMS"); v != "" {
        var x int; fmt.Sscanf(v, "%d", &x); if x > 0 { cfg.SteamDT.CacheMaxItems = x }
    }
    if v := os.Getenv("PRICEMPIRE_API_KEY"); v != "" { cfg.Pricempire.APIKey = v }
    if v := os.Getenv("PRICEMPIRE_APP_ID"); v != "" {
        var x int; fmt.Sscanf(v, "%d", &x); if x > 0 { cfg.Pricempire.AppID = x }
    }
    if v := os.Getenv("PRICEMPIRE_CURRENCY"); v != "" { cfg.Pricempire.Currency = v }
    if v := os.Getenv("PRICEMPIRE_SOURCES"); v != "" { cfg.Pricempire.Sources = splitCSV(v) }
    if v := os.Getenv("PRICEMPIRE_MIN_INTERVAL_SEC"); v != "" {
        var x int; fmt.Sscanf(v, "%d", &x); if x >= 0 { cfg.Pricempire.MinRequestIntervalSec = x }
    }
    if v := os.Getenv("PRICEMPIRE_MAX_RPM"); v != "" {
        var x int; fmt.Sscanf(v, "%d", &x); if x >= 0 { cfg.Pricempire.MaxRequestsPerMinute = x }
    }
    if v := os.Getenv("PRICEMPIRE_BURST"); v != "" {
        var x int; fmt.Sscanf(v, "%d", &x); if x > 0 { cfg.Pricempire.Burst = x }
    }
    if v := os.Getenv("PRICEMPIRE_CACHE_TTL_SEC"); v != "" {
        var x int; fmt.Sscanf(v, "%d", &x); if x >= 0 { cfg.Pricempire.CacheTTLSeconds = x }
    }
    if v := os.Getenv("PRICEMPIRE_CACHE_MAX_ITEMS"); v != "" {
        var x int; fmt.Sscanf(v, "%d", &x); if x > 0 { cfg.Pricempire.CacheMaxItems = x }
    }

    // Skinstable env
    if v := os.Getenv("SKINSTABLE_ENABLED"); v != "" {
        switch strings.ToLower(v) {
        case "1","true","yes","y": cfg.Skinstable.Enabled = true
        case "0","false","no","n": cfg.Skinstable.Enabled = false
        }
    }
    if v := os.Getenv("SKINSTABLE_ENDPOINT"); v != "" { cfg.Skinstable.Endpoint = v }
    if v := os.Getenv("SKINSTABLE_API_KEY"); v != "" { cfg.Skinstable.APIKey = v }
    if v := os.Getenv("SKINSTABLE_CURRENCY"); v != "" { cfg.Skinstable.Currency = v }
    if v := os.Getenv("SKINSTABLE_ITEMS_CACHE_TTL_SEC"); v != "" {
        var x int; fmt.Sscanf(v, "%d", &x); if x >= 0 { cfg.Skinstable.ItemsCacheTTLSeconds = x }
    }
    if v := os.Getenv("SKINSTABLE_APP_ID"); v != "" {
        var x int; fmt.Sscanf(v, "%d", &x); if x > 0 { cfg.Skinstable.AppID = x }
    }
    if v := os.Getenv("SKINSTABLE_SITES"); v != "" { cfg.Skinstable.Sites = splitCSV(v) }
    if v := os.Getenv("SKINSTABLE_MIN_INTERVAL_SEC"); v != "" {
        var x int; fmt.Sscanf(v, "%d", &x); if x >= 0 { cfg.Skinstable.MinRequestIntervalSec = x }
    }
    if v := os.Getenv("SKINSTABLE_MAX_RPM"); v != "" {
        var x int; fmt.Sscanf(v, "%d", &x); if x >= 0 { cfg.Skinstable.MaxRequestsPerMinute = x }
    }
    if v := os.Getenv("SKINSTABLE_BURST"); v != "" {
        var x int; fmt.Sscanf(v, "%d", &x); if x > 0 { cfg.Skinstable.Burst = x }
    }
    if v := os.Getenv("SKINSTABLE_CACHE_TTL_SEC"); v != "" {
        var x int; fmt.Sscanf(v, "%d", &x); if x >= 0 { cfg.Skinstable.CacheTTLSeconds = x }
    }
    if v := os.Getenv("SKINSTABLE_CACHE_MAX_ITEMS"); v != "" {
        var x int; fmt.Sscanf(v, "%d", &x); if x > 0 { cfg.Skinstable.CacheMaxItems = x }
    }
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
