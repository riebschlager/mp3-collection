SHELL := /bin/bash

.PHONY: help pipeline compile extract listening timeline web-data images doctor web-dev web-build web-preview mcp clean-bins

help:
	@echo "Targets:"
	@echo "  pipeline    Run full Go data pipeline (compile -> web data + images)"
	@echo "  compile     Compile raw iTunes exports into one CSV"
	@echo "  extract     Build tracks/albums/artists JSON artifacts"
	@echo "  listening   Refresh Last.fm + Spotify merged listening data"
	@echo "  timeline    Rebuild timeline JSON"
	@echo "  web-data    Rebuild web-data JSON/chunks/indexes"
	@echo "  images      Refresh Last.fm artist/album image metadata"
	@echo "  doctor      Validate ETL path config and required inputs"
	@echo "  web-dev     Start Astro dev server"
	@echo "  web-build   Build Astro site for production"
	@echo "  web-preview Preview built Astro site"
	@echo "  mcp         Start local MCP server"
	@echo "  clean-bins  Remove generated local Go binaries"

pipeline:
	cd tools/pipeline && ./run_all.sh

compile:
	cd tools/pipeline && go run . compile-itunes-exports

extract:
	cd tools/pipeline && go run . extract-tracks
	cd tools/pipeline && go run . extract-albums
	cd tools/pipeline && go run . extract-artists

listening:
	cd tools/pipeline && go run . fetch-lastfm
	cd tools/pipeline && go run . merge-listening
	cd tools/pipeline && go run . process-lastfm

timeline:
	cd tools/pipeline && go run . build-timeline

web-data:
	cd tools/pipeline && go run . build-web-data

images:
	cd tools/pipeline && go run . fetch-images

doctor:
	cd tools/pipeline && go run . doctor

web-dev:
	cd apps/web && npm install && npm run dev

web-build:
	cd apps/web && npm ci && npm run build

web-preview:
	cd apps/web && npm run preview

mcp:
	./apps/mcp-server/run-mcp.sh

clean-bins:
	rm -f tools/pipeline/mp3-scripts apps/mcp-server/mcp-server
