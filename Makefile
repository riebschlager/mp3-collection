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
	cd go-scripts && ./run_all.sh

compile:
	cd go-scripts && go run . compile-itunes-exports

extract:
	cd go-scripts && go run . extract-tracks
	cd go-scripts && go run . extract-albums
	cd go-scripts && go run . extract-artists

listening:
	cd go-scripts && go run . fetch-lastfm
	cd go-scripts && go run . merge-listening
	cd go-scripts && go run . process-lastfm

timeline:
	cd go-scripts && go run . build-timeline

web-data:
	cd go-scripts && go run . build-web-data

images:
	cd go-scripts && go run . fetch-images

doctor:
	cd go-scripts && go run . doctor

web-dev:
	cd mp3-collection-web && npm install && npm run dev

web-build:
	cd mp3-collection-web && npm ci && npm run build

web-preview:
	cd mp3-collection-web && npm run preview

mcp:
	./mcp-server/run-mcp.sh

clean-bins:
	rm -f go-scripts/mp3-scripts mcp-server/mcp-server
