package main

import (
    "bufio"
    "bytes"
    "context"
    "encoding/json"
    "flag"
    "fmt"
    "io"
    "log"
    "net/http"
    "os"
    "sort"
    "sync"
    "time"

    "priceprovider/internal/config"
)

type apiResp struct {
    Success      bool              `json:"success"`
    Data         []json.RawMessage `json:"data"`
    ErrorCode    int               `json:"errorCode"`
    ErrorMsg     string            `json:"errorMsg"`
    ErrorData    json.RawMessage   `json:"errorData"`
    ErrorCodeStr string            `json:"errorCodeStr"`
}

type httpStatusErr struct {
    code int
    body string
}

func (e *httpStatusErr) Error() string { return fmt.Sprintf("http %d: %s", e.code, e.body) }

func main() {
    var (
        symbolsFile string
        outPath     string
        cfgPath     string
        batchSize   int
        concurrency int
        timeoutSec  int
        maxRetries  int
        rpm         int
    )
    flag.StringVar(&symbolsFile, "symbols-file", "pricempire_all_prices.json", "JSON file with keys as marketHashNames")
    flag.StringVar(&outPath, "out", "steamdt_all_prices.json", "output JSON file path")
    flag.StringVar(&cfgPath, "config", "", "path to config.json (optional)")
    flag.IntVar(&batchSize, "batch", 25, "batch size for SteamDT requests")
    flag.IntVar(&concurrency, "concurrency", 4, "number of parallel requests")
    flag.IntVar(&timeoutSec, "timeout", 20, "HTTP timeout seconds")
    flag.IntVar(&maxRetries, "retries", 3, "max retries on 429/5xx")
    flag.IntVar(&rpm, "rpm", 0, "max requests per minute (0 = unlimited)")
    flag.Parse()

    // Load config/env
    cfg, err := config.Load(cfgPath)
    if err != nil {
        log.Fatalf("config: %v", err)
    }
    if cfg.SteamDT.APIKey == "" {
        log.Fatal("STEAMDT_API_KEY missing (set in config.json or env)")
    }
    endpoint := cfg.SteamDT.Endpoint
    if endpoint == "" {
        endpoint = "https://open.steamdt.com/open/cs2/v1/price/batch"
    }

    // Load symbols as keys from the provided file
    names, err := readKeys(symbolsFile)
    if err != nil {
        log.Fatalf("read symbols: %v", err)
    }
    if len(names) == 0 {
        log.Fatal("no symbols found in symbols-file")
    }
    log.Printf("symbols: %d", len(names))

    // Prepare HTTP client
    hc := &http.Client{Timeout: time.Duration(timeoutSec) * time.Second}

    // Prepare output writer (streaming)
    outFile, err := os.Create(outPath)
    if err != nil {
        log.Fatalf("create out: %v", err)
    }
    defer outFile.Close()
    bw := bufio.NewWriterSize(outFile, 1<<20)
    defer bw.Flush()

    // Start JSON envelope
    _, _ = bw.WriteString("{\"success\":true,\"data\":[")
    first := true
    var writeMu sync.Mutex

    // Request rate limiter by RPM, if provided
    var tokenCh <-chan time.Time
    if rpm > 0 {
        interval := time.Minute / time.Duration(rpm)
        t := time.NewTicker(interval)
        defer t.Stop()
        tokenCh = t.C
    }

    // Worker pool
    type job struct{ idx int; batch []string }
    jobs := make(chan job, concurrency*2)
    wg := sync.WaitGroup{}

    doReq := func(ctx context.Context, names []string) ([]json.RawMessage, error) {
        // prepare body
        bodyObj := map[string]any{"marketHashNames": names}
        body, _ := json.Marshal(bodyObj)
        req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
        if err != nil { return nil, err }
        req.Header.Set("Content-Type", "application/json")
        req.Header.Set("Accept", "application/json")
        req.Header.Set("Authorization", "Bearer "+cfg.SteamDT.APIKey)
        if tokenCh != nil {
            <-tokenCh // gate by RPM
        }
        resp, err := hc.Do(req)
        if err != nil { return nil, err }
        defer resp.Body.Close()
        if resp.StatusCode < 200 || resp.StatusCode >= 300 {
            b, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<10))
            return nil, &httpStatusErr{code: resp.StatusCode, body: string(b)}
        }
        var ar apiResp
        dec := json.NewDecoder(resp.Body)
        if err := dec.Decode(&ar); err != nil {
            return nil, fmt.Errorf("decode: %w", err)
        }
        return ar.Data, nil
    }

    var fetchSplit func(ctx context.Context, names []string) ([]json.RawMessage, error)
    fetchSplit = func(ctx context.Context, names []string) ([]json.RawMessage, error) {
        // retry loop for 429/5xx
        attempt := 0
        for {
            data, err := doReq(ctx, names)
            if err == nil {
                return data, nil
            }
            var hs *httpStatusErr
            if errorsAs(err, &hs) {
                // If 400/413, split
                if hs.code == 400 || hs.code == 413 {
                    if len(names) <= 1 {
                        // skip problematic symbol
                        log.Printf("skip symbol due to %d: %s", hs.code, names[0])
                        return nil, nil
                    }
                    mid := len(names) / 2
                    left, right := names[:mid], names[mid:]
                    lData, lErr := fetchSplit(ctx, left)
                    rData, rErr := fetchSplit(ctx, right)
                    if lErr != nil { return nil, lErr }
                    if rErr != nil { return nil, rErr }
                    return append(lData, rData...), nil
                }
                // 429/5xx -> retry with backoff
                if hs.code == 429 || (hs.code >= 500 && hs.code < 600) {
                    if attempt < maxRetries {
                        back := time.Duration(250*(1<<attempt)) * time.Millisecond
                        time.Sleep(back)
                        attempt++
                        continue
                    }
                }
            }
            // other errors
            return nil, err
        }
    }

    worker := func() {
        defer wg.Done()
        for j := range jobs {
            ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
            data, err := fetchSplit(ctx, j.batch)
            cancel()
            if err != nil {
                log.Printf("batch %d error: %v", j.idx, err)
                continue
            }
            if len(data) == 0 {
                continue
            }
            // write entries
            writeMu.Lock()
            for _, raw := range data {
                if !first { _, _ = bw.WriteString(",") } else { first = false }
                _, _ = bw.Write(raw)
            }
            writeMu.Unlock()
        }
    }

    for i := 0; i < concurrency; i++ {
        wg.Add(1)
        go worker()
    }

    // enqueue jobs
    count := 0
    for i := 0; i < len(names); i += batchSize {
        end := i + batchSize
        if end > len(names) { end = len(names) }
        b := make([]string, end-i)
        copy(b, names[i:end])
        jobs <- job{idx: count, batch: b}
        count++
    }
    close(jobs)
    wg.Wait()

    // Close JSON envelope
    _, _ = bw.WriteString("],\"errorCode\":0,\"errorMsg\":null,\"errorData\":null,\"errorCodeStr\":null}")
    if err := bw.Flush(); err != nil {
        log.Fatalf("flush: %v", err)
    }
    log.Printf("done: wrote %s", outPath)
}

func readKeys(path string) ([]string, error) {
    b, err := os.ReadFile(path)
    if err != nil { return nil, err }
    var m map[string]any
    if err := json.Unmarshal(b, &m); err != nil { return nil, err }
    names := make([]string, 0, len(m))
    for k := range m { names = append(names, k) }
    sort.Strings(names)
    return names, nil
}

// errorsAs is a small local helper to avoid importing errors in many spots
func errorsAs(err error, target **httpStatusErr) bool {
    if err == nil { return false }
    if v, ok := err.(*httpStatusErr); ok {
        *target = v
        return true
    }
    return false
}
