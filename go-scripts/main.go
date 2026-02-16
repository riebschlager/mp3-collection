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
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage: mp3-scripts <command>")
	fmt.Println("Commands:")
	fmt.Println("  extract-tracks    Extract tracks to data/tracks.json")
	fmt.Println("  extract-artists   Extract artists to data/artists.json")
	fmt.Println("  extract-albums    Extract albums to data/albums.json")
	fmt.Println("  fetch-lastfm      Fetch recent scrobbles from Last.fm API")
	fmt.Println("  merge-listening   Merge Last.fm + Spotify history with dedupe")
	fmt.Println("  process-lastfm    Process merged listening history to data/playcounts.json")
	fmt.Println("  fetch-images      Fetch artist/album image metadata from Last.fm")
	fmt.Println("  build-timeline    Build timeline data from merged history to web-data/timeline.json")
	fmt.Println("  build-web-data    Build optimized web data to web-data/")
}
