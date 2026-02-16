#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEFAULT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
ROOT="${MP3_COLLECTION_ROOT:-$DEFAULT_ROOT}"
SERVER_BIN="$ROOT/apps/mcp-server/mcp-server"
GO_BIN="${GO_BIN:-$(command -v go || true)}"

export MP3_COLLECTION_ROOT="$ROOT"
export MP3_WEB_DATA_DIR="${MP3_WEB_DATA_DIR:-$ROOT/data/derived/web}"
export MP3_LASTFM_DIR="${MP3_LASTFM_DIR:-$ROOT/data/inputs/lastfm}"
export MP3_ALIAS_MAP_PATH="$ROOT/apps/mcp-server/data/alias_map.json"
export GOCACHE="${GOCACHE:-/tmp/mp3-go-build}"

if [[ ! -x "$SERVER_BIN" ]]; then
  if [[ -z "$GO_BIN" ]]; then
    echo "Error: go binary not found in PATH. Install Go or set GO_BIN." >&2
    exit 1
  fi
  "$GO_BIN" -C "$ROOT/apps/mcp-server" build -o "$SERVER_BIN" .
fi

exec "$SERVER_BIN"
