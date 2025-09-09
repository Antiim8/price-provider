package ratelimit

import (
    "context"
    "sync"
    "time"

    "priceprovider/internal/provider"
)

// TokenBucket provides a stdlib-only token bucket limiter.
// - rate: tokens per second
// - capacity: maximum tokens the bucket can hold (burst)
type TokenBucket struct {
    rate     float64
    capacity float64

    mu     sync.Mutex
    tokens float64
    last   time.Time
}

func NewTokenBucket(tokensPerSecond float64, burst int) *TokenBucket {
    if tokensPerSecond <= 0 { tokensPerSecond = 0.0000001 }
    if burst <= 0 { burst = 1 }
    return &TokenBucket{
        rate:     tokensPerSecond,
        capacity: float64(burst),
        tokens:   float64(burst), // start full to allow an initial burst
        last:     time.Now(),
    }
}

// wait blocks until one token is available or context is canceled.
func (tb *TokenBucket) wait(ctx context.Context) error {
    for {
        tb.mu.Lock()
        now := time.Now()
        // Refill
        elapsed := now.Sub(tb.last).Seconds()
        if elapsed > 0 {
            tb.tokens += elapsed * tb.rate
            if tb.tokens > tb.capacity {
                tb.tokens = tb.capacity
            }
            tb.last = now
        }
        if tb.tokens >= 1 {
            tb.tokens -= 1
            tb.mu.Unlock()
            return nil
        }
        // Need to wait for the remaining fraction
        deficit := 1 - tb.tokens
        tb.mu.Unlock()
        // time needed to accumulate one token
        waitDur := time.Duration(deficit/tb.rate*1e9) * time.Nanosecond
        if waitDur <= 0 { waitDur = time.Millisecond }
        timer := time.NewTimer(waitDur)
        select {
        case <-ctx.Done():
            timer.Stop()
            return ctx.Err()
        case <-timer.C:
        }
    }
}

// TokenBucketProvider wraps a Provider and gates calls using a token bucket.
type TokenBucketProvider struct {
    P  provider.Provider
    TB *TokenBucket
}

func (t *TokenBucketProvider) Name() string { return t.P.Name() }

func (t *TokenBucketProvider) Fetch(ctx context.Context, symbols []string) ([]provider.Quote, error) {
    if t.TB != nil {
        if err := t.TB.wait(ctx); err != nil { return nil, err }
    }
    return t.P.Fetch(ctx, symbols)
}

