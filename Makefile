SHELL := /bin/bash
LASTFM_IMAGE_SCOPE ?= all

.PHONY: help pipeline compile extract listening timeline anniversary-cache artist-race transitions transition-cache era-similarity-cache wrapped-stories wrapped-month-stories web-data images doctor validate web-dev web-build web-preview mcp clean-bins

help:
	@echo "Targets:"
	@echo "  pipeline    Run full Go data pipeline (compile -> web data + images, scope=$(LASTFM_IMAGE_SCOPE))"
	@echo "  compile     Compile raw iTunes exports into one CSV"
	@echo "  extract     Build tracks/albums/artists JSON artifacts"
	@echo "  listening   Refresh Last.fm + Spotify merged listening data"
	@echo "  timeline    Rebuild timeline JSON"
	@echo "  anniversary-cache Rebuild day/week anniversaries JSON"
	@echo "  artist-race Rebuild weekly/monthly artist race variants"
	@echo "  transitions Rebuild listening transition graph JSON"
	@echo "  transition-cache Rebuild MCP-backed transition query cache JSON"
	@echo "  era-similarity-cache Rebuild MCP-backed era similarity matrix JSON"
	@echo "  wrapped-stories Rebuild wrapped story JSON via MCP year-story"
	@echo "  wrapped-month-stories Rebuild wrapped month story JSON via MCP month-story"
	@echo "  web-data    Rebuild web-data JSON/chunks/indexes"
	@echo "  images      Refresh merged listening (Last.fm+Spotify), rebuild web-data, then refresh image metadata (scope=$(LASTFM_IMAGE_SCOPE))"
	@echo "  doctor      Validate ETL path config and required inputs"
	@echo "  validate    Run full repo validation (doctor + Go builds + web build)"
	@echo "  web-dev     Start Astro dev server"
	@echo "  web-build   Build Astro site for production"
	@echo "  web-preview Preview built Astro site"
	@echo "  mcp         Start local MCP server"
	@echo "  clean-bins  Remove generated local Go binaries"

pipeline:
	cd tools/pipeline && LASTFM_IMAGE_SCOPE=$(LASTFM_IMAGE_SCOPE) ./run_all.sh

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

anniversary-cache:
	cd tools/pipeline && go run . build-anniversary-cache

artist-race:
	cd tools/pipeline && go run . build-artist-race

transitions:
	cd tools/pipeline && go run . build-transition-graph

transition-cache:
	cd tools/pipeline && go run . build-transition-query-cache

era-similarity-cache:
	cd tools/pipeline && go run . build-era-similarity-cache

wrapped-stories:
	cd tools/pipeline && go run . build-wrapped-stories

wrapped-month-stories:
	cd tools/pipeline && go run . build-wrapped-month-stories

web-data:
	cd tools/pipeline && go run . build-web-data

images:
	$(MAKE) listening
	cd tools/pipeline && go run . build-web-data
	cd tools/pipeline && LASTFM_IMAGE_SCOPE=$(LASTFM_IMAGE_SCOPE) go run . fetch-images

doctor:
	GOCACHE="$${GOCACHE:-$(CURDIR)/.cache/go-build}" go -C backend run ./cmd/etl doctor

validate:
	mkdir -p .cache/go-build
	LASTFM_API_KEY="$${LASTFM_API_KEY:-ci-placeholder}" GOCACHE="$${GOCACHE:-$(CURDIR)/.cache/go-build}" $(MAKE) doctor
	GOCACHE="$${GOCACHE:-$(CURDIR)/.cache/go-build}" go -C backend build ./...
	GOCACHE="$${GOCACHE:-$(CURDIR)/.cache/go-build}" go -C apps/mcp-server build ./...
	npm -C apps/web ci
	npm -C apps/web run build

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
