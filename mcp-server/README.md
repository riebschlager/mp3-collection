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
- `music_find_dormant_returns`
- `music_reload_alias_map`

## Data Dependencies

- `web-data/chunks/tracks-*.json` (resolver index source)
- `lastfm/lastfmstats-riebschlager.json` (scrobble analyses)

Note: current server code reads Last.fm scrobbles from `lastfm/lastfmstats-riebschlager.json`.

## Run

From repo root:

```bash
cd mcp-server
go run .
```

Or use launcher (builds binary if missing and sets env defaults):

```bash
./mcp-server/run-mcp.sh
```

## Root and Alias Path Resolution

- Project root auto-discovery: walks parent directories until `web-data/chunks` exists.
- Optional override: `MP3_COLLECTION_ROOT=/absolute/path/to/repo`.
- Alias file optional override: `MP3_ALIAS_MAP_PATH=/absolute/or/relative/path/to/alias_map.json`.
- Default alias lookup order:
  - `<root>/data/alias_map.json`
  - `<root>/mcp-server/data/alias_map.json`

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
4. `music_reload_alias_map` reloads in process only; it does not edit alias files.

## Schemas

`mcp-server/schemas/` includes:
- `sqlite_seed.sql`
- `canonical_tracks.schema.json`
- `alias_map.schema.json`
- `match_events.schema.json`
- `coverage_snapshots.schema.json`
