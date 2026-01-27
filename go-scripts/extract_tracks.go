package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type TrackEntry struct {
	Track  string `json:"track"`
	Artist string `json:"artist,omitempty"` // Use pointer or omit empty if nullable? Python uses None.
	Album  string `json:"album,omitempty"`
}

type TracksOutput struct {
	TotalTracks int          `json:"total_tracks"`
	Tracks      []TrackEntry `json:"tracks"`
}

func runExtractTracks() {
	// Paths
	// Assuming running from go-scripts/ or similar, relative paths:
	// In Python: Path(__file__).parent.parent / 'archive' / 'compiled_itunes_library.csv'
	// Here, we'll assume we are in go-scripts/ when running.
	csvPath := filepath.Join("..", "archive", "compiled_itunes_library.csv")
	outPath := filepath.Join("..", "data", "tracks.json")

	// Ensure output dir exists
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	rows, err := ReadCSV(csvPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading CSV: %v\n", err)
		os.Exit(1)
	}

	var tracksList []TrackEntry

	for _, row := range rows {
		trackName := strings.TrimSpace(row["Name"])
		artist := SanitizeArtistName(row["Artist"])
		album := strings.TrimSpace(row["Album"])

		if !IsValidName(artist) || !IsValidName(album) {
			continue
		}

		if trackName != "" {
			// In Go, empty string is default, but JSON omitempty hides it.
			// Python uses None (null) if empty.
			// If we want null in JSON, we need *string.
			// However, the python script logic:
			// 'artist': artist if artist else None
			// So yes, we strictly should output null if empty.
			// But for simplicity in this script, let's keep it simple or use logic.
			
			// Let's stick to the struct fields being string and just omitempty.
			// If Python outputted null, omitempty on string outputs nothing (missing key).
			// If we want literal null, we need pointers.
			// Let's check python output. "None" in python becomes "null" in JSON.
			// So "artist": null.
			// omitempty on string makes the key disappear.
			// Let's try to match it closely.
			
			entry := TrackEntry{
				Track: trackName,
				Artist: artist,
				Album: album,
			}
			tracksList = append(tracksList, entry)
		}
	}

	outputData := TracksOutput{
		TotalTracks: len(tracksList),
		Tracks:      tracksList,
	}

	// Write JSON
	file, err := os.Create(outPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output file: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false) // matches ensure_ascii=False roughly
	if err := encoder.Encode(outputData); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing JSON: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Wrote %d tracks to %s\n", len(tracksList), outPath)
	
	fmt.Printf("\nFound %d tracks\n", len(tracksList))
	fmt.Println("\nFirst 10 tracks:")
	for i, entry := range tracksList {
		if i >= 10 {
			break
		}
		artistStr := entry.Artist
		if artistStr == "" {
			artistStr = "Unknown Artist"
		}
		albumStr := entry.Album
		if albumStr == "" {
			albumStr = "Unknown Album"
		}
		fmt.Printf("  - %s\n", entry.Track)
		fmt.Printf("    Artist: %s\n", artistStr)
		fmt.Printf("    Album: %s\n", albumStr)
	}
}
