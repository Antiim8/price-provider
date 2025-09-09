package main

import (
    "context"
    "encoding/json"
    "net/http/httptest"
    "testing"
    "time"

    "priceprovider/internal/provider"
)

type fakeProvider struct { name string; quotes []provider.Quote }
func (f fakeProvider) Name() string { return f.name }
func (f fakeProvider) Fetch(_ context.Context, symbols []string) ([]provider.Quote, error) {
    // naive filter by symbol if provided
    if len(symbols) == 0 { return f.quotes, nil }
    var out []provider.Quote
    want := make(map[string]struct{}, len(symbols))
    for _, s := range symbols { want[s] = struct{}{} }
    for _, q := range f.quotes {
        if _, ok := want[q.Symbol]; ok { out = append(out, q) }
    }
    return out, nil
}

func TestLatest_NewestAcrossProviders_SameMarket(t *testing.T) {
    sym := "AK-47 | Redline (Field-Tested)"
    t1 := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
    t2 := t1.Add(time.Hour)
    p1 := fakeProvider{"steamdt", []provider.Quote{{Symbol: sym, Price: "10", Currency: "USD", Source: "SteamDT:BUFF:sell", ReceivedAt: t1}}}
    p2 := fakeProvider{"pricempire", []provider.Quote{{Symbol: sym, Price: "11", Currency: "USD", Source: "Pricempire:buff", ReceivedAt: t2}}}

    rr := httptest.NewRecorder()
    writeLatest(rr, t.Context(), []provider.Provider{p1, p2}, []string{sym}, "all", "")
    if rr.Code != 200 { t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String()) }
    var resp latestResponse
    if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil { t.Fatalf("decode: %v", err) }
    if len(resp.Latest) != 1 { t.Fatalf("want 1 row, got %d: %+v", len(resp.Latest), resp.Latest) }
    got := resp.Latest[0]
    if got.Market != "BUFF" || got.Side != "" || got.Currency != "USD" || got.Price != "11" || !got.ReceivedAt.Equal(t2) {
        t.Fatalf("unexpected: %+v", got)
    }
}

func TestLatest_SideFiltering_SellAndBid(t *testing.T) {
    sym := "AK-47 | Redline (Field-Tested)"
    t1 := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
    t2 := t1.Add(time.Minute)
    p := fakeProvider{"steamdt", []provider.Quote{
        {Symbol: sym, Price: "10", Currency: "USD", Source: "SteamDT:BUFF:sell", ReceivedAt: t1},
        {Symbol: sym, Price: "9", Currency: "USD", Source: "SteamDT:BUFF:bid", ReceivedAt: t2},
    }}

    // sell only
    rrSell := httptest.NewRecorder()
    writeLatest(rrSell, t.Context(), []provider.Provider{p}, []string{sym}, "sell", "")
    var respSell latestResponse
    if err := json.Unmarshal(rrSell.Body.Bytes(), &respSell); err != nil { t.Fatalf("decode sell: %v", err) }
    if len(respSell.Latest) != 1 || respSell.Latest[0].Side != "sell" || respSell.Latest[0].Price != "10" {
        t.Fatalf("unexpected sell: %+v", respSell.Latest)
    }

    // bid only
    rrBid := httptest.NewRecorder()
    writeLatest(rrBid, t.Context(), []provider.Provider{p}, []string{sym}, "bid", "")
    var respBid latestResponse
    if err := json.Unmarshal(rrBid.Body.Bytes(), &respBid); err != nil { t.Fatalf("decode bid: %v", err) }
    if len(respBid.Latest) != 1 || respBid.Latest[0].Side != "bid" || respBid.Latest[0].Price != "9" {
        t.Fatalf("unexpected bid: %+v", respBid.Latest)
    }
}

func TestLatest_MixedCurrencies_DoNotCollapse_Server(t *testing.T) {
    sym := "AK-47 | Redline (Field-Tested)"
    t1 := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
    t2 := t1.Add(time.Minute)
    p := fakeProvider{"pricempire", []provider.Quote{
        {Symbol: sym, Price: "100", Currency: "USD", Source: "Pricempire:BUFF", ReceivedAt: t1},
        {Symbol: sym, Price: "700", Currency: "CNY", Source: "Pricempire:buff.163", ReceivedAt: t2},
    }}
    rr := httptest.NewRecorder()
    writeLatest(rr, t.Context(), []provider.Provider{p}, []string{sym}, "all", "")
    var resp latestResponse
    if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil { t.Fatalf("decode: %v", err) }
    if len(resp.Latest) != 2 { t.Fatalf("want 2 rows, got %d: %+v", len(resp.Latest), resp.Latest) }
    seen := map[string]bool{"USD": false, "CNY": false}
    for _, r := range resp.Latest { seen[r.Currency] = true }
    if !seen["USD"] || !seen["CNY"] { t.Fatalf("currencies missing: %+v", resp.Latest) }
}
