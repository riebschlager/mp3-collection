# Pipeline CLI for MP3 Collection

This directory contains the primary data pipeline for the project. The commands are implemented as one Go CLI with subcommands.

## Requirements

- Go 1.21+ (`tools/pipeline/go.mod` uses `go 1.21`)
- Repository root contains:
  - `data/inputs/itunes/` raw export files (`Library.export*`, `.txt`) for `compile-itunes-exports`
  - `data/inputs/spotify/Streaming_History_Audio_*.json` (for merge/timeline/playcount flow)
  - `data/inputs/lastfm/lastfmstats-<username>.json` (created/updated by `fetch-lastfm`)

## Environment Variables

The CLI auto-loads `.env` by searching current and parent directories.

- `LASTFM_API_KEY`: required for `fetch-lastfm` and `fetch-images`.
- `LASTFM_USERNAME`: optional, defaults to `riebschlager`.
- `MP3_PROJECT_ROOT`: optional path override for repository root.
- `MP3_ARCHIVE_DIR`: optional path override (default: `<root>/data/inputs/itunes`).
- `MP3_COMPILED_DIR`: optional path override (default: `<root>/data/derived/compiled`).
- `MP3_DATA_DIR`: optional path override (default: `<root>/data/derived/core`).
- `MP3_WEB_DATA_DIR`: optional path override (default: `<root>/data/derived/web`).
- `MP3_LASTFM_DIR`: optional path override (default: `<root>/data/inputs/lastfm`).
- `MP3_SPOTIFY_DIR`: optional path override (default: `<root>/data/inputs/spotify`).
- `SPOTIFY_MIN_MS_PLAYED`: optional, defaults to `30000`.
- `SPOTIFY_LASTFM_DEDUPE_WINDOW_MS`: optional, defaults to `120000`.
- `LASTFM_IMAGE_SCOPE`: `played` or `all` (CLI default: `played`; `run_all.sh` default: `all`).
- `LASTFM_IMAGE_FORCE_REFRESH`: optional boolean (`true/false`).
- `LASTFM_IMAGE_REFRESH_MISSING`: optional boolean (`true/false`).
- `LASTFM_IMAGE_MAX_ARTISTS`: optional int limit for fetch runs.
- `LASTFM_IMAGE_MAX_ALBUMS`: optional int limit for fetch runs.
- `LASTFM_IMAGE_NOT_FOUND_TTL_DAYS`: optional int, default `30` (retry cached `not_found` entries after this many days).
- `LASTFM_IMAGE_ERROR_TTL_HOURS`: optional int, default `24` (retry cached `error` entries after this many hours).
- `MP3_TRANSITION_SESSION_GAP_MINUTES`: optional int for transition graph session boundary, defaults to `30`.
- `MP3_TRANSITION_MIN_EDGE_WEIGHT`: optional int minimum transition count to keep an edge, defaults to `2`.
- `MP3_TRANSITION_MAX_EDGES`: optional int maximum retained edges per scope (`track`, `artist`), defaults to `2500`.
- `MP3_TRANSITION_INCLUDE_SELF_LOOPS`: optional boolean (`true/false`), defaults to `false`.
- `MP3_TRANSITION_QUERY_SOURCES`: optional comma-delimited source list for query cache (default: `all,lastfm,spotify`).
- `MP3_TRANSITION_QUERY_SESSION_GAP_MINUTES`: optional int for MCP query-cache session boundary, defaults to `30`.
- `MP3_TRANSITION_QUERY_MIN_EDGE_WEIGHT`: optional int minimum transition count for MCP slices, defaults to `2`.
- `MP3_TRANSITION_QUERY_MAX_EDGES`: optional int maximum retained edges per MCP slice scope, defaults to `180`.
- `MP3_TRANSITION_QUERY_INCLUDE_SELF_LOOPS`: optional boolean (`true/false`), defaults to `false`.
- `MP3_ERA_SIMILARITY_SOURCES`: optional comma-delimited source list for era cache (default: `all,lastfm,spotify`).
- `MP3_ERA_SIMILARITY_TOP_N`: optional int `topN` passed to MCP `music_compare_eras`, defaults to `20`.

## Quick Start

From repository root, ensure compiled CSV exists:

```bash
cd tools/pipeline
go run . compile-itunes-exports
```

Then run the Go pipeline:

```bash
cd tools/pipeline
./run_all.sh
```

`run_all.sh` builds `mp3-scripts` and runs:
1. `compile-itunes-exports`
2. `extract-tracks`
3. `extract-albums`
4. `extract-artists`
5. `fetch-lastfm`
6. `merge-listening`
7. `process-lastfm`
8. `build-timeline`
9. `build-transition-graph`
10. `build-transition-query-cache`
11. `build-era-similarity-cache`
12. `build-wrapped-stories`
13. `build-wrapped-month-stories`
14. `build-web-data`
15. `fetch-images` (with `LASTFM_IMAGE_SCOPE` defaulted to `all` by `run_all.sh`)

## Command Reference

Run commands with:

```bash
cd tools/pipeline
go run . <command>
```

Available commands:
- `compile-itunes-exports`: compiles raw exports in `data/inputs/itunes/` into `data/derived/compiled/compiled_itunes_library.csv` and `data/derived/compiled/validation_report.txt`
- `compile-exports`: alias for `compile-itunes-exports`
- `extract-tracks`: writes `data/derived/core/tracks.json`
- `extract-artists`: writes `data/derived/core/artists.json`
- `extract-albums`: writes `data/derived/core/albums.json`
- `fetch-lastfm`: fetches latest Last.fm page (up to 200 recent tracks) and appends new scrobbles
- `merge-listening`: merges Last.fm + Spotify into `data/derived/core/listening-history.json` and merge reports
- `process-lastfm`: builds `data/derived/core/playcounts.json` and `data/derived/web/playcounts.json`
- `process-listening`: alias for `process-lastfm`
- `build-timeline`: builds `data/derived/web/timeline.json`
- `build-transition-graph`: builds `data/derived/core/transition-graph.json` and `data/derived/web/transition-graph.json`
- `build-transition-query-cache`: calls MCP `music_transition_graph` per source/year and writes `data/derived/web/transition-query-cache.json`
- `build-era-similarity-cache`: calls MCP `music_compare_eras` across year pairs and sources, then writes `data/derived/web/era-similarity-cache.json`
- `build-wrapped-stories`: calls MCP `music_batch_year_story` (with per-year fallback) and writes `data/derived/web/wrapped-stories.json`
- `build-wrapped-month-stories`: calls MCP `music_month_story` for each available month and writes `data/derived/web/wrapped-month-stories.json`
- `build-web-data`: builds chunked/indexed web artifacts in `data/derived/web/`
- `fetch-images`: fetches/caches Last.fm image metadata into `data/derived/web/artist-images.json` and `data/derived/web/album-images.json`
- `fetch-metadata`: alias for `fetch-images`
- `doctor`: validates resolved path config, required inputs, and output directories

## Notes

- `process-lastfm` and `build-timeline` call shared logic that rebuilds listening history when stale relative to Last.fm/Spotify source files.
- `build-wrapped-stories` launches the local MCP server (`apps/mcp-server/run-mcp.sh`) and can be tuned with:
  - `MP3_WRAPPED_TIMEZONE` (default: `UTC`)
  - `MP3_WRAPPED_SOURCE` (default: `all`, options: `all|lastfm|spotify`)
  - `MP3_WRAPPED_DISCOVERY_BASELINE` (default: `global`, options: `global|source|window`)
- `build-wrapped-month-stories` shares the same env vars above and also supports:
  - `MP3_WRAPPED_MONTH_INCLUDE_DORMANT` (default: `false`, options: `true|false`)
- `build-web-data` enriches tracks with playcounts from `data/derived/core/playcounts.json` when available.
- `fetch-images` applies artist-image fallback for non-canonical album labels (genre buckets, chart buckets, obvious live/bootleg labels) to reduce visible missing artwork.
- Track chunks are written as `data/derived/web/chunks/tracks-###.json` (chunk size currently 1000).

## Build Binary (optional)

```bash
cd tools/pipeline
go build -o mp3-scripts
./mp3-scripts build-web-data
```
