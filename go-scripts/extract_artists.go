package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type ArtistEntry struct {
	Artist string   `json:"artist"`
	Albums []string `json:"albums"`
}

type ArtistsOutput struct {
	TotalArtists int           `json:"total_artists"`
	Artists      []ArtistEntry `json:"artists"`
}

func runExtractArtists() {
	csvPath := filepath.Join("..", "archive", "compiled_itunes_library.csv")
	outPath := filepath.Join("..", "data", "artists.json")

	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	rows, err := ReadCSV(csvPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading CSV: %v\n", err)
		os.Exit(1)
	}

	// Map artist -> set of albums
	artistsMap := make(map[string]map[string]bool)

	for _, row := range rows {
		artist := SanitizeArtistName(row["Artist"])
		album := strings.TrimSpace(row["Album"])

		if artist != "" && IsValidName(artist) {
			if _, ok := artistsMap[artist]; !ok {
				artistsMap[artist] = make(map[string]bool)
			}
			if album != "" {
				artistsMap[artist][album] = true
			}
		}
	}

	// Convert to list
	var artistList []ArtistEntry
	for artist, albumSet := range artistsMap {
		var albums []string
		for alb := range albumSet {
			albums = append(albums, alb)
		}
		sort.Strings(albums)
		artistList = append(artistList, ArtistEntry{
			Artist: artist,
			Albums: albums,
		})
	}

	// Sort artists by name
	sort.Slice(artistList, func(i, j int) bool {
		return artistList[i].Artist < artistList[j].Artist
	})

	outputData := ArtistsOutput{
		TotalArtists: len(artistList),
		Artists:      artistList,
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

	fmt.Printf("Wrote %d unique artists to %s\n", len(artistList), outPath)
	fmt.Printf("\nFound %d unique artists\n", len(artistList))
	fmt.Println("\nFirst 10 artists:")
	
	for i, entry := range artistList {
		if i >= 10 {
			break
		}
		albumCount := len(entry.Albums)
		
		// Preview albums
		previewCount := 3
		if albumCount < 3 {
			previewCount = albumCount
		}
		albumsPreview := strings.Join(entry.Albums[:previewCount], ", ")
		if albumCount > 3 {
			albumsPreview += fmt.Sprintf(" ... (+%d more)", albumCount-3)
		}
		
		fmt.Printf("  - %s (%d albums)\n", entry.Artist, albumCount)
		fmt.Printf("    %s\n", albumsPreview)
	}
}
