# System Agents

This project uses a suite of Go-based "Agents" (subcommands) to manage the lifecycle of music data.
Each agent has a specific responsibility in the ETL (Extract, Transform, Load) pipeline.

## 🏗️ The Builder Agent
**Script:** `tools/pipeline/build_web_data.go` (invoked via `mp3-scripts build-web-data`)

Transforms compiled CSV + playcount data into web-optimized JSON artifacts.

**Capabilities:**
-   **Slugification:** Converts artist and album names into URL-safe slugs.
-   **Indexing:** Creates `artists-index.json` and `albums-index.json`.
-   **Chunking:** Partitions tracks into `data/derived/web/chunks/tracks-###.json`.
-   **Enrichment:** Merges listening `playCount` data onto track records.
-   **Sanitization:** Handles missing values, type conversion, and duration formatting.

**Usage:**
```bash
cd tools/pipeline
go run . build-web-data
```

---

## 📚 The Compiler Agent
**Script:** `tools/pipeline/compile_itunes_exports.go` (invoked via `mp3-scripts compile-itunes-exports`)

Ingests disjointed iTunes export files and compiles them into one normalized CSV.

**Capabilities:**
-   **Discovery:** Recursively finds `Library.export*` and `.txt` files across nested directories.
-   **Pattern Recognition:** Automatically detects header rows and data start points in non-standard text files.
-   **Unification:** Merges hundreds of files into a single `compiled_itunes_library.csv`.
-   **Deduplication:** Removes exact duplicate song rows (excluding metadata fields).
-   **Validation:** Writes `validation_report.txt` with file format and quality stats.

**Usage:**
```bash
cd tools/pipeline
go run . compile-itunes-exports
```

---

## 🔍 The Extractor Agents
**Scripts:**
-   `tools/pipeline/extract_tracks.go`
-   `tools/pipeline/extract_albums.go`
-   `tools/pipeline/extract_artists.go`

Specialized workers focused on isolating specific entities from the dataset.

**Capabilities:**
-   **Focused Extraction:** Each agent focuses on a single entity type (Tracks, Albums, or Artists).
-   **JSON Serialization:** Outputs clean, formatted JSON lists for external use or simple analysis.

**Usage:**
```bash
cd tools/pipeline
go run . extract-tracks
go run . extract-albums
go run . extract-artists
```

---

## 📈 Listening History Agents
**Scripts:**
-   `tools/pipeline/fetch_lastfm.go`
-   `tools/pipeline/merge_listening.go`
-   `tools/pipeline/process_lastfm.go`
-   `tools/pipeline/build_timeline.go`
-   `tools/pipeline/fetch_metadata.go`

Builds and enriches listening-history data from Last.fm and Spotify sources.

**Capabilities:**
-   **Fetch:** Pulls latest scrobbles from Last.fm.
-   **Merge:** Deduplicates and merges Last.fm + Spotify listening events.
-   **Aggregate:** Computes track playcounts and source breakdowns.
-   **Timeline:** Produces yearly/monthly timeline summaries.
-   **Artwork Metadata:** Caches artist/album image URLs from Last.fm.

**Usage:**
```bash
cd tools/pipeline
go run . fetch-lastfm
go run . merge-listening
go run . process-lastfm
go run . build-timeline
go run . fetch-images
```

---

## 🔁 Transition Graph Agent
**Script:** `tools/pipeline/build_transition_graph.go` (invoked via `mp3-scripts build-transition-graph`)

Builds a session-aware transition graph from merged listening history for flow visualizations.

**Capabilities:**
-   **Session Segmentation:** Applies a configurable session gap (default 30 minutes).
-   **Flow Edges:** Computes weighted transitions for both `track` and `artist` scopes.
-   **Probability Metrics:** Includes per-edge conditional transition probabilities.
-   **Noise Control:** Supports edge-count filtering and max-edge caps for visualization-ready output.

**Usage:**
```bash
cd tools/pipeline
go run . build-transition-graph
```
