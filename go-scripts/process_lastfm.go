package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type LastFmScrobble struct {
	Track   string `json:"track"`
	Artist  string `json:"artist"`
	Album   string `json:"album"`
	AlbumID string `json:"albumId"`
	Date    int64  `json:"date"`
}

type LastFmData struct {
	Username  string           `json:"username"`
	Scrobbles []LastFmScrobble `json:"scrobbles"`
}

type TrackKey struct {
	Artist string
	Track  string
}

type PlayCountEntry struct {
	Artist     string `json:"artist"`
	Track      string `json:"track"`
	PlayCount  int    `json:"playCount"`
	FirstPlay  int64  `json:"firstPlay"`
	LastPlay   int64  `json:"lastPlay"`
}

type PlayCountData struct {
	TotalScrobbles int              `json:"totalScrobbles"`
	UniqueTracks   int              `json:"uniqueTracks"`
	PlayCounts     []PlayCountEntry `json:"playCounts"`
}

// NormalizeForMatching normalizes a string for matching (lowercase, remove special chars, trim spaces)
func NormalizeForMatching(s string) string {
	// Convert to lowercase
	s = strings.ToLower(s)

	// Remove common variations
	s = strings.ReplaceAll(s, " & ", " and ")
	s = strings.ReplaceAll(s, "&", "and")

	// Remove featuring variations
	featuringPatterns := []string{
		" feat. ", " feat ", " ft. ", " ft ", " featuring ",
	}
	for _, pattern := range featuringPatterns {
		if idx := strings.Index(s, pattern); idx != -1 {
			s = s[:idx]
		}
	}

	// Remove anything in parentheses or brackets (remixes, versions, etc.)
	regexParens := regexp.MustCompile(`\([^)]*\)`)
	s = regexParens.ReplaceAllString(s, "")

	regexBrackets := regexp.MustCompile(`\[[^\]]*\]`)
	s = regexBrackets.ReplaceAllString(s, "")

	// Remove non-alphanumeric characters except spaces
	regexNonAlnum := regexp.MustCompile(`[^a-z0-9\s]`)
	s = regexNonAlnum.ReplaceAllString(s, "")

	// Collapse multiple spaces into one
	regexSpaces := regexp.MustCompile(`\s+`)
	s = regexSpaces.ReplaceAllString(s, " ")

	return strings.TrimSpace(s)
}

func runProcessLastFm() {
	// Paths
	lastfmPath := filepath.Join(ProjectRoot, "lastfm", "lastfmstats-riebschlager.json")
	outputPath := filepath.Join(ProjectRoot, "data", "playcounts.json")

	fmt.Println("Processing Last.fm scrobbles...")
	fmt.Printf("Reading from: %s\n", lastfmPath)

	// Read Last.fm JSON
	data, err := os.ReadFile(lastfmPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading Last.fm file: %v\n", err)
		os.Exit(1)
	}

	var lastfmData LastFmData
	if err := json.Unmarshal(data, &lastfmData); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing Last.fm JSON: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Found %d scrobbles for user: %s\n\n", len(lastfmData.Scrobbles), lastfmData.Username)

	// Aggregate playcounts
	playCounts := make(map[TrackKey]*PlayCountEntry)

	for _, scrobble := range lastfmData.Scrobbles {
		// Skip empty tracks or artists
		if strings.TrimSpace(scrobble.Track) == "" || strings.TrimSpace(scrobble.Artist) == "" {
			continue
		}

		// Create normalized key for matching
		normalizedArtist := NormalizeForMatching(scrobble.Artist)
		normalizedTrack := NormalizeForMatching(scrobble.Track)

		if normalizedArtist == "" || normalizedTrack == "" {
			continue
		}

		key := TrackKey{
			Artist: normalizedArtist,
			Track:  normalizedTrack,
		}

		if entry, exists := playCounts[key]; exists {
			entry.PlayCount++
			if scrobble.Date < entry.FirstPlay {
				entry.FirstPlay = scrobble.Date
			}
			if scrobble.Date > entry.LastPlay {
				entry.LastPlay = scrobble.Date
			}
		} else {
			// Use the original (non-normalized) names for display
			playCounts[key] = &PlayCountEntry{
				Artist:    scrobble.Artist,
				Track:     scrobble.Track,
				PlayCount: 1,
				FirstPlay: scrobble.Date,
				LastPlay:  scrobble.Date,
			}
		}
	}

	// Convert map to slice
	var playCountsList []PlayCountEntry
	for _, entry := range playCounts {
		playCountsList = append(playCountsList, *entry)
	}

	outputData := PlayCountData{
		TotalScrobbles: len(lastfmData.Scrobbles),
		UniqueTracks:   len(playCountsList),
		PlayCounts:     playCountsList,
	}

	// Write output
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	file, err := os.Create(outputPath)
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

	fmt.Println("Processing complete!")
	fmt.Printf("Total scrobbles:  %d\n", outputData.TotalScrobbles)
	fmt.Printf("Unique tracks:    %d\n", outputData.UniqueTracks)
	fmt.Printf("Output written to: %s\n", outputPath)

	// Show top 10 most played tracks
	fmt.Println("\nTop 10 most played tracks:")

	// Sort by playcount
	sortedTracks := make([]PlayCountEntry, len(playCountsList))
	copy(sortedTracks, playCountsList)

	// Simple bubble sort for top 10 (good enough for this use case)
	for i := 0; i < len(sortedTracks); i++ {
		for j := i + 1; j < len(sortedTracks); j++ {
			if sortedTracks[j].PlayCount > sortedTracks[i].PlayCount {
				sortedTracks[i], sortedTracks[j] = sortedTracks[j], sortedTracks[i]
			}
		}
		if i >= 9 {
			break // Only need top 10
		}
	}

	for i := 0; i < 10 && i < len(sortedTracks); i++ {
		fmt.Printf("%2d. %s - %s (%d plays)\n",
			i+1,
			sortedTracks[i].Artist,
			sortedTracks[i].Track,
			sortedTracks[i].PlayCount)
	}
}
