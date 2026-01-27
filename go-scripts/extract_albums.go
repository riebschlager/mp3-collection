package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type AlbumEntry struct {
	Album   string   `json:"album"`
	Artists []string `json:"artists"`
}

type AlbumsOutput struct {
	TotalAlbums int          `json:"total_albums"`
	Albums      []AlbumEntry `json:"albums"`
}

func runExtractAlbums() {
	csvPath := filepath.Join("..", "archive", "compiled_itunes_library.csv")
	outPath := filepath.Join("..", "data", "albums.json")

	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output directory: %v\n", err)
		os.Exit(1)
	}


rows, err := ReadCSV(csvPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading CSV: %v\n", err)
		os.Exit(1)
	}

	// Map album -> set of artists
	albumsMap := make(map[string]map[string]bool)

	for _, row := range rows {
		album := SanitizeAlbumName(row["Album"])
		artist := SanitizeArtistName(row["Artist"])

		if album != "" && IsValidName(album) {
			if _, ok := albumsMap[album]; !ok {
				albumsMap[album] = make(map[string]bool)
			}
			if artist != "" {
				albumsMap[album][artist] = true
			}
		}
	}

	// Convert to list
	var albumList []AlbumEntry
	for album, artistSet := range albumsMap {
		var artists []string
		for art := range artistSet {
			artists = append(artists, art)
		}
		sort.Strings(artists)
		albumList = append(albumList, AlbumEntry{
			Album:   album,
			Artists: artists,
		})
	}

	// Sort albums by name
	sort.Slice(albumList, func(i, j int) bool {
		return albumList[i].Album < albumList[j].Album
	})

	outputData := AlbumsOutput{
		TotalAlbums: len(albumList),
		Albums:      albumList,
	}

	file, err := os.Create(outPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output file: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(outputData); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing JSON: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Wrote %d unique albums to %s\n", len(albumList), outPath)
	fmt.Printf("\nFound %d unique albums\n", len(albumList))
	fmt.Println("\nFirst 10 albums:")
	
	for i, entry := range albumList {
		if i >= 10 {
			break
		}
		
		artistsStr := "Unknown Artist"
		if len(entry.Artists) > 0 {
			artistsStr = strings.Join(entry.Artists, ", ")
		}
		
		fmt.Printf("  - %s — %s\n", entry.Album, artistsStr)
	}
}
