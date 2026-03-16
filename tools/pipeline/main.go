package main

import (
	"fmt"
	"os"
)

func main() {
	// Load .env from root directory if it exists
	LoadEnv()

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "compile-itunes-exports":
		runCompileITunesExports()
	case "compile-exports":
		runCompileITunesExports()
	case "extract-tracks":
		runExtractTracks()
	case "extract-artists":
		runExtractArtists()
	case "extract-albums":
		runExtractAlbums()
	case "process-lastfm":
		runProcessLastFm()
	case "process-listening":
		runProcessLastFm()
	case "merge-listening":
		runMergeListening()
	case "fetch-lastfm":
		runFetchLastFm()
	case "fetch-images":
		runFetchMetadata()
	case "fetch-metadata":
		runFetchMetadata()
	case "build-timeline":
		runBuildTimeline()
	case "build-artist-race":
		runBuildArtistRace()
	case "build-wrapped-stories":
		runBuildWrappedStories()
	case "build-wrapped-month-stories":
		runBuildWrappedMonthStories()
	case "build-web-data":
		runBuildWebData()
	case "build-transition-graph":
		runBuildTransitionGraph()
	case "build-transition-query-cache":
		runBuildTransitionQueryCache()
	case "build-era-similarity-cache":
		runBuildEraSimilarityCache()
	case "build-streaks-cache":
		runBuildStreaksCache()
	case "doctor":
		runDoctor()
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage: mp3-scripts <command>")
	fmt.Println("Commands:")
	fmt.Println("  compile-itunes-exports [inputDir] Compile raw exports to data/derived/compiled/compiled_itunes_library.csv")
	fmt.Println("  compile-exports  Alias for compile-itunes-exports")
	fmt.Println("  extract-tracks    Extract tracks to data/derived/core/tracks.json")
	fmt.Println("  extract-artists   Extract artists to data/derived/core/artists.json")
	fmt.Println("  extract-albums    Extract albums to data/derived/core/albums.json")
	fmt.Println("  fetch-lastfm      Fetch recent scrobbles from Last.fm API")
	fmt.Println("  merge-listening   Merge Last.fm + Spotify history with dedupe")
	fmt.Println("  process-lastfm    Process merged Last.fm + Spotify listening history to data/derived/core/playcounts.json")
	fmt.Println("  process-listening Alias for process-lastfm")
	fmt.Println("  fetch-images      Fetch artist/album image metadata from Last.fm")
	fmt.Println("  build-timeline    Build timeline data from merged history to data/derived/web/timeline.json")
	fmt.Println("  build-artist-race Build cumulative monthly artist race data to data/derived/web/artist-race.json")
	fmt.Println("  build-wrapped-stories Build wrapped story artifact from MCP year-story tool to data/derived/web/wrapped-stories.json")
	fmt.Println("  build-wrapped-month-stories Build wrapped month story artifact from MCP month-story tool to data/derived/web/wrapped-month-stories.json")
	fmt.Println("  build-web-data    Build optimized web data to data/derived/web/")
	fmt.Println("  build-transition-graph Build listening transition graph to data/derived/core/transition-graph.json and data/derived/web/transition-graph.json")
	fmt.Println("  build-transition-query-cache Build MCP-backed per-year transition slices to data/derived/web/transition-query-cache.json")
	fmt.Println("  build-era-similarity-cache Build MCP-backed era similarity matrix/cache to data/derived/web/era-similarity-cache.json")
	fmt.Println("  build-streaks-cache Build MCP-backed streaks and bursts cache to data/derived/web/streaks-cache.json")
	fmt.Println("  doctor            Validate path config, required inputs, and output directories")
}
