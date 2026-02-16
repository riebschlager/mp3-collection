#!/usr/bin/env bash
set -euo pipefail

ROOT="/Users/criebschlager/Projects/mp3-collection"
SERVER_BIN="$ROOT/apps/mcp-server/mcp-server"

export MP3_COLLECTION_ROOT="$ROOT"
export MP3_WEB_DATA_DIR="${MP3_WEB_DATA_DIR:-$ROOT/data/derived/web}"
export MP3_LASTFM_DIR="${MP3_LASTFM_DIR:-$ROOT/data/inputs/lastfm}"
export MP3_ALIAS_MAP_PATH="$ROOT/apps/mcp-server/data/alias_map.json"
export GOCACHE="${GOCACHE:-/tmp/mp3-go-build}"

if [[ ! -x "$SERVER_BIN" ]]; then
  /opt/homebrew/bin/go -C "$ROOT/apps/mcp-server" build -o "$SERVER_BIN" .
fi

exec "$SERVER_BIN"
