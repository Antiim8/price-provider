<#
.SYNOPSIS
  Sync the current repo into your WSL home for faster Go builds.

.DESCRIPTION
  Mirrors (or copies) the repository from Windows to a native WSL path using the 
  built-in UNC provider (\\wsl$). Defaults to your default WSL distro and user.

.PARAMETER Distro
  WSL distro name (e.g., Ubuntu). Auto-detected if omitted.

.PARAMETER User
  Linux username inside the distro. Auto-detected if omitted.

.PARAMETER Destination
  Relative path under ~ (home) where the repo will be placed. Default: code/price-provider

.PARAMETER Mirror
  Use robocopy /MIR to mirror (deletes extraneous files at destination).

.EXAMPLE
  pwsh -File scripts/sync_to_wsl.ps1

.EXAMPLE
  pwsh -File scripts/sync_to_wsl.ps1 -Distro Ubuntu-22.04 -Destination dev/price-provider -Mirror
#>

param(
  [string]$Distro,
  [string]$User,
  [string]$Destination = 'code/price-provider',
  [switch]$Mirror
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Get-DefaultDistro {
  try {
    $list = & wsl -l -v 2>$null | Out-String
    if (-not $list) { return $null }
    foreach ($line in $list -split "`n") {
      if ($line -match '^\*\s*(?<name>\S+)') { return $Matches['name'] }
    }
    # Fallback: first non-header line from -q
    $q = (& wsl -l -q 2>$null) | Where-Object { $_ -and ($_ -notmatch 'Windows Subsystem for Linux') } | Select-Object -First 1
    if ($q) { return $q.Trim() }
  } catch { }
  return $null
}

function Get-DistroUser([string]$distro) {
  try {
    $u = & wsl -d $distro sh -lc 'printf %s "$USER"' 2>$null | Out-String
    if ($u) { return $u.Trim() }
  } catch { }
  return 'root'
}

if (-not $Distro) { $Distro = Get-DefaultDistro }
if (-not $Distro) { throw 'Unable to detect default WSL distro. Provide -Distro.' }
if (-not $User)   { $User   = Get-DistroUser $Distro }

# Resolve repo root (script is in scripts/)
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$source = $repoRoot

# Build destination UNC path: \\wsl$\<distro>\home\<user>\<Destination>
$unc = "\\wsl$\$Distro\home\$User\$Destination" -replace '/','\'
Write-Host "Syncing from" -NoNewline; Write-Host " $source " -ForegroundColor Cyan -NoNewline; Write-Host "to" -NoNewline; Write-Host " $unc" -ForegroundColor Cyan

# Ensure destination exists
New-Item -ItemType Directory -Force -Path $unc | Out-Null

# Robocopy options
$opts = @('/E','/MT:8','/R:1','/W:1','/NFL','/NDL','/NP','/XD','.git','.idea','.vscode','node_modules','bin','obj','__pycache__')
if ($Mirror) { $opts = @('/MIR','/MT:8','/R:1','/W:1','/NFL','/NDL','/NP','/XD','.git','.idea','.vscode','node_modules','bin','obj','__pycache__') }

& robocopy $source $unc * $opts | Out-Null
$code = $LASTEXITCODE

# Robocopy exit codes below 8 are success/warning
if ($code -ge 8) {
  throw "robocopy failed with exit code $code"
}

Write-Host "Done. In WSL: cd ~/$Destination" -ForegroundColor Green

