# MP3 Collection

Historical iTunes archive + listening-history analytics + static web explorer.

This repo combines:
- iTunes export compilation from many legacy files.
- Data extraction/build pipelines in Go.
- Last.fm + Spotify listening-history merge and derived analytics.
- A static Astro site for browsing artists, albums, tracks, timeline, and wrapped-style yearly summaries.
- A local MCP server for deeper music-intel analysis workflows.

## Repository Layout

- `archive/`: raw iTunes export files plus compiled CSV/validation outputs.
- `go-scripts/`: Go command suite for extraction, listening merge, timeline/build, image metadata.
- `data/`: intermediate JSON artifacts.
- `web-data/`: web-ready JSON artifacts used by the Astro app.
- `mp3-collection-web/`: Astro frontend (`public/data` is a symlink to `../../web-data`).
- `mcp-server/`: Go MCP server for data-backed music analysis tools.

## Prerequisites

- Go 1.22+ (Go 1.21 works for `go-scripts`; `mcp-server` targets Go 1.22)
- Node.js 20+ (matches CI workflow)

## Environment Setup

```bash
cp .env.example .env
```

`.env` is used by Go commands (auto-loaded from current/parent directories):
- `LASTFM_API_KEY` (required for Last.fm API calls)
- `LASTFM_USERNAME` (optional, defaults to `riebschlager`)

## Data Pipelines

### 1. Compile iTunes Exports (when source archives change)

```bash
cd go-scripts
go run . compile-itunes-exports
```

Outputs:
- `archive/compiled_itunes_library.csv`
- `archive/validation_report.txt`

### 2. Build Data with Go (recommended)

```bash
cd go-scripts
./run_all.sh
```

`run_all.sh` executes:
1. `compile-itunes-exports`
2. `extract-tracks`
3. `extract-albums`
4. `extract-artists`
5. `fetch-lastfm`
6. `merge-listening`
7. `process-lastfm`
8. `build-timeline`
9. `build-web-data`
10. `fetch-images`

You can run any step individually:

```bash
cd go-scripts
go run . <command>
```

Common commands:
- `compile-itunes-exports` (alias: `compile-exports`)
- `extract-tracks`, `extract-albums`, `extract-artists`
- `fetch-lastfm`
- `merge-listening`
- `process-lastfm`
- `build-timeline`
- `build-web-data`
- `fetch-images` (alias: `fetch-metadata`)

## Generated Artifact Summary

- `data/tracks.json`, `data/albums.json`, `data/artists.json`
- `data/listening-history.json`, `data/listening-merge-report.json`
- `data/playcounts.json`
- `web-data/chunks/tracks-*.json`
- `web-data/artists-index.json`, `web-data/albums-index.json`, `web-data/metadata.json`
- `web-data/timeline.json`
- `web-data/playcounts.json`, `web-data/listening-merge-report.json`
- `web-data/artist-images.json`, `web-data/album-images.json`

## Run the Web App

```bash
cd mp3-collection-web
npm install
npm run dev
```

Default dev URL: `http://localhost:4321`

Build/preview locally:

```bash
npm run build
npm run preview
```

## Deployment

GitHub Pages deployment is automated via `.github/workflows/deploy.yml`.

- Trigger: push to `main`/`master` with changes in `mp3-collection-web/**`, `web-data/**`, or the workflow file.
- Site URL: `https://riebschlager.github.io/mp3-collection`

See `DEPLOYMENT.md` for full details.

## MCP Server

```bash
cd mcp-server
go run .
```

Or use the launcher script:

```bash
./mcp-server/run-mcp.sh
```

Current tools:
- `music_resolve_track_identity`
- `music_audit_match_coverage`
- `music_compare_eras`
- `music_listening_summary`
- `music_new_discoveries`
- `music_genre_profile`
- `music_listening_patterns`
- `music_find_dormant_returns`
- `music_reload_alias_map`
