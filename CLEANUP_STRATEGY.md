# Project Cleanup Strategy

## Goal

Reorganize the repository so code, raw inputs, generated artifacts, and deploy outputs are clearly separated without breaking the existing ETL pipeline, web deploy, or MCP tooling.

## Current Friction

1. Root mixes concerns: source code, raw data, generated data, deploy assets, and binaries.
2. Go scripts and MCP server rely on hardcoded directory names (`archive`, `data`, `web-data`, `spotify`, `lastfm`).
3. Generated binaries are currently committed:
   - `go-scripts/mp3-scripts`
   - `mcp-server/mcp-server`
4. No root `.gitignore`, so ignore rules are fragmented.
5. Deployment and web app currently depend on `web-data/` living at repository root.

## Design Principles

1. Keep runtime behavior stable during migration.
2. Move structure in phases, not in one big refactor.
3. Introduce path indirection before moving folders.
4. Preserve deploy compatibility until final cutover.
5. Make generated vs source artifacts explicit.

## Target Layout

```text
apps/
  web/                  # Astro app (from mp3-collection-web)
  mcp-server/           # MCP server (from mcp-server)

tools/
  pipeline/             # Go ETL CLI (from go-scripts)

data/
  inputs/
    itunes/             # raw export text files
    spotify/            # streaming history source files
    lastfm/             # scrobble source dumps
  derived/
    compiled/           # compiled csv + validation
    core/               # tracks/albums/artists/history/playcounts
    web/                # deploy-ready web JSON payloads
  cache/
    images/             # optional image/metadata cache

docs/
  architecture.md
```

Transitional compatibility:
- keep `web-data/` as a symlink or mirror to `data/derived/web` until deployment updates are complete.

## Migration Plan

### Phase 1: Repository Hygiene (low risk)

1. Add root `.gitignore` covering:
   - `**/.DS_Store`
   - `go-scripts/mp3-scripts`
   - `mcp-server/mcp-server`
   - common temp/log/cache files
2. Remove tracked binaries from git history going forward (stop tracking in current tree).
3. Add a root task entrypoint (`Makefile` or `justfile`) to standardize common commands.
4. Document a single "daily workflow" in root `README.md`.

Acceptance:
- clean `git status` after local build/run.
- no generated binaries committed.

### Phase 2: Path Abstraction Layer (required before folder moves)

1. Add centralized path config in Go ETL (`tools/pipeline`):
   - read from env vars with current directory defaults.
2. Update all ETL commands to use config paths instead of direct `filepath.Join(ProjectRoot, "...")`.
3. Add equivalent path config handling in MCP server root detection.
4. Add a `doctor` command to validate required inputs and expected outputs.

Acceptance:
- existing commands still work with old layout.
- commands also work when paths are overridden via env.

### Phase 3: Directory Moves + Compatibility

1. Move code directories:
   - `mp3-collection-web` -> `apps/web`
   - `go-scripts` -> `tools/pipeline`
   - `mcp-server` -> `apps/mcp-server`
2. Move data directories under `data/inputs` and `data/derived`.
3. Keep compatibility symlinks from old paths during transition window.
4. Update all scripts/docs/workflows to prefer new paths.

Acceptance:
- pipeline, web build, and MCP startup work from new structure.
- old command paths still function through compatibility links.

### Phase 4: Deploy and Contract Cleanup

1. Update GitHub Actions path triggers and build paths.
2. Remove legacy path compatibility links after one stable cycle.
3. Finalize docs and onboarding instructions.

Acceptance:
- deploy succeeds from new structure.
- no references to legacy folder names in code/docs (except migration notes).

## Versioning Policy Decisions (to confirm)

1. Keep `data/derived/web` tracked in git for static deploy continuity? (likely yes)
2. Keep large source datasets (`spotify`, `lastfm`, compiled csv) in git, or move to Git LFS / external storage? Resolved: move large files to Git LFS.
3. Keep caches versioned (`image-cache`) or generate on demand?

## Suggested Execution Order

1. Execute Phase 1 in a small PR.
2. Execute Phase 2 in a separate PR with no folder moves.
3. Execute Phase 3 and 4 once path abstraction is proven.

## Progress

As of February 16, 2026:
- Phase 1 item 1 completed: root `.gitignore` added.
- Phase 1 item 2 completed: generated binaries are no longer tracked in git.
- Phase 1 item 3 completed: root `Makefile` added for common workflows.
- Phase 1 item 4 completed: root `README.md` now documents a daily workflow from repo root.
- Phase 2 item 1 completed: centralized ETL path config added (`MP3_*_DIR` + `MP3_PROJECT_ROOT` support).
- Phase 2 item 2 completed: ETL commands now resolve inputs/outputs via shared path config helpers.
- Phase 2 item 3 completed: MCP root/data resolution now supports `MP3_WEB_DATA_DIR`, `MP3_LASTFM_DIR`, and `MP3_LASTFM_FILE`.
- Phase 2 item 4 completed: `doctor` command added to validate path config and required inputs.
