package cache

import (
    "context"
    "sync"
    "time"

    "priceprovider/internal/provider"
)

// entry stores cached quotes for a single symbol with expiry.
type entry struct {
    expiresAt time.Time
    quotes    []provider.Quote
}

// Provider caches results per symbol for a TTL.
// It requests only missing symbols from the underlying provider and
// combines cached + fresh results.
type Provider struct {
    P        provider.Provider
    TTL      time.Duration
    MaxItems int

    mu    sync.RWMutex
    items map[string]entry // key: symbol
}

func (c *Provider) Name() string { return c.P.Name() }

// Fetch returns quotes for requested symbols using cache when valid.
func (c *Provider) Fetch(ctx context.Context, symbols []string) ([]provider.Quote, error) {
    if c.P == nil || c.TTL <= 0 {
        return c.P.Fetch(ctx, symbols)
    }

    now := time.Now()

    // Split into cached and missing symbols
    cached := make([]provider.Quote, 0, len(symbols))
    missingSet := make(map[string]struct{}, len(symbols))

    c.mu.RLock()
    for _, s := range symbols {
        if e, ok := c.items[s]; ok && now.Before(e.expiresAt) {
            cached = append(cached, e.quotes...)
            continue
        }
        missingSet[s] = struct{}{}
    }
    c.mu.RUnlock()

    // If everything is cached, return quickly
    if len(missingSet) == 0 {
        return cached, nil
    }

    // Build list of unique missing symbols preserving request order
    missing := make([]string, 0, len(missingSet))
    seen := make(map[string]struct{}, len(missingSet))
    for _, s := range symbols {
        if _, ok := missingSet[s]; ok {
            if _, dup := seen[s]; !dup { seen[s] = struct{}{}; missing = append(missing, s) }
        }
    }

    fresh, err := c.P.Fetch(ctx, missing)
    if err != nil {
        // If we have at least some cached data, return it rather than failing entirely
        if len(cached) > 0 {
            return cached, nil
        }
        return nil, err
    }

    // Index fresh quotes by symbol for storage and output ordering
    bySymbol := make(map[string][]provider.Quote, len(missing))
    for _, q := range fresh {
        bySymbol[q.Symbol] = append(bySymbol[q.Symbol], q)
    }

    // Update cache
    if c.items == nil {
        c.mu.Lock()
        if c.items == nil { c.items = make(map[string]entry, len(bySymbol)) }
        c.mu.Unlock()
    }

    expiry := now.Add(c.TTL)
    c.mu.Lock()
    for sym, qs := range bySymbol {
        c.items[sym] = entry{expiresAt: expiry, quotes: qs}
    }
    // best-effort cap cache size
    if c.MaxItems > 0 && len(c.items) > c.MaxItems {
        // simple random/oldest eviction: remove expired first, then arbitrary
        for k, v := range c.items {
            if time.Now().After(v.expiresAt) {
                delete(c.items, k)
            }
            if len(c.items) <= c.MaxItems {
                break
            }
        }
        // If still too big, delete arbitrary keys until under limit
        for k := range c.items {
            if len(c.items) <= c.MaxItems { break }
            delete(c.items, k)
        }
    }
    c.mu.Unlock()

    // Merge cached and fresh preserving request order
    out := make([]provider.Quote, 0, len(cached)+len(fresh))
    for _, s := range symbols {
        if qs, ok := bySymbol[s]; ok {
            out = append(out, qs...)
            continue
        }
        // pull from cached per-symbol
        c.mu.RLock()
        if e, ok := c.items[s]; ok && now.Before(e.expiresAt) {
            out = append(out, e.quotes...)
        }
        c.mu.RUnlock()
    }
    return out, nil
}

