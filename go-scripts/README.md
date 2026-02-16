# Go Scripts for MP3 Collection

This directory contains the primary data pipeline for the project. The commands are implemented as one Go CLI with subcommands.

## Requirements

- Go 1.21+ (`go-scripts/go.mod` uses `go 1.21`)
- Repository root contains:
  - `archive/compiled_itunes_library.csv` (from `go run . compile-itunes-exports`)
  - `spotify/Streaming_History_Audio_*.json` (for merge/timeline/playcount flow)
  - `lastfm/lastfmstats-<username>.json` (created/updated by `fetch-lastfm`)

## Environment Variables

The CLI auto-loads `.env` by searching current and parent directories.

- `LASTFM_API_KEY`: required for `fetch-lastfm` and `fetch-images`.
- `LASTFM_USERNAME`: optional, defaults to `riebschlager`.
- `SPOTIFY_MIN_MS_PLAYED`: optional, defaults to `30000`.
- `SPOTIFY_LASTFM_DEDUPE_WINDOW_MS`: optional, defaults to `120000`.
- `LASTFM_IMAGE_SCOPE`: `played` (default) or `all`.
- `LASTFM_IMAGE_FORCE_REFRESH`: optional boolean (`true/false`).
- `LASTFM_IMAGE_REFRESH_MISSING`: optional boolean (`true/false`).
- `LASTFM_IMAGE_MAX_ARTISTS`: optional int limit for fetch runs.
- `LASTFM_IMAGE_MAX_ALBUMS`: optional int limit for fetch runs.

## Quick Start

From repository root, ensure compiled CSV exists:

```bash
cd go-scripts
go run . compile-itunes-exports
```

Then run the Go pipeline:

```bash
cd go-scripts
./run_all.sh
```

`run_all.sh` builds `mp3-scripts` and runs:
1. `extract-tracks`
2. `extract-albums`
3. `extract-artists`
4. `fetch-lastfm`
5. `merge-listening`
6. `process-lastfm`
7. `build-timeline`
8. `build-web-data`
9. `fetch-images`

## Command Reference

Run commands with:

```bash
cd go-scripts
go run . <command>
```

Available commands:
- `compile-itunes-exports`: compiles raw exports in `archive/` into `archive/compiled_itunes_library.csv` and `archive/validation_report.txt`
- `compile-exports`: alias for `compile-itunes-exports`
- `extract-tracks`: writes `data/tracks.json`
- `extract-artists`: writes `data/artists.json`
- `extract-albums`: writes `data/albums.json`
- `fetch-lastfm`: fetches latest Last.fm page (up to 200 recent tracks) and appends new scrobbles
- `merge-listening`: merges Last.fm + Spotify into `data/listening-history.json` and merge reports
- `process-lastfm`: builds `data/playcounts.json` and `web-data/playcounts.json`
- `build-timeline`: builds `web-data/timeline.json`
- `build-web-data`: builds chunked/indexed web artifacts in `web-data/`
- `fetch-images`: fetches/caches Last.fm image metadata into `web-data/artist-images.json` and `web-data/album-images.json`
- `fetch-metadata`: alias for `fetch-images`

## Notes

- `process-lastfm` and `build-timeline` call shared logic that rebuilds listening history when stale relative to Last.fm/Spotify source files.
- `build-web-data` enriches tracks with playcounts from `data/playcounts.json` when available.
- Track chunks are written as `web-data/chunks/tracks-###.json` (chunk size currently 1000).

## Build Binary (optional)

```bash
cd go-scripts
go build -o mp3-scripts
./mp3-scripts build-web-data
```
