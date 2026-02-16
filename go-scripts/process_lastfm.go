package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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
	Artist    string `json:"artist"`
	Track     string `json:"track"`
	PlayCount int    `json:"playCount"`
	FirstPlay int64  `json:"firstPlay"`
	LastPlay  int64  `json:"lastPlay"`
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
	fmt.Println("Processing listening history into playcounts...")

	history, err := loadListeningHistoryOrBuild()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading listening history: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Found %d total listening events\n", len(history.Events))
	if len(history.SourceCounts) > 0 {
		fmt.Println("Source breakdown:")
		sourceNames := make([]string, 0, len(history.SourceCounts))
		for source := range history.SourceCounts {
			sourceNames = append(sourceNames, source)
		}
		sort.Strings(sourceNames)
		for _, source := range sourceNames {
			fmt.Printf("  - %s: %d\n", source, history.SourceCounts[source])
		}
	}
	fmt.Println()

	// Aggregate playcounts
	playCounts := make(map[TrackKey]*PlayCountEntry)
	totalCountedEvents := 0

	for _, event := range history.Events {
		// Skip empty tracks or artists
		if strings.TrimSpace(event.Track) == "" || strings.TrimSpace(event.Artist) == "" {
			continue
		}

		// Create normalized key for matching
		normalizedArtist := NormalizeForMatching(event.Artist)
		normalizedTrack := NormalizeForMatching(event.Track)

		if normalizedArtist == "" || normalizedTrack == "" {
			continue
		}
		totalCountedEvents++

		key := TrackKey{
			Artist: normalizedArtist,
			Track:  normalizedTrack,
		}

		if entry, exists := playCounts[key]; exists {
			entry.PlayCount++
			if event.Date < entry.FirstPlay {
				entry.FirstPlay = event.Date
			}
			if event.Date > entry.LastPlay {
				entry.LastPlay = event.Date
			}
		} else {
			// Use the original (non-normalized) names for display
			playCounts[key] = &PlayCountEntry{
				Artist:    event.Artist,
				Track:     event.Track,
				PlayCount: 1,
				FirstPlay: event.Date,
				LastPlay:  event.Date,
			}
		}
	}

	// Convert map to slice
	var playCountsList []PlayCountEntry
	for _, entry := range playCounts {
		playCountsList = append(playCountsList, *entry)
	}

	outputData := PlayCountData{
		TotalScrobbles: totalCountedEvents,
		UniqueTracks:   len(playCountsList),
		PlayCounts:     playCountsList,
	}

	// Write output to both data/ and web-data/
	outputPaths := []string{
		DataPath("playcounts.json"),
		WebDataPath("playcounts.json"),
	}

	for _, path := range outputPaths {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating output directory for %s: %v\n", path, err)
			os.Exit(1)
		}

		file, err := os.Create(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating output file %s: %v\n", path, err)
			os.Exit(1)
		}

		encoder := json.NewEncoder(file)
		encoder.SetIndent("", "  ")
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(outputData); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing JSON to %s: %v\n", path, err)
			file.Close()
			os.Exit(1)
		}
		file.Close()
		fmt.Printf("Output written to: %s\n", path)
	}
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
