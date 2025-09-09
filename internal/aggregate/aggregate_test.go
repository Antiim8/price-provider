package aggregate

import (
    "testing"
    "time"

    "priceprovider/internal/provider"
)

func TestLatest_Prices_NewestWinsAcrossProviders_SameMarket(t *testing.T) {
    sym := "AK-47 | Redline (Field-Tested)"
    t1 := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
    t2 := t1.Add(1 * time.Hour)

    in := []provider.Quote{
        {Symbol: sym, Price: "10", Currency: "USD", Source: "SteamDT:BUFF:sell", ReceivedAt: t1},
        {Symbol: sym, Price: "11", Currency: "USD", Source: "Pricempire:buff", ReceivedAt: t2},
    }

    out := LatestByMarket(in, false)
    if len(out) != 1 {
        t.Fatalf("want 1, got %d: %+v", len(out), out)
    }
    got := out[0]
    if got.Market != "BUFF" || got.Side != "" || got.Price != "11" || !got.ReceivedAt.Equal(t2) {
        t.Fatalf("unexpected result: %+v", got)
    }
}

func TestLatest_SideSeparation_WhenEnabledOrDisabled(t *testing.T) {
    sym := "AK-47 | Redline (Field-Tested)"
    t1 := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
    t2 := t1.Add(1 * time.Minute)

    in := []provider.Quote{
        {Symbol: sym, Price: "10", Currency: "USD", Source: "SteamDT:BUFF:sell", ReceivedAt: t1},
        {Symbol: sym, Price: "9", Currency: "USD", Source: "SteamDT:BUFF:bid", ReceivedAt: t2},
    }

    // includeSides=true -> two rows, separate by side
    outTrue := LatestByMarket(in, true)
    if len(outTrue) != 2 {
        t.Fatalf("want 2 rows with includeSides=true, got %d: %+v", len(outTrue), outTrue)
    }
    if outTrue[0].Market != "BUFF" || (outTrue[0].Side != "bid" && outTrue[1].Side != "bid") {
        t.Fatalf("expected a bid row: %+v", outTrue)
    }
    if outTrue[0].Market != "BUFF" || (outTrue[0].Side != "sell" && outTrue[1].Side != "sell") {
        t.Fatalf("expected a sell row: %+v", outTrue)
    }

    // includeSides=false -> collapse to one (newest wins between sell/bid)
    outFalse := LatestByMarket(in, false)
    if len(outFalse) != 1 {
        t.Fatalf("want 1 row with includeSides=false, got %d: %+v", len(outFalse), outFalse)
    }
    if outFalse[0].Market != "BUFF" || outFalse[0].Side != "" || outFalse[0].Price != "9" || !outFalse[0].ReceivedAt.Equal(t2) {
        t.Fatalf("unexpected collapsed row: %+v", outFalse[0])
    }
}

func TestLatest_SkinstableAndPricempire_SameMarket_NewestWins(t *testing.T) {
    sym := "AK-47 | Redline (Field-Tested)"
    t1 := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
    t2 := t1.Add(2 * time.Minute)
    in := []provider.Quote{
        {Symbol: sym, Price: "1.95", Currency: "USD", Source: "SkinstableXYZ:CS.MONEY", ReceivedAt: t1},
        {Symbol: sym, Price: "2.05", Currency: "USD", Source: "Pricempire:CS.MONEY", ReceivedAt: t2},
    }
    out := LatestByMarket(in, true)
    if len(out) != 1 {
        t.Fatalf("want 1 row, got %d: %+v", len(out), out)
    }
    if out[0].Market != "CS.MONEY" || out[0].Side != "" || out[0].Price != "2.05" || !out[0].ReceivedAt.Equal(t2) {
        t.Fatalf("unexpected: %+v", out[0])
    }
}

func TestLatest_CaseInsensitiveNormalization_BuffAliases(t *testing.T) {
    sym := "AK-47 | Redline (Field-Tested)"
    t1 := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
    t2 := t1.Add(1 * time.Minute)
    in := []provider.Quote{
        {Symbol: sym, Price: "100", Currency: "USD", Source: "Pricempire:BUFF.163", ReceivedAt: t1},
        {Symbol: sym, Price: "101", Currency: "USD", Source: "Pricempire:buff", ReceivedAt: t2},
    }
    out := LatestByMarket(in, false)
    if len(out) != 1 {
        t.Fatalf("want 1 row, got %d: %+v", len(out), out)
    }
    if out[0].Market != "BUFF" || out[0].Price != "101" || !out[0].ReceivedAt.Equal(t2) {
        t.Fatalf("unexpected: %+v", out[0])
    }
}

func TestLatest_MixedCurrencies_DoNotCollapse(t *testing.T) {
    sym := "AK-47 | Redline (Field-Tested)"
    t1 := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
    t2 := t1.Add(1 * time.Minute)
    in := []provider.Quote{
        {Symbol: sym, Price: "100", Currency: "USD", Source: "Pricempire:BUFF", ReceivedAt: t1},
        {Symbol: sym, Price: "700", Currency: "CNY", Source: "Pricempire:BUFF", ReceivedAt: t2},
    }
    out := LatestByMarket(in, false)
    if len(out) != 2 {
        t.Fatalf("want 2 rows (distinct currencies), got %d: %+v", len(out), out)
    }
    // Ensure both currencies are present
    seen := map[string]bool{"USD": false, "CNY": false}
    for _, r := range out { seen[r.Currency] = true }
    if !seen["USD"] || !seen["CNY"] {
        t.Fatalf("currencies not both present: %+v", out)
    }
}

func TestNormalizeSource_MarketAliases_Casing(t *testing.T) {
    // Steam mapping
    m, s := NormalizeSource("SteamDT:STEAM:sell")
    if m != "Steam" || s != "sell" { t.Fatalf("steam mapping: %s %s", m, s) }

    // Skinport casing
    m, s = NormalizeSource("SteamDT:SKINPORT:sell")
    if m != "Skinport" || s != "sell" { t.Fatalf("skinport mapping: %s %s", m, s) }

    // C5 to C5GAME
    m, s = NormalizeSource("SteamDT:C5:bid")
    if m != "C5GAME" || s != "bid" { t.Fatalf("c5 mapping: %s %s", m, s) }

    // Pricempire no side, market normalized
    m, s = NormalizeSource("Pricempire:cs.money")
    if m != "CS.MONEY" || s != "" { t.Fatalf("csmoney mapping: %s %s", m, s) }

    // SkinstableXYZ site pass-through normalization
    m, s = NormalizeSource("SkinstableXYZ:BUFF.163")
    if m != "BUFF" || s != "" { t.Fatalf("buff163 mapping: %s %s", m, s) }
}
