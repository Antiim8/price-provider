package pricempireadapter

import (
    "context"
    "fmt"
    "strconv"
    "strings"
    "time"
    "sync"

    "priceprovider/internal/provider"
    "priceprovider/internal/provider/pricempire"
)

type Config struct {
    Name     string   // display name, default: Pricempire
    AppID    int      // Steam app id (e.g., 730)
    Currency string   // e.g., USD
    Sources  []string // e.g., ["buff","steam","skinport"]
    // ItemsCacheTTLSeconds caches the full Pricempire items payload
    // to avoid re-fetching the entire dataset for successive calls.
    // If <= 0, no internal caching is used.
    ItemsCacheTTLSeconds int
}

type Adapter struct {
    cfg    Config
    client *pricempire.PricempireAPIClient

    // cache of last fetched items keyed by item name for fast lookup
    mu           sync.RWMutex
    itemsByName  map[string]pricempire.Item
    itemsExpires time.Time
}

func New(cfg Config, client *pricempire.PricempireAPIClient) *Adapter {
    if cfg.Name == "" { cfg.Name = "Pricempire" }
    if cfg.AppID == 0 { cfg.AppID = 730 }
    if cfg.Currency == "" { cfg.Currency = "USD" }
    if len(cfg.Sources) == 0 { cfg.Sources = []string{"buff"} }
    return &Adapter{cfg: cfg, client: client}
}

func (a *Adapter) Name() string { return a.cfg.Name }

func (a *Adapter) Fetch(ctx context.Context, symbols []string) ([]provider.Quote, error) {
    // Try internal cache first if enabled
    var itemsByName map[string]pricempire.Item
    ttl := time.Duration(a.cfg.ItemsCacheTTLSeconds) * time.Second
    if ttl > 0 {
        a.mu.RLock()
        if !a.itemsExpires.IsZero() && time.Now().Before(a.itemsExpires) && len(a.itemsByName) > 0 {
            itemsByName = a.itemsByName
        }
        a.mu.RUnlock()
    }

    // Cache miss -> fetch and populate cache map
    if itemsByName == nil {
        items, err := a.client.GetAllItemsV3(ctx, a.cfg.AppID, a.cfg.Currency, a.cfg.Sources)
        if err != nil {
            return nil, err
        }
        m := make(map[string]pricempire.Item, len(items))
        for _, it := range items { m[it.Name] = it }
        itemsByName = m
        if ttl > 0 {
            a.mu.Lock()
            a.itemsByName = m
            a.itemsExpires = time.Now().Add(ttl)
            a.mu.Unlock()
        }
    }

    // Build a set of requested symbols for quick filtering (case-sensitive match)
    want := make(map[string]struct{}, len(symbols))
    for _, s := range symbols { want[s] = struct{}{} }

    now := time.Now().UTC()
    // Rough capacity hint: if filtering, assume 1-2 sources per symbol
    capHint := len(itemsByName)
    if len(want) > 0 { capHint = len(want) * 2 }
    out := make([]provider.Quote, 0, capHint)

    emit := func(name string, it pricempire.Item) {
        for src, p := range it.Prices {
            if p.Price == nil { continue }
            price := formatFloat(*p.Price)
            if price == "" { continue }
            ts := now
            if p.CreatedAt != nil { ts = p.CreatedAt.UTC() }
            out = append(out, provider.Quote{
                Symbol:     name,
                Price:      price,
                Currency:   a.cfg.Currency,
                Source:     fmt.Sprintf("%s:%s", a.cfg.Name, src),
                ReceivedAt: ts,
            })
        }
    }

    if len(want) > 0 {
        for name := range want {
            if it, ok := itemsByName[name]; ok { emit(name, it) }
        }
    } else {
        for name, it := range itemsByName { emit(name, it) }
    }
    return out, nil
}

func formatFloat(v float64) string {
    // Preserve precision without trailing zeros
    s := strconv.FormatFloat(v, 'f', -1, 64)
    // Normalize "+Inf", "-Inf", "NaN" if they ever appear (shouldn't) by skipping
    switch strings.ToLower(s) {
    case "inf", "+inf", "-inf", "nan":
        return ""
    }
    return s
}
