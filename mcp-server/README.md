# music-intel-mcp (starter)

Minimal Go MCP server scaffold for deep analysis of the MP3 collection and Last.fm timeline.

## Included tools

- `music.resolve_track_identity`
- `music.audit_match_coverage`
- `music.compare_eras`
- `music.find_dormant_returns`
- `music.reload_alias_map`

All tools are implemented with real data-backed outputs.
- `music.resolve_track_identity`: exact/fuzzy matching with alias override support.
- `music.audit_match_coverage`: match-rate audit, failure clustering, and unmatched rankings.
- `music.compare_eras`: overlap, novelty/entropy, rising/falling tracks, and genre shift.
- `music.find_dormant_returns`: tracks with long inactivity gaps that reappear in a target return window.
- `music.reload_alias_map`: in-process alias + resolver index reload after alias file edits.

## Run

```bash
cd /Users/criebschlager/Projects/mp3-collection/mcp-server
go run .
```

The server speaks JSON-RPC 2.0 over stdio with `Content-Length` framing.
By default it auto-discovers the repo root by walking parent directories for `web-data/chunks`.
You can also set `MP3_COLLECTION_ROOT` explicitly.

## Alias overrides

The resolver supports manual alias overrides to force canonical matching.

- Default alias file lookup:
  - `/Users/criebschlager/Projects/mp3-collection/data/alias_map.json`
  - `/Users/criebschlager/Projects/mp3-collection/mcp-server/data/alias_map.json`
- Override with env var:
  - `MP3_ALIAS_MAP_PATH=/absolute/or/relative/path/to/alias_map.json`

Supported formats:

```json
{
  "aliases": [
    { "entityType": "artist", "aliasValue": "apc", "canonicalValue": "a perfect circle" },
    { "entityType": "track", "aliasValue": "judith (album version)", "canonicalValue": "judith" }
  ]
}
```

or grouped:

```json
{
  "artists": { "apc": "a perfect circle" },
  "tracks": { "judith (album version)": "judith" },
  "albums": {}
}
```

## Current limitations

1. `music.audit_match_coverage` currently treats ambiguous matches as unmatched.
2. `music.compare_eras` genre shifts depend on exact track-to-library resolution (unmatched tracks are excluded from genre stats).
3. `music.find_dormant_returns` only analyzes canonically matched scrobbles.
4. `music.reload_alias_map` refreshes aliases in-process, but does not persist alias edits for you.

## Seed schemas

`/Users/criebschlager/Projects/mp3-collection/mcp-server/schemas` contains:

- `sqlite_seed.sql`
- `canonical_tracks.schema.json`
- `alias_map.schema.json`
- `match_events.schema.json`
- `coverage_snapshots.schema.json`
