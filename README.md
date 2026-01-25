# MP3 Collection Manager

A comprehensive system for archiving, processing, and exploring a massive music collection. This project ingests raw iTunes library export files, normalizes the data, and presents it through a modern, searchable web interface.

## 🚀 Overview

The system operates in two main stages:
1.  **Data Pipeline (Python):** Consolidates scattered iTunes export files into a unified dataset, cleanses metadata, and generates optimized JSON chunks.
2.  **Web Interface (Astro):** A static-site generated (SSG) web application that provides a fast, responsive UI to browse artists, albums, and tracks.

## 📂 Project Structure

-   `scripts/`: Python "agents" that handle data extraction and processing.
-   `mp3-collection-web/`: The frontend application built with [Astro](https://astro.build) and [Tailwind CSS](https://tailwindcss.com).
-   `archive/`: Storage for raw iTunes export files and the compilation script.
-   `data/`: Intermediate JSON data (artists, albums, tracks).
-   `web-data/`: Optimized, chunked data ready for the web application.

## 🛠️ Prerequisites

-   **Python 3.8+**
-   **Node.js 18+** (for the web interface)

## 🏁 Quick Start

### 1. Data Processing Pipeline

The pipeline transforms raw exports into web-ready data.

```bash
# 1. Compile raw exports into a single CSV (if needed)
# Navigate to archive/ and run the compiler (check script for specific args)
python3 archive/compile_itunes_exports.py

# 2. Build the optimized web data
# This reads the compiled CSV and generates JSON chunks in web-data/
python3 scripts/build_web_data.py

# (Optional) specific extractions
python3 scripts/extract_tracks.py
```

### 2. Web Application

Initialize and run the frontend.

```bash
cd mp3-collection-web

# Install dependencies
npm install

# Start the development server
npm run dev
```

Visit `http://localhost:4321` to browse the collection.

## � Deployment to GitHub Pages

The project is configured for automatic deployment to GitHub Pages. Deployment happens automatically when you push changes to the `main` or `master` branch.

### Configuration

The deployment is configured in:
- **Astro config:** `mp3-collection-web/astro.config.mjs` (sets `site` and `base` for GitHub Pages)
- **GitHub Actions:** `.github/workflows/deploy.yml` (automates the build and deployment)

### Accessing the Deployed Site

Once deployed, the site will be available at:
```
https://riebschlager.github.io/mp3-collection
```

### Manual Deployment

To manually trigger a deployment:
1. Go to the [Actions](https://github.com/riebschlager/mp3-collection/actions) tab in the repository
2. Select the "Deploy to GitHub Pages" workflow
3. Click "Run workflow"

### Building for Production Locally

To test a production build locally:

```bash
cd mp3-collection-web
npm run build
npm run preview
```

## �📦 Data Architecture

The system uses a "static database" approach:
-   **Indexes:** `artists-index.json`, `albums-index.json` provide O(1) lookups for routing.
-   **Chunks:** Data is split into manageable JSON chunks in `web-data/chunks/` to avoid loading the entire library size in the browser.
-   **Search:** Powered by [Pagefind](https://pagefind.app/) for static, full-text search capabilities.

## 🤝 Contributing

1.  Ensure all Python scripts use type hinting where possible.
2.  Web components should be kept small and functional.
3.  Run `npm run build` to verify the production build before committing.
