#!/usr/bin/env bash
set -euo pipefail

# Sync this repo from a Windows path into native WSL filesystem.
# Requires: running in WSL. rsync is optional (uses cp fallback if missing).

usage() {
  cat <<'EOF'
Usage: scripts/sync_from_windows.sh --src "C:\\path\\to\\repo" [--dst "~/code/price-provider"] [--mirror]

Options:
  --src     Windows source path of the repo (required)
  --dst     Destination path in WSL (default: ~/code/price-provider)
  --mirror  Delete extraneous files at destination (mirror)
EOF
}

SRC_WIN=""
DST_DEFAULT="$HOME/code/price-provider"
DST="$DST_DEFAULT"
MIRROR=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --src) SRC_WIN="$2"; shift 2;;
    --dst) DST="$2"; shift 2;;
    --mirror) MIRROR=true; shift;;
    -h|--help) usage; exit 0;;
    *) echo "Unknown arg: $1"; usage; exit 1;;
  esac
done

if [[ -z "$SRC_WIN" ]]; then
  echo "--src is required (Windows path)" >&2
  usage; exit 1
fi

# Convert Windows path to WSL path
SRC=$(wslpath -a "$SRC_WIN")

mkdir -p "$DST"

if command -v rsync >/dev/null 2>&1; then
  if $MIRROR; then
    rsync -a --delete --info=progress2 --exclude '.git/' --exclude '.idea/' --exclude '.vscode/' --exclude 'node_modules/' --exclude 'bin/' --exclude 'obj/' --exclude '__pycache__/' "$SRC"/ "$DST"/
  else
    rsync -a --info=progress2 --exclude '.git/' --exclude '.idea/' --exclude '.vscode/' --exclude 'node_modules/' --exclude 'bin/' --exclude 'obj/' --exclude '__pycache__/' "$SRC"/ "$DST"/
  fi
else
  shopt -s dotglob
  if $MIRROR; then
    rm -rf "$DST"/*
  fi
  cp -a "$SRC"/* "$DST"/
fi

echo "Synced to $DST"

