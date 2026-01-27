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

echo "==> Running extract-tracks"
./mp3-scripts extract-tracks

echo "==> Running extract-albums"
./mp3-scripts extract-albums

echo "==> Running extract-artists"
./mp3-scripts extract-artists

echo "==> Running build-web-data"
./mp3-scripts build-web-data

echo "All scripts completed."
