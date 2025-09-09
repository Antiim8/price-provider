package ratelimit

import (
    "context"
    "sync"
    "time"

    "priceprovider/internal/provider"
)

// MinInterval wraps a provider and enforces a minimum time between calls.
// Concurrent calls will wait until the interval has elapsed since the last call,
// or return early if the context is canceled.
type MinInterval struct {
    P           provider.Provider
    Interval    time.Duration
    mu          sync.Mutex
    last        time.Time
}

func (m *MinInterval) Name() string { return m.P.Name() }

func (m *MinInterval) Fetch(ctx context.Context, symbols []string) ([]provider.Quote, error) {
    if m.Interval > 0 {
        // simple gate: ensure at least Interval since last
        m.mu.Lock()
        wait := time.Until(m.last.Add(m.Interval))
        m.mu.Unlock()
        if wait > 0 {
            t := time.NewTimer(wait)
            defer t.Stop()
            select {
            case <-ctx.Done():
                return nil, ctx.Err()
            case <-t.C:
            }
        }
    }
    qs, err := m.P.Fetch(ctx, symbols)
    if m.Interval > 0 {
        m.mu.Lock()
        m.last = time.Now()
        m.mu.Unlock()
    }
    return qs, err
}

