package main

import (
	"fmt"
	"os"
)

func main() {
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
	fmt.Println("  build-web-data    Build optimized web data to web-data/")
}
