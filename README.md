# Price Provider

Clean Go service to fetch prices from multiple sources and expose them via a simple HTTP API. First source wired is SteamDT CS2 Batch Price API. The code is structured to add more providers easily.

## Structure

- `cmd/server`: HTTP server exposing `/api/quotes` and `/healthz`.
- `internal/provider`: Provider interface and quote type.
- `internal/provider/steamdt`: SteamDT batch price adapter (stdlib only).
- `internal/provider/pricempire`: Pricempire API client (as provided; unchanged).
- `internal/provider/pricempireadapter`: Adapter to our Provider interface.
- `internal/provider/skinstablexyz`: SkinstableXYZ adapter (aggregated items endpoint; filtered per request).
- `internal/httpx`: Small HTTP client wrapper with sane timeouts.

## Run

Environment variables:

- `PORT` (default `8080`)
- `STEAMDT_API_KEY` (required to reach SteamDT)
- `STEAMDT_ENDPOINT` (default `https://open.steamdt.com/open/cs2/v1/price/batch`)
- `INCLUDE_BIDS` (default `true`)
- `CURRENCY` (default `CNY`)
- `REQUEST_TIMEOUT_SEC` (default `10`)
- `STEAMDT_CACHE_TTL_SEC` (default `3`) — per-symbol cache TTL
- `STEAMDT_CACHE_MAX_ITEMS` (default `10000`)
- `PRICEMPIRE_API_KEY` (optional; enables Pricempire integration)
- `PRICEMPIRE_APP_ID` (default `730`)
- `PRICEMPIRE_CURRENCY` (default `USD`)
- `PRICEMPIRE_SOURCES` (CSV; default `buff`)
- `PRICEMPIRE_CACHE_TTL_SEC` (default `15`) — per-symbol cache TTL
- `PRICEMPIRE_CACHE_MAX_ITEMS` (default `50000`)
- `SKINSTABLE_ENABLED` (default `false`)
- `SKINSTABLE_ENDPOINT` (required when enabled)
- `SKINSTABLE_API_KEY` (optional)
- `SKINSTABLE_CURRENCY` (default `USD`)
- `SKINSTABLE_ITEMS_CACHE_TTL_SEC` (default `15`)
- `SKINSTABLE_MAX_RPM`, `SKINSTABLE_MIN_INTERVAL_SEC`, `SKINSTABLE_BURST`
- `SKINSTABLE_CACHE_TTL_SEC`, `SKINSTABLE_CACHE_MAX_ITEMS`

Config file (preferred):

- Copy `config.example.json` to `config.json` and fill in your API keys and intervals.
- Alternatively set `CONFIG_FILE` to a custom path.

Example `config.json` keys:

- `steamdt.api_key`: SteamDT token
- `steamdt.max_requests_per_minute`: token-bucket rate (cap), with optional `steamdt.burst`.
- `steamdt.burst`: bucket capacity (number of requests allowed at once). Alternatively, `steamdt.min_request_interval_sec`.
- `steamdt.max_items_per_request`: split large symbol lists into batches (e.g., 200).
- `steamdt.max_concurrency`: number of concurrent batch requests (e.g., 2-3).
- `steamdt.cache_ttl_sec`: cache quotes per symbol to reduce upstream calls.
- `steamdt.cache_max_items`: cap cache size.
- `pricempire.api_key`: Pricempire token
- `pricempire.max_requests_per_minute`: token-bucket rate (cap), with optional `pricempire.burst`.
- `pricempire.burst`: bucket capacity. Alternatively, `pricempire.min_request_interval_sec`.
- `pricempire.cache_ttl_sec`: cache quotes per symbol to reduce upstream calls.
- `pricempire.cache_max_items`: cap cache size.
- `skinstable.enabled`: enable SkinstableXYZ
- `skinstable.endpoint`: items endpoint URL
- `skinstable.api_key`: optional bearer token
- `skinstable.currency`: currency tag (e.g., `USD`)
- `skinstable.items_cache_ttl_sec`: cache full items payload
- `skinstable.max_requests_per_minute`/`min_request_interval_sec`/`burst`: rate limiting
- `skinstable.cache_ttl_sec`/`cache_max_items`: per-symbol cache wrapper

Start the server:

```
go run ./cmd/server   # reads config.json automatically if present
```

Fetch quotes:

- GET: `http://localhost:8080/api/quotes?symbols=A,B,C`
- POST: `POST /api/quotes` with body `{ "symbols": ["A","B"] }`

Response shape:

```
{
  "quotes": [
    {"symbol":"...","price":"...","currency":"CNY","source":"SteamDT:Steam:sell","received_at":"..."}
  ]
}
```

## Notes

- Prices are represented as strings to avoid float rounding and external dependencies.
- CORS is permissive by default for quick browser testing. Tighten as needed.
- Responses are gzip-compressed when supported by the client.
- Server has read/write/idle timeouts and panic recovery.
- To add more sources, implement `internal/provider.Provider` and wire into the server handler.

## SteamDT Dump CLI

- Tool: `cmd/steamdt_dump` — batches SteamDT requests using names from a JSON file and streams a combined JSON result.
- Reads `config.json` (or `config.example.json`) for `steamdt.api_key` and endpoint. You can also use env vars `STEAMDT_API_KEY`, `STEAMDT_ENDPOINT`.

Usage examples:

```
# Dump full dataset using symbols from pricempire_all_prices.json
go run ./cmd/steamdt_dump \
  --symbols-file pricempire_all_prices.json \
  --out steamdt_all_prices.json \
  --batch 50 --concurrency 4 --timeout 20 --retries 3 --rpm 0

# Or via Makefile shortcut
make steamdt-dump
```

Behavior:
- Splits batches recursively on 400/413 responses.
- Retries 429/5xx with exponential backoff.
- Streams output to avoid high memory usage: writes `{success:true,data:[...]}` structure.

## WSL Workflow

- Use the repo from WSL directly (fast to try):
  - `cd "/mnt/c/Users/Elect/Documents/Traden/Software/Website/price provider" && go run ./cmd/server`
- Or sync into native WSL filesystem for best performance:
  - From Windows PowerShell: `pwsh -File scripts/sync_to_wsl.ps1`
  - From inside WSL: `bash scripts/sync_from_windows.sh --src "C:\Users\Elect\Documents\Traden\Software\Website\price provider" --dst "$HOME/code/price-provider"`
  - Then in WSL: `cd ~/code/price-provider && go run ./cmd/server`

### WSL Setup Tips

- Ensure Go 1.22+ is installed in WSL (e.g., Ubuntu): `sudo apt update && sudo apt install -y golang`
- Prefer a native WSL path (e.g., `~/code/price-provider`) over `/mnt/c` for faster I/O.
- Optionally increase file descriptors before running: `ulimit -n 8192`.
- Helpers:
  - `make server` to run, `make build` to compile (requires Go in WSL)
  - `bash scripts/run_server_wsl.sh` bumps ulimit and runs the server
- Line endings are normalized via `.gitattributes` for cross-OS usage (LF for Go/shell, CRLF for PowerShell).
