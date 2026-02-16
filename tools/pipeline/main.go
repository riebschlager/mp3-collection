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
	case "build-web-data":
		runBuildWebData()
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
	fmt.Println("  process-lastfm    Process merged listening history to data/derived/core/playcounts.json")
	fmt.Println("  fetch-images      Fetch artist/album image metadata from Last.fm")
	fmt.Println("  build-timeline    Build timeline data from merged history to data/derived/web/timeline.json")
	fmt.Println("  build-web-data    Build optimized web data to data/derived/web/")
	fmt.Println("  doctor            Validate path config, required inputs, and output directories")
}
