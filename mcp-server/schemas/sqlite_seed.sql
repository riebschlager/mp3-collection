PRAGMA foreign_keys = ON;

-- Canonical track entities produced by identity resolution.
CREATE TABLE IF NOT EXISTS canonical_tracks (
  canonical_track_id TEXT PRIMARY KEY,
  canonical_artist TEXT NOT NULL,
  canonical_track TEXT NOT NULL,
  canonical_album TEXT,
  duration_sec INTEGER,
  release_year INTEGER,
  first_seen_ts_ms INTEGER,
  last_seen_ts_ms INTEGER,
  source_count INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_canonical_tracks_artist_track
  ON canonical_tracks (canonical_artist, canonical_track);

-- Variant forms for artists/tracks/albums and manual overrides.
CREATE TABLE IF NOT EXISTS alias_map (
  alias_id INTEGER PRIMARY KEY AUTOINCREMENT,
  entity_type TEXT NOT NULL CHECK (entity_type IN ('artist', 'track', 'album')),
  canonical_track_id TEXT,
  canonical_value TEXT NOT NULL,
  alias_value TEXT NOT NULL,
  normalized_alias TEXT NOT NULL,
  confidence REAL NOT NULL DEFAULT 1.0 CHECK (confidence >= 0 AND confidence <= 1),
  source TEXT,
  notes TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (canonical_track_id) REFERENCES canonical_tracks (canonical_track_id)
);

CREATE INDEX IF NOT EXISTS idx_alias_map_lookup
  ON alias_map (entity_type, normalized_alias);

-- Per-scrobble resolver decisions for observability and replay.
CREATE TABLE IF NOT EXISTS match_events (
  match_event_id INTEGER PRIMARY KEY AUTOINCREMENT,
  scrobble_hash TEXT NOT NULL,
  scrobble_ts_ms INTEGER,
  raw_artist TEXT NOT NULL,
  raw_track TEXT NOT NULL,
  raw_album TEXT,
  normalized_artist TEXT,
  normalized_track TEXT,
  canonical_track_id TEXT,
  match_status TEXT NOT NULL CHECK (match_status IN ('matched', 'ambiguous', 'unmatched')),
  match_method TEXT,
  confidence REAL NOT NULL DEFAULT 0 CHECK (confidence >= 0 AND confidence <= 1),
  evidence_json TEXT,
  run_id TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
  FOREIGN KEY (canonical_track_id) REFERENCES canonical_tracks (canonical_track_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_match_events_scrobble_hash
  ON match_events (scrobble_hash);

CREATE INDEX IF NOT EXISTS idx_match_events_status
  ON match_events (match_status, scrobble_ts_ms);

-- Materialized quality snapshots for trend analysis.
CREATE TABLE IF NOT EXISTS coverage_snapshots (
  snapshot_id INTEGER PRIMARY KEY AUTOINCREMENT,
  snapshot_date TEXT NOT NULL,
  period_start TEXT,
  period_end TEXT,
  group_by TEXT NOT NULL CHECK (group_by IN ('all', 'month', 'year')),
  total_scrobbles INTEGER NOT NULL DEFAULT 0,
  matched_scrobbles INTEGER NOT NULL DEFAULT 0,
  unmatched_scrobbles INTEGER NOT NULL DEFAULT 0,
  match_rate REAL NOT NULL DEFAULT 0,
  library_tracks_total INTEGER NOT NULL DEFAULT 0,
  library_tracks_with_plays INTEGER NOT NULL DEFAULT 0,
  library_track_coverage REAL NOT NULL DEFAULT 0,
  failure_clusters_json TEXT,
  created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_coverage_snapshots_period
  ON coverage_snapshots (snapshot_date, group_by, period_start, period_end);
