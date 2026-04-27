CREATE TABLE IF NOT EXISTS artists (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    slug TEXT UNIQUE NOT NULL,
    image_url TEXT
);

CREATE TABLE IF NOT EXISTS albums (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    artist_id TEXT,
    slug TEXT UNIQUE NOT NULL,
    image_url TEXT,
    FOREIGN KEY(artist_id) REFERENCES artists(id)
);

CREATE TABLE IF NOT EXISTS tracks (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    album_id TEXT,
    artist_id TEXT,
    genre TEXT,
    year INTEGER,
    duration_ms INTEGER,
    track_number INTEGER,
    disc_number INTEGER,
    slug TEXT UNIQUE NOT NULL,
    FOREIGN KEY(album_id) REFERENCES albums(id),
    FOREIGN KEY(artist_id) REFERENCES artists(id)
);

CREATE TABLE IF NOT EXISTS listening_history (
    id TEXT PRIMARY KEY, 
    track_id TEXT NOT NULL,
    played_at DATETIME NOT NULL,
    source TEXT NOT NULL, 
    FOREIGN KEY(track_id) REFERENCES tracks(id)
);

CREATE INDEX IF NOT EXISTS idx_listening_history_played_at ON listening_history(played_at);
CREATE INDEX IF NOT EXISTS idx_listening_history_track_id ON listening_history(track_id);
CREATE INDEX IF NOT EXISTS idx_tracks_album_id ON tracks(album_id);
CREATE INDEX IF NOT EXISTS idx_tracks_artist_id ON tracks(artist_id);
CREATE INDEX IF NOT EXISTS idx_albums_artist_id ON albums(artist_id);

-- MATERIALIZED TABLES FOR COMPLEX ANALYTICS

CREATE TABLE IF NOT EXISTS transition_edges (
    source_type TEXT NOT NULL, -- 'track' or 'artist'
    source_id TEXT NOT NULL,
    target_id TEXT NOT NULL,
    weight INTEGER NOT NULL,
    probability REAL NOT NULL,
    PRIMARY KEY(source_type, source_id, target_id)
);

CREATE INDEX IF NOT EXISTS idx_transitions_source ON transition_edges(source_type, source_id, weight DESC);
CREATE INDEX IF NOT EXISTS idx_transitions_target ON transition_edges(source_type, target_id);

CREATE TABLE IF NOT EXISTS era_similarities (
    source_filter TEXT NOT NULL, -- 'all', 'spotify', 'lastfm'
    year_a INTEGER NOT NULL,
    year_b INTEGER NOT NULL,
    similarity_score REAL NOT NULL,
    PRIMARY KEY(source_filter, year_a, year_b)
);
