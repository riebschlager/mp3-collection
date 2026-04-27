# MP3 Collection

Historical iTunes archive + listening-history analytics + dynamic web explorer.

This repo has been modernized from a static-JSON pipeline to a robust **Go + SQLite** architecture.

## Architecture

- **Backend (`backend/`)**: A unified Go service providing:
  - **ETL CLI (`cmd/etl`)**: High-performance data ingestion for iTunes, Last.fm, and Spotify.
  - **REST API (`cmd/server`)**: A dynamic API serving music data and analytics.
  - **MCP Server (`cmd/mcp`)**: A Model Context Protocol server for AI-powered music intelligence.
- **Frontend (`apps/web`)**: An Astro-based web application using **Server-Side Rendering (SSR)** to fetch live data from the backend.
- **Database**: A central SQLite database (`data/mp3_collection.db`) that unifies tracks, artists, albums, and listening history.

## Repository Layout

- `backend/`:
  - `cmd/etl/`: CLI tools for database initialization and data ingestion.
  - `cmd/server/`: REST API server (port 8080).
  - `cmd/mcp/`: MCP Stdio server.
  - `internal/`: Shared logic for database access, API routing, and MCP tools.
- `apps/web/`: Astro frontend.
- `data/`:
  - `mp3_collection.db`: The unified SQLite database.
  - `inputs/`: Raw data sources (iTunes CSV, Last.fm JSON, Spotify JSON).

## Prerequisites

- Go 1.22+
- Node.js 20+
- SQLite3

## Getting Started

### 1. Database Setup & Ingestion

Initialize the database and import your music data:

```bash
cd backend
# Initialize schema
go run ./cmd/etl init
# Ingest iTunes library
go run ./cmd/etl ingest-itunes
# Fetch recent scrobbles
go run ./cmd/etl fetch-lastfm
# Ingest Spotify history
go run ./cmd/etl ingest-spotify
# Pre-compute analytics (Transitions & Eras)
go run ./cmd/etl compute-transitions
go run ./cmd/etl compute-eras
```

### 2. Run the Backend

Start the REST API server:

```bash
cd backend
go run ./cmd/server
```
The API will be available at `http://localhost:8080/api/v1/`.

### 3. Run the Frontend

Start the Astro dev server:

```bash
cd apps/web
npm install
npm run dev
```
The web app will be available at `http://localhost:4321/mp3-collection`.

### 4. Run the MCP Server

To use the AI tools, configure your MCP client (like Claude Desktop) to run:

```bash
cd backend
go run ./cmd/mcp
```

## Analytics Capabilities

The new architecture enables advanced on-the-fly analytics:
- **Transition Flow Atlas**: Session-aware directional listening patterns.
- **Era Similarity Index**: Jaccard-based taste proximity between years.
- **Dynamic Stats**: Instant play counts and top charts across 20 years of data.
- **AI Intelligence**: Direct SQL-backed tools for exploring music trends.
