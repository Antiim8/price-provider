package aggregate

import (
    "sort"
    "strings"
    "time"

    "priceprovider/internal/provider"
)

// MarketKey identifies a normalized market quote bucket.
type MarketKey struct {
    Symbol   string
    Market   string
    Side     string
    Currency string
}

// Latest is the latest quote per MarketKey.
type Latest struct {
    Symbol     string    `json:"symbol"`
    Market     string    `json:"market"`
    Side       string    `json:"side"`
    Currency   string    `json:"currency"`
    Price      string    `json:"price"`
    Provider   string    `json:"provider"`
    ReceivedAt time.Time `json:"received_at"`
}

// NormalizeSource extracts market and side from a quote Source.
// Rules:
// - Split on ':'
// - SteamDT: parts[1]=market, parts[2]=side (sell|bid) when len>=3
// - Pricempire: parts[1]=market when len>=2 (no side)
// - SkinstableXYZ: parts[1]=market when len>=2 (no side)
// - Normalize case and aliases for market; side lower-cased; trim spaces.
//   Aliases:
//     BUFF, buff, BUFF.163, BUFF163 -> BUFF
//     STEAM, Steam -> Steam
//     C5, C5GAME -> C5GAME
//     CSMONEY, CS.MONEY -> CS.MONEY
//     SKINPORT, Skinport -> Skinport
// aliasMap normalizes various market/site spellings and aliases.
var aliasMap = map[string]string{
    "buff":     "BUFF",
    "buff.163": "BUFF",
    "buff163":  "BUFF",
    "steam":    "Steam",
    "c5":       "C5GAME",
    "c5game":   "C5GAME",
    "csmoney":  "CS.MONEY",
    "cs.money": "CS.MONEY",
    "skinport": "Skinport",
}

func NormalizeSource(src string) (market string, side string) {
    s := strings.TrimSpace(src)
    if s == "" { return "", "" }
    parts := strings.Split(s, ":")
    if len(parts) == 0 { return "", "" }
    pref := strings.TrimSpace(parts[0])
    lpref := strings.ToLower(pref)

    var mraw, sraw string
    switch lpref {
    case "steamdt":
        if len(parts) >= 2 { mraw = parts[1] }
        if len(parts) >= 3 { sraw = parts[2] }
    case "pricempire", "skinstablexyz":
        if len(parts) >= 2 { mraw = parts[1] }
    default:
        if len(parts) >= 2 { mraw = parts[1] }
        if len(parts) >= 3 { sraw = parts[2] }
    }

    m := strings.TrimSpace(mraw)
    if norm, ok := aliasMap[strings.ToLower(m)]; ok {
        market = norm
    } else {
        market = m
    }

    side = strings.ToLower(strings.TrimSpace(sraw))
    if side != "sell" && side != "bid" {
        if side == "" {
            // keep empty
        } else {
            // Unknown side; keep lower-cased string
        }
    }
    return market, side
}

// LatestByMarket collapses quotes by (Symbol, Market, Side?, Currency) keeping the newest.
// If includeSides is false, side is forced to "" for grouping.
// For equal timestamps, later input wins. Zero timestamps are replaced with time.Now().UTC().
func LatestByMarket(quotes []provider.Quote, includeSides bool) []Latest {
    now := time.Now().UTC()
    latest := make(map[MarketKey]Latest, len(quotes))

    for _, q := range quotes {
        market, side := NormalizeSource(q.Source)
        if !includeSides { side = "" }
        ts := q.ReceivedAt
        if ts.IsZero() { ts = now }

        // Provider is the prefix before ':' from Source
        providerName := ""
        if idx := strings.Index(q.Source, ":"); idx > 0 {
            providerName = q.Source[:idx]
        } else {
            providerName = q.Source
        }

        key := MarketKey{Symbol: q.Symbol, Market: market, Side: side, Currency: q.Currency}
        if cur, ok := latest[key]; ok {
            if ts.After(cur.ReceivedAt) || ts.Equal(cur.ReceivedAt) {
                latest[key] = Latest{
                    Symbol:     q.Symbol,
                    Market:     market,
                    Side:       side,
                    Currency:   q.Currency,
                    Price:      q.Price,
                    Provider:   providerName,
                    ReceivedAt: ts,
                }
            }
        } else {
            latest[key] = Latest{
                Symbol:     q.Symbol,
                Market:     market,
                Side:       side,
                Currency:   q.Currency,
                Price:      q.Price,
                Provider:   providerName,
                ReceivedAt: ts,
            }
        }
    }

    out := make([]Latest, 0, len(latest))
    for _, v := range latest { out = append(out, v) }
    sort.Slice(out, func(i, j int) bool {
        if out[i].Symbol != out[j].Symbol { return out[i].Symbol < out[j].Symbol }
        if out[i].Market != out[j].Market { return out[i].Market < out[j].Market }
        if out[i].Side != out[j].Side { return out[i].Side < out[j].Side }
        return false
    })
    return out
}
