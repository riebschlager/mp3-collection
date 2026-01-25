# System Agents

This project utilizes a suite of specialized "Agents" (scripts) to manage the lifecycle of the music data. Each agent has a specific responsibility in the ETL (Extract, Transform, Load) pipeline.

## 🏗️ The Builder Agent
**Script:** `scripts/build_web_data.py`

The core intelligence of the pipeline. It transforms the flat CSV data into a relational, web-optimized structure.

**Capabilities:**
-   **Slugification:** Converts arbitrary text (Artist names, Albums) into URL-safe slugs.
-   **Indexing:** Creates lookup maps for artists and albums.
-   **Chunking:** Partitions data into small JSON files to ensure the web app remains performant.
-   **Sanitization:** Handles missing values, type conversions (`safe_int`, `safe_str`), and duration formatting.

**Usage:**
```bash
python3 scripts/build_web_data.py
```

---

## 📚 The Compiler Agent
**Script:** `archive/compile_itunes_exports.py`

The archivist. Its job is to ingest the chaotic collection of disjointed export files and unify them.

**Capabilities:**
-   **Discovery:** Recursively finds `Library.export*` and `.txt` files across nested directories.
-   **Pattern Recognition:** Automatically detects header rows and data start points in non-standard text files.
-   **Unification:** Merges hundreds of files into a single `compiled_itunes_library.csv`.
-   **Validation:** Filters out system files and ensures data integrity during the merge.

**Usage:**
```bash
python3 archive/compile_itunes_exports.py
```

---

## 🔍 The Extractor Agents
**Scripts:**
-   `scripts/extract_tracks.py`
-   `scripts/extract_albums.py`
-   `scripts/extract_artists.py`

Specialized workers focused on isolating specific entities from the dataset.

**Capabilities:**
-   **Focused Extraction:** Each agent focuses on a single entity type (Tracks, Albums, or Artists).
-   **JSON Serialization:** Outputs clean, formatted JSON lists for external use or simple analysis.

**Usage:**
```bash
python3 scripts/extract_tracks.py
```
