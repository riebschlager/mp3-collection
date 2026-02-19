# music-intel-mcp

Go MCP server for data-backed analysis of the MP3 collection and listening history.

## Protocol and Transport

- JSON-RPC 2.0 over stdio
- `Content-Length` framed messages
- MCP protocol version advertised by server: `2024-11-05`

## Included Tools

- `music_resolve_track_identity`
- `music_audit_match_coverage`
- `music_compare_eras`
- `music_listening_summary`
- `music_new_discoveries`
- `music_genre_profile`
- `music_listening_patterns`
- `music_streaks_and_bursts`
- `music_transition_graph`
- `music_month_story`
- `music_year_story`
- `music_batch_year_story`
- `music_find_dormant_returns`
- `music_reload_alias_map`

Most analytics tools accept optional `source` filtering: `all` (default), `lastfm`, or `spotify`.
Discovery-oriented tools (`music_new_discoveries`, `music_month_story`, `music_year_story`, `music_batch_year_story`) also accept `discoveryBaseline`: `global` (default), `source`, or `window`.

## Data Dependencies

- `<web-data>/chunks/tracks-*.json` (resolver index source)
- `<core>/listening-history.json` (merged Last.fm + Spotify listening events)
- legacy fallback: `<lastfm>/lastfmstats-<username>.json` (if merged history is unavailable)

Default values:
- `<web-data>` resolves to `<root>/data/derived/web`
- `<core>` resolves to `<root>/data/derived/core`
- `<lastfm>` resolves to `<root>/data/inputs/lastfm`
- `<username>` resolves to `LASTFM_USERNAME` or `riebschlager`

## Run

From repo root:

```bash
cd apps/mcp-server
go run .
```

Or use launcher (builds binary if missing and sets env defaults):

```bash
./apps/mcp-server/run-mcp.sh
```

## Root and Alias Path Resolution

- Project root auto-discovery: walks parent directories until track chunk data is found.
- Optional override: `MP3_COLLECTION_ROOT=/absolute/path/to/repo`.
- Optional web-data override: `MP3_WEB_DATA_DIR=/absolute/or/relative/path/to/web-data`.
- Optional merged-history file override: `MP3_LISTENING_HISTORY_FILE=/absolute/or/relative/path/to/listening-history.json`.
- Optional derived-core dir override: `MP3_DATA_DIR=/absolute/or/relative/path/to/data/derived/core`.
- Optional Last.fm dir override: `MP3_LASTFM_DIR=/absolute/or/relative/path/to/lastfm`.
- Optional Last.fm file override: `MP3_LASTFM_FILE=/absolute/or/relative/path/to/lastfmstats-*.json`.
- Alias file optional override: `MP3_ALIAS_MAP_PATH=/absolute/or/relative/path/to/alias_map.json`.
- Default alias lookup order:
  - `<root>/data/alias_map.json`
  - `<root>/apps/mcp-server/data/alias_map.json`

## Alias Map Formats

Supported format 1:

```json
{
  "aliases": [
    {
      "entityType": "artist",
      "aliasValue": "apc",
      "canonicalValue": "a perfect circle"
    },
    {
      "entityType": "track",
      "aliasValue": "judith (album version)",
      "canonicalValue": "judith"
    }
  ]
}
```

Supported format 2:

```json
{
  "artists": { "apc": "a perfect circle" },
  "tracks": { "judith (album version)": "judith" },
  "albums": {}
}
```

## Current Limitations

1. `music_audit_match_coverage` treats ambiguous matches as unmatched.
2. `music_compare_eras` genre shifts exclude unmatched tracks.
3. `music_find_dormant_returns` only evaluates canonically matched scrobbles.
4. Most date-bucketed tools evaluate timestamps in UTC (timezone-aware bucketing is currently explicit in selected tools only).
5. `music_reload_alias_map` reloads in process only; it does not edit alias files.

## Schemas

`apps/mcp-server/schemas/` includes:
- `sqlite_seed.sql`
- `canonical_tracks.schema.json`
- `alias_map.schema.json`
- `match_events.schema.json`
- `coverage_snapshots.schema.json`
