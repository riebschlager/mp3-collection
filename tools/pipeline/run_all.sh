#!/usr/bin/env bash
set -euo pipefail

# run_all.sh — run all Go scripts
# Usage: ./run_all.sh

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

echo "Building Go scripts..."
if ! command -v go &> /dev/null; then
    echo "Error: 'go' is not installed or not in PATH."
    echo "Please install Go following the instructions in README.md"
    exit 1
fi

go build -o mp3-scripts

LASTFM_IMAGE_SCOPE="${LASTFM_IMAGE_SCOPE:-all}"
echo "Using LASTFM_IMAGE_SCOPE=${LASTFM_IMAGE_SCOPE}"

echo "==> Running compile-itunes-exports"
./mp3-scripts compile-itunes-exports

echo "==> Running extract-tracks"
./mp3-scripts extract-tracks

echo "==> Running extract-albums"
./mp3-scripts extract-albums

echo "==> Running extract-artists"
./mp3-scripts extract-artists

echo "==> Running fetch-lastfm"
./mp3-scripts fetch-lastfm

echo "==> Running merge-listening"
./mp3-scripts merge-listening

echo "==> Running process-lastfm"
./mp3-scripts process-lastfm

echo "==> Running build-timeline"
./mp3-scripts build-timeline

echo "==> Running build-transition-graph"
./mp3-scripts build-transition-graph

echo "==> Running build-transition-query-cache"
./mp3-scripts build-transition-query-cache

echo "==> Running build-wrapped-stories"
./mp3-scripts build-wrapped-stories

echo "==> Running build-wrapped-month-stories"
./mp3-scripts build-wrapped-month-stories

echo "==> Running build-web-data"
./mp3-scripts build-web-data

echo "==> Running fetch-images"
LASTFM_IMAGE_SCOPE="${LASTFM_IMAGE_SCOPE}" ./mp3-scripts fetch-images

echo "All scripts completed."
