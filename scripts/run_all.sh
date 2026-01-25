#!/usr/bin/env bash
set -euo pipefail

# run_all.sh — run all Python scripts in the repository
# Usage: ./scripts/run_all.sh

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$ROOT_DIR"

PY=python3

echo "Running extractor and build scripts from: $ROOT_DIR"

echo "==> Running extract_tracks.py"
"$PY" scripts/extract_tracks.py

echo "==> Running extract_albums.py"
"$PY" scripts/extract_albums.py

echo "==> Running extract_artists.py"
"$PY" scripts/extract_artists.py

echo "==> Running build_web_data.py"
"$PY" scripts/build_web_data.py

echo "All scripts completed."
