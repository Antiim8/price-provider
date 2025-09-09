#!/usr/bin/env bash
set -euo pipefail

# Simple WSL-friendly runner that bumps file descriptor limit and runs the server.

ulimit -n 8192 || true

DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$DIR"

echo "Running server with Go in WSL (ulimit -n=$(ulimit -n))"

exec go run ./cmd/server

