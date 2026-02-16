# MP3 Collection

Historical iTunes archive + listening-history analytics + static web explorer.

This repo combines:
- iTunes export compilation from many legacy files.
- Data extraction/build pipelines in Go.
- Last.fm + Spotify listening-history merge and derived analytics.
- A static Astro site for browsing artists, albums, tracks, timeline, and wrapped-style yearly summaries.
- A local MCP server for deeper music-intel analysis workflows.

## Repository Layout

- `apps/web/`: Astro frontend (`public/data` symlinks to `../../../data/derived/web`).
- `apps/mcp-server/`: Go MCP server for data-backed music analysis tools.
- `tools/pipeline/`: Go command suite for extraction, listening merge, timeline/build, image metadata.
- `data/`: intermediate pipeline artifacts plus organized input/derived datasets.
  - `data/inputs/itunes`: raw iTunes export files (`Library.export*`, `.txt`).
  - `data/inputs/lastfm`, `data/inputs/spotify`: source listening-history inputs.
  - `data/derived/compiled`: compiled iTunes CSV + validation report.
  - `data/derived/core`: canonical tracks/albums/artists/history/playcounts artifacts.
  - `data/derived/web`: web-ready JSON artifacts used by the Astro app.
- Legacy compatibility links remain:
  - `archive/* -> data/inputs/itunes/*` for historical import batch folders (plus compiled output links)
  - `data/*.json -> data/derived/core/*.json`

## Prerequisites

- Go 1.22+ (Go 1.21 works for `tools/pipeline`; `apps/mcp-server` targets Go 1.22)
- Node.js 20+ (matches CI workflow)
- Git LFS (required for large dataset files tracked in this repo)

## Environment Setup

```bash
git lfs install
git lfs pull
cp .env.example .env
```

`.env` is used by Go commands (auto-loaded from current/parent directories):
- `LASTFM_API_KEY` (required for Last.fm API calls)
- `LASTFM_USERNAME` (optional, defaults to `riebschlager`)
- Optional path overrides for ETL commands:
  - `MP3_PROJECT_ROOT`
  - `MP3_ARCHIVE_DIR`, `MP3_COMPILED_DIR`, `MP3_DATA_DIR`, `MP3_WEB_DATA_DIR`, `MP3_LASTFM_DIR`, `MP3_SPOTIFY_DIR`
  - Defaults now point to the organized layout:
    - `MP3_ARCHIVE_DIR=data/inputs/itunes`
    - `MP3_COMPILED_DIR=data/derived/compiled`
    - `MP3_DATA_DIR=data/derived/core`
    - `MP3_WEB_DATA_DIR=data/derived/web`
    - `MP3_LASTFM_DIR=data/inputs/lastfm`
    - `MP3_SPOTIFY_DIR=data/inputs/spotify`

## Daily Workflow (from Repo Root)

Use root `make` targets for the common workflows:

```bash
make pipeline   # full Go pipeline refresh
make web-dev    # run Astro dev server
make mcp        # start MCP server
```

Other useful targets:
- `make compile` (rebuild compiled iTunes CSV)
- `make listening` (refresh merged listening history + playcounts)
- `make web-data` (rebuild web JSON/chunks/indexes)
- `make images` (refresh artist/album image metadata)
- `make doctor` (validate ETL path config + required inputs)
- `make help` (list all targets)

## Data Pipelines

### 1. Compile iTunes Exports (when source archives change)

```bash
cd tools/pipeline
go run . compile-itunes-exports
```

Outputs:
- Canonical:
  - `data/derived/compiled/compiled_itunes_library.csv`
  - `data/derived/compiled/validation_report.txt`
- Compatibility links:
  - `archive/compiled_itunes_library.csv`
  - `archive/validation_report.txt`

### 2. Build Data with Go (recommended)

```bash
cd tools/pipeline
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
cd tools/pipeline
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
- `doctor` (validate resolved path config and required inputs)

## Generated Artifact Summary

- `data/derived/core/tracks.json`, `data/derived/core/albums.json`, `data/derived/core/artists.json`
- `data/derived/core/listening-history.json`, `data/derived/core/listening-merge-report.json`
- `data/derived/core/playcounts.json`
- `data/derived/web/chunks/tracks-*.json`
- `data/derived/web/artists-index.json`, `data/derived/web/albums-index.json`, `data/derived/web/metadata.json`
- `data/derived/web/timeline.json`
- `data/derived/web/playcounts.json`, `data/derived/web/listening-merge-report.json`
- `data/derived/web/artist-images.json`, `data/derived/web/album-images.json`

## Run the Web App

```bash
cd apps/web
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

- Trigger: push to `main`/`master` with changes in `apps/web/**`, `data/derived/web/**`, or the workflow file.
- Site URL: `https://riebschlager.github.io/mp3-collection`

See `DEPLOYMENT.md` for full details.

## MCP Server

```bash
cd apps/mcp-server
go run .
```

Or use the launcher script:

```bash
./apps/mcp-server/run-mcp.sh
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
