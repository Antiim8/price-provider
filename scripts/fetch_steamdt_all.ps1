# Fetch all SteamDT prices by batching marketHashNames from a source list.
# Defaults: reads API key + endpoint from config.json (or config.example.json),
# and reads item names from pricempire_all_prices.json keys.

[CmdletBinding()]
param(
  [string]$ConfigPath = '',
  [string]$SourceSymbolsFile = 'pricempire_all_prices.json',
  [string]$OutputPath = 'steamdt_all_prices.json',
  [int]$BatchSize = 50,
  [int]$DelayMs = 250,
  [int]$Limit = 0,
  [int]$MaxRetries = 4,
  [int]$InitialBackoffMs = 500
)

set-strictmode -version latest
$ErrorActionPreference = 'Stop'

function Resolve-ConfigPath {
  param([string]$p)
  if ($p -ne '') { return $p }
  if (Test-Path 'config.json') { return 'config.json' }
  if (Test-Path 'config.example.json') { return 'config.example.json' }
  throw 'No config.json or config.example.json found; pass -ConfigPath'
}

function Read-SteamDTConfig {
  param([string]$path)
  $cfg = Get-Content -Raw $path | ConvertFrom-Json
  $apiKey = $cfg.steamdt.api_key
  $endpoint = $cfg.steamdt.endpoint
  if (-not $apiKey) { throw "Missing steamdt.api_key in $path" }
  if (-not $endpoint) { $endpoint = 'https://open.steamdt.com/open/cs2/v1/price/batch' }
  [pscustomobject]@{ ApiKey = $apiKey; Endpoint = $endpoint }
}

function Read-AllSymbols {
  param([string]$path, [int]$limit)
  if (-not (Test-Path $path)) {
    throw "SourceSymbolsFile not found: $path"
  }
  $obj = (Get-Content -Raw $path | ConvertFrom-Json)
  $props = $obj.PSObject.Properties | ForEach-Object { $_.Name }
  if ($limit -gt 0) { $props = $props | Select-Object -First $limit }
  return ,$props
}

function Invoke-SteamDTBatchRaw {
  param([string[]]$Names, [string]$Endpoint, [string]$Token)
  $bodyObj = @{ marketHashNames = $Names }
  $body = $bodyObj | ConvertTo-Json -Compress
  $headers = @{ Authorization = ('Bearer ' + $Token); 'Content-Type' = 'application/json'; 'Accept' = 'application/json' }
  $resp = Invoke-WebRequest -Method Post -Uri $Endpoint -Headers $headers -Body $body -TimeoutSec 60 -ErrorAction Stop
  return $resp
}

function Invoke-SteamDTBatch-Retry {
  param(
    [string[]]$Names,
    [string]$Endpoint,
    [string]$Token,
    [int]$Retries,
    [int]$BackoffMs
  )
  for ($attempt = 0; $attempt -le $Retries; $attempt++) {
    try {
      $resp = Invoke-SteamDTBatchRaw -Names $Names -Endpoint $Endpoint -Token $Token
      return $resp
    } catch {
      $ex = $_.Exception
      $status = $null
      if ($ex.Response -and $ex.Response.StatusCode) { $status = [int]$ex.Response.StatusCode }
      # If request is too large or invalid, break to split logic
      if ($status -in 400,413) {
        throw [System.Exception]::new("SPLIT_REQUIRED:$status")
      }
      # Retry on 429/5xx
      if ($status -eq 429 -or ($status -ge 500 -and $status -lt 600)) {
        if ($attempt -lt $Retries) {
          Start-Sleep -Milliseconds ([Math]::Min($BackoffMs * [Math]::Pow(2,$attempt), 8000))
          continue
        }
      }
      throw
    }
  }
}

function Get-BatchResponse {
  param([string[]]$Names, [string]$Endpoint, [string]$Token, [int]$Retries, [int]$BackoffMs)
  try {
    $resp = Invoke-SteamDTBatch-Retry -Names $Names -Endpoint $Endpoint -Token $Token -Retries $Retries -BackoffMs $BackoffMs
    return $resp.Content
  } catch {
    $msg = $_.Exception.Message
    if ($msg -like 'SPLIT_REQUIRED:*') {
      if ($Names.Count -le 1) {
        Write-Warning ("Skipping symbol due to repeated 400: '{0}'" -f $Names[0])
        return '{"success":true,"data":[],"errorCode":0,"errorMsg":null,"errorData":null,"errorCodeStr":null}'
      }
      $mid = [int]([Math]::Floor($Names.Count / 2))
      $left = $Names[0..($mid-1)]
      $right = $Names[$mid..($Names.Count-1)]
      Write-Warning ("Splitting batch of {0} into {1} and {2} due to {3}" -f $Names.Count, $left.Count, $right.Count, $msg)
      $rawL = Get-BatchResponse -Names $left -Endpoint $Endpoint -Token $Token -Retries $Retries -BackoffMs $BackoffMs
      $rawR = Get-BatchResponse -Names $right -Endpoint $Endpoint -Token $Token -Retries $Retries -BackoffMs $BackoffMs
      $objL = $rawL | ConvertFrom-Json
      $objR = $rawR | ConvertFrom-Json
      $merged = Merge-DataArrays -Chunks @($objL, $objR)
      return ($merged | ConvertTo-Json -Depth 6)
    }
    throw
  }
}

function Merge-DataArrays {
  param([object[]]$Chunks)
  # Each chunk is a parsed response object. We combine their data arrays.
  $all = New-Object System.Collections.Generic.List[object]
  foreach ($c in $Chunks) {
    if ($null -ne $c.data) { $c.data | ForEach-Object { [void]$all.Add($_) } }
  }
  [pscustomobject]@{
    success = $true
    data = $all
    errorCode = 0
    errorMsg = $null
    errorData = $null
    errorCodeStr = $null
  }
}

Write-Host "Resolving config..." -ForegroundColor Cyan
$cfgPath = Resolve-ConfigPath -p $ConfigPath
$st = Read-SteamDTConfig -path $cfgPath

Write-Host "Loading symbols from $SourceSymbolsFile ..." -ForegroundColor Cyan
$names = Read-AllSymbols -path $SourceSymbolsFile -limit $Limit
$total = $names.Count
if ($total -eq 0) { throw 'No symbols loaded' }
Write-Host ("Symbols to fetch: {0}" -f $total)

# Stream write to avoid high memory usage
Write-Host ("Streaming full result to {0} ..." -f $OutputPath) -ForegroundColor Cyan
if (Test-Path $OutputPath) { Remove-Item $OutputPath -Force }
Add-Content -Path $OutputPath -Value '{"success":true,"data":[' -Encoding utf8
$first = $true

for ($i = 0; $i -lt $total; $i += $BatchSize) {
  $batch = $names[$i..([Math]::Min($i+$BatchSize-1, $total-1))]
  Write-Host ("Requesting batch {0}-{1} of {2} (size={3})..." -f $i, ($i + $batch.Count - 1), $total, $batch.Count)
  $raw = Get-BatchResponse -Names $batch -Endpoint $st.Endpoint -Token $st.ApiKey -Retries $MaxRetries -BackoffMs $InitialBackoffMs
  try {
    $parsed = $raw | ConvertFrom-Json
  } catch {
    Write-Warning ("Failed to parse JSON for batch {0}-{1}; skipping" -f $i, ($i + $batch.Count - 1))
    $parsed = $null
  }
  if ($parsed -and -not $parsed.success) {
    Write-Warning ("Batch {0}-{1} returned success=false: code={2} msg={3}" -f $i, ($i + $batch.Count - 1), $parsed.errorCode, $parsed.errorMsg)
  }
  if ($parsed -and $null -ne $parsed.data) {
    foreach ($e in $parsed.data) {
      $json = ($e | ConvertTo-Json -Depth 6 -Compress)
      if (-not $first) {
        Add-Content -Path $OutputPath -Value ',' -Encoding utf8
      } else { $first = $false }
      Add-Content -Path $OutputPath -Value $json -Encoding utf8
    }
  }
  if ($DelayMs -gt 0 -and $i + $BatchSize -lt $total) { Start-Sleep -Milliseconds $DelayMs }
}

Add-Content -Path $OutputPath -Value '],"errorCode":0,"errorMsg":null,"errorData":null,"errorCodeStr":null}' -Encoding utf8
Write-Host "Done." -ForegroundColor Green
