package provider

import (
    "context"
    "time"
)

// Quote is the normalized shape returned by all providers.
// Keep price as a string to avoid float rounding and external deps.
type Quote struct {
    Symbol     string    `json:"symbol"`
    Price      string    `json:"price"`
    Currency   string    `json:"currency"`
    Source     string    `json:"source"`
    ReceivedAt time.Time `json:"received_at"`
}

type Provider interface {
    Name() string
    Fetch(ctx context.Context, symbols []string) ([]Quote, error)
}

