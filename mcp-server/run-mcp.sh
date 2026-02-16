#!/usr/bin/env bash
set -euo pipefail

ROOT="/Users/criebschlager/Projects/mp3-collection"
SERVER_BIN="$ROOT/mcp-server/mcp-server"

export MP3_COLLECTION_ROOT="$ROOT"
export MP3_WEB_DATA_DIR="${MP3_WEB_DATA_DIR:-$ROOT/web-data}"
export MP3_LASTFM_DIR="${MP3_LASTFM_DIR:-$ROOT/lastfm}"
export MP3_ALIAS_MAP_PATH="$ROOT/mcp-server/data/alias_map.json"
export GOCACHE="${GOCACHE:-/tmp/mp3-go-build}"

if [[ ! -x "$SERVER_BIN" ]]; then
  /opt/homebrew/bin/go -C "$ROOT/mcp-server" build -o "$SERVER_BIN" .
fi

exec "$SERVER_BIN"
