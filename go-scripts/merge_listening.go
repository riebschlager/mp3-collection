package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultSpotifyMinMsPlayed      int64 = 30000
	defaultCrossSourceDedupeWindow int64 = 120000
)

type ListeningEvent struct {
	Track           string `json:"track"`
	Artist          string `json:"artist"`
	Album           string `json:"album"`
	Date            int64  `json:"date"`
	Source          string `json:"source"`
	SpotifyTrackURI string `json:"spotifyTrackUri,omitempty"`
	MsPlayed        int64  `json:"msPlayed,omitempty"`
}

type ListeningHistoryData struct {
	GeneratedAt  string           `json:"generatedAt"`
	TotalEvents  int              `json:"totalEvents"`
	SourceCounts map[string]int   `json:"sourceCounts"`
	Events       []ListeningEvent `json:"events"`
}

type ListeningMergeConfig struct {
	SpotifyMinMsPlayed      int64 `json:"spotifyMinMsPlayed"`
	CrossSourceDedupeWindow int64 `json:"crossSourceDedupeWindowMs"`
}

type ListeningMergeReport struct {
	GeneratedAt string               `json:"generatedAt"`
	Config      ListeningMergeConfig `json:"config"`
	LastFm      struct {
		InputEvents int `json:"inputEvents"`
		ValidEvents int `json:"validEvents"`
	} `json:"lastfm"`
	Spotify struct {
		InputFiles                int `json:"inputFiles"`
		RawEvents                 int `json:"rawEvents"`
		FilteredMissingMetadata   int `json:"filteredMissingMetadata"`
		FilteredShortPlay         int `json:"filteredShortPlay"`
		FilteredInvalidTimestamp  int `json:"filteredInvalidTimestamp"`
		QualifiedEvents           int `json:"qualifiedEvents"`
		DedupedExactWithinSpotify int `json:"dedupedExactWithinSpotify"`
		DedupedAgainstLastFm      int `json:"dedupedAgainstLastfm"`
		AddedEvents               int `json:"addedEvents"`
	} `json:"spotify"`
	Output struct {
		TotalEvents  int            `json:"totalEvents"`
		SourceCounts map[string]int `json:"sourceCounts"`
		FirstEvent   int64          `json:"firstEvent"`
		LastEvent    int64          `json:"lastEvent"`
		Years        map[string]int `json:"years"`
	} `json:"output"`
}

type spotifyAudioRow struct {
	TS       string `json:"ts"`
	MsPlayed int64  `json:"ms_played"`
	Track    string `json:"master_metadata_track_name"`
	Artist   string `json:"master_metadata_album_artist_name"`
	Album    string `json:"master_metadata_album_album_name"`
	TrackURI string `json:"spotify_track_uri"`
}

func defaultListeningMergeConfig() ListeningMergeConfig {
	return ListeningMergeConfig{
		SpotifyMinMsPlayed:      readEnvInt64("SPOTIFY_MIN_MS_PLAYED", defaultSpotifyMinMsPlayed),
		CrossSourceDedupeWindow: readEnvInt64("SPOTIFY_LASTFM_DEDUPE_WINDOW_MS", defaultCrossSourceDedupeWindow),
	}
}

func readEnvInt64(name string, defaultValue int64) int64 {
	val := strings.TrimSpace(os.Getenv(name))
	if val == "" {
		return defaultValue
	}

	parsed, err := strconv.ParseInt(val, 10, 64)
	if err != nil || parsed < 0 {
		fmt.Printf("Warning: Invalid value for %s=%q, using default %d\n", name, val, defaultValue)
		return defaultValue
	}

	return parsed
}

func normalizedEventKey(artist, track string) string {
	normalizedArtist := NormalizeForMatching(artist)
	normalizedTrack := NormalizeForMatching(track)
	if normalizedArtist == "" || normalizedTrack == "" {
		return ""
	}
	return normalizedArtist + "|" + normalizedTrack
}

func hasNearbyTimestamp(sortedTimestamps []int64, timestamp, maxDistanceMs int64) bool {
	if len(sortedTimestamps) == 0 {
		return false
	}

	index := sort.Search(len(sortedTimestamps), func(i int) bool {
		return sortedTimestamps[i] >= timestamp
	})

	if index < len(sortedTimestamps) && absInt64(sortedTimestamps[index]-timestamp) <= maxDistanceMs {
		return true
	}
	if index > 0 && absInt64(sortedTimestamps[index-1]-timestamp) <= maxDistanceMs {
		return true
	}

	return false
}

func absInt64(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

func loadLastFmListeningEvents(lastfmPath string, report *ListeningMergeReport) ([]ListeningEvent, error) {
	data, err := os.ReadFile(lastfmPath)
	if err != nil {
		return nil, err
	}

	var lastfmData LastFmData
	if err := json.Unmarshal(data, &lastfmData); err != nil {
		return nil, err
	}

	report.LastFm.InputEvents = len(lastfmData.Scrobbles)

	events := make([]ListeningEvent, 0, len(lastfmData.Scrobbles))
	for _, scrobble := range lastfmData.Scrobbles {
		if strings.TrimSpace(scrobble.Track) == "" || strings.TrimSpace(scrobble.Artist) == "" {
			continue
		}
		events = append(events, ListeningEvent{
			Track:  scrobble.Track,
			Artist: scrobble.Artist,
			Album:  scrobble.Album,
			Date:   scrobble.Date,
			Source: "lastfm",
		})
	}

	report.LastFm.ValidEvents = len(events)
	return events, nil
}

func loadQualifiedSpotifyEvents(spotifyDir string, config ListeningMergeConfig, report *ListeningMergeReport) ([]ListeningEvent, error) {
	pattern := filepath.Join(spotifyDir, "Streaming_History_Audio_*.json")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	sort.Strings(files)

	report.Spotify.InputFiles = len(files)
	if len(files) == 0 {
		return []ListeningEvent{}, nil
	}

	events := make([]ListeningEvent, 0, 50000)

	for _, filePath := range files {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, err
		}

		var rows []spotifyAudioRow
		if err := json.Unmarshal(data, &rows); err != nil {
			return nil, fmt.Errorf("parse spotify file %s: %w", filePath, err)
		}

		report.Spotify.RawEvents += len(rows)

		for _, row := range rows {
			if strings.TrimSpace(row.Track) == "" || strings.TrimSpace(row.Artist) == "" {
				report.Spotify.FilteredMissingMetadata++
				continue
			}
			if row.MsPlayed < config.SpotifyMinMsPlayed {
				report.Spotify.FilteredShortPlay++
				continue
			}

			parsed, err := time.Parse(time.RFC3339, row.TS)
			if err != nil {
				report.Spotify.FilteredInvalidTimestamp++
				continue
			}

			events = append(events, ListeningEvent{
				Track:           row.Track,
				Artist:          row.Artist,
				Album:           row.Album,
				Date:            parsed.UnixMilli(),
				Source:          "spotify",
				SpotifyTrackURI: row.TrackURI,
				MsPlayed:        row.MsPlayed,
			})
		}
	}

	report.Spotify.QualifiedEvents = len(events)
	sort.Slice(events, func(i, j int) bool {
		return events[i].Date < events[j].Date
	})

	return events, nil
}

func dedupeSpotifyEventsAgainstLastFm(spotifyEvents, lastfmEvents []ListeningEvent, config ListeningMergeConfig, report *ListeningMergeReport) []ListeningEvent {
	lastfmTimesByTrack := make(map[string][]int64)
	for _, event := range lastfmEvents {
		key := normalizedEventKey(event.Artist, event.Track)
		if key == "" {
			continue
		}
		lastfmTimesByTrack[key] = append(lastfmTimesByTrack[key], event.Date)
	}
	for key := range lastfmTimesByTrack {
		sort.Slice(lastfmTimesByTrack[key], func(i, j int) bool {
			return lastfmTimesByTrack[key][i] < lastfmTimesByTrack[key][j]
		})
	}

	seenSpotifyExact := make(map[string]struct{}, len(spotifyEvents))
	deduped := make([]ListeningEvent, 0, len(spotifyEvents))

	for _, event := range spotifyEvents {
		key := normalizedEventKey(event.Artist, event.Track)
		if key == "" {
			continue
		}

		signature := key + "|" + strconv.FormatInt(event.Date, 10) + "|" + strconv.FormatInt(event.MsPlayed, 10) + "|" + event.SpotifyTrackURI
		if _, exists := seenSpotifyExact[signature]; exists {
			report.Spotify.DedupedExactWithinSpotify++
			continue
		}
		seenSpotifyExact[signature] = struct{}{}

		if hasNearbyTimestamp(lastfmTimesByTrack[key], event.Date, config.CrossSourceDedupeWindow) {
			report.Spotify.DedupedAgainstLastFm++
			continue
		}

		deduped = append(deduped, event)
	}

	report.Spotify.AddedEvents = len(deduped)
	return deduped
}

func buildListeningHistory(config ListeningMergeConfig) (*ListeningHistoryData, *ListeningMergeReport, error) {
	lastfmPath := LastFMStatsPath("")
	spotifyDir := Paths.SpotifyDir

	report := &ListeningMergeReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Config:      config,
	}

	lastfmEvents, err := loadLastFmListeningEvents(lastfmPath, report)
	if err != nil {
		return nil, nil, fmt.Errorf("load lastfm events: %w", err)
	}

	spotifyEvents, err := loadQualifiedSpotifyEvents(spotifyDir, config, report)
	if err != nil {
		return nil, nil, fmt.Errorf("load spotify events: %w", err)
	}

	spotifySupplement := dedupeSpotifyEventsAgainstLastFm(spotifyEvents, lastfmEvents, config, report)

	mergedEvents := make([]ListeningEvent, 0, len(lastfmEvents)+len(spotifySupplement))
	mergedEvents = append(mergedEvents, lastfmEvents...)
	mergedEvents = append(mergedEvents, spotifySupplement...)

	sort.Slice(mergedEvents, func(i, j int) bool {
		return mergedEvents[i].Date < mergedEvents[j].Date
	})

	sourceCounts := map[string]int{
		"lastfm":  0,
		"spotify": 0,
	}
	years := make(map[string]int)

	var firstEvent int64
	var lastEvent int64
	for i, event := range mergedEvents {
		sourceCounts[event.Source]++
		year := time.Unix(event.Date/1000, 0).Year()
		years[strconv.Itoa(year)]++

		if i == 0 || event.Date < firstEvent {
			firstEvent = event.Date
		}
		if i == 0 || event.Date > lastEvent {
			lastEvent = event.Date
		}
	}

	report.Output.TotalEvents = len(mergedEvents)
	report.Output.SourceCounts = sourceCounts
	report.Output.FirstEvent = firstEvent
	report.Output.LastEvent = lastEvent
	report.Output.Years = years

	history := &ListeningHistoryData{
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
		TotalEvents:  len(mergedEvents),
		SourceCounts: sourceCounts,
		Events:       mergedEvents,
	}

	return history, report, nil
}

func writeJSONData(path string, value interface{}) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}

func writeListeningHistoryArtifacts(history *ListeningHistoryData, report *ListeningMergeReport) error {
	historyPath := DataPath("listening-history.json")
	reportPaths := []string{
		DataPath("listening-merge-report.json"),
		WebDataPath("listening-merge-report.json"),
	}

	if err := os.MkdirAll(filepath.Dir(historyPath), 0755); err != nil {
		return err
	}

	if err := writeJSONData(historyPath, history); err != nil {
		return err
	}

	for _, path := range reportPaths {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		if err := writeJSONData(path, report); err != nil {
			return err
		}
	}

	return nil
}

func loadListeningHistoryOrBuild() (*ListeningHistoryData, error) {
	historyPath := DataPath("listening-history.json")

	shouldRebuild, err := listeningHistoryIsStale(historyPath)
	if err != nil {
		return nil, err
	}

	if !shouldRebuild {
		data, err := os.ReadFile(historyPath)
		if err == nil {
			var history ListeningHistoryData
			if err := json.Unmarshal(data, &history); err == nil && len(history.Events) > 0 {
				return &history, nil
			}
		}
	}

	config := defaultListeningMergeConfig()
	history, report, err := buildListeningHistory(config)
	if err != nil {
		return nil, err
	}

	if err := writeListeningHistoryArtifacts(history, report); err != nil {
		return nil, err
	}

	return history, nil
}

func listeningHistoryIsStale(historyPath string) (bool, error) {
	historyInfo, err := os.Stat(historyPath)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, err
	}

	historyMod := historyInfo.ModTime()
	lastfmPath := LastFMStatsPath("")
	lastfmInfo, err := os.Stat(lastfmPath)
	if err != nil {
		return false, err
	}
	if lastfmInfo.ModTime().After(historyMod) {
		return true, nil
	}

	spotifyFiles, err := filepath.Glob(SpotifyPath("Streaming_History_Audio_*.json"))
	if err != nil {
		return false, err
	}
	for _, spotifyPath := range spotifyFiles {
		info, err := os.Stat(spotifyPath)
		if err != nil {
			return false, err
		}
		if info.ModTime().After(historyMod) {
			return true, nil
		}
	}

	return false, nil
}

func runMergeListening() {
	fmt.Println("Merging Last.fm and Spotify listening history...")

	config := defaultListeningMergeConfig()
	history, report, err := buildListeningHistory(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error building listening history: %v\n", err)
		os.Exit(1)
	}

	if err := writeListeningHistoryArtifacts(history, report); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing listening history artifacts: %v\n", err)
		os.Exit(1)
	}

	reportPath := DataPath("listening-merge-report.json")
	historyPath := DataPath("listening-history.json")

	fmt.Printf("Last.fm valid events: %d\n", report.LastFm.ValidEvents)
	fmt.Printf("Spotify qualified events: %d\n", report.Spotify.QualifiedEvents)
	fmt.Printf("Spotify duplicates (exact): %d\n", report.Spotify.DedupedExactWithinSpotify)
	fmt.Printf("Spotify duplicates vs Last.fm: %d\n", report.Spotify.DedupedAgainstLastFm)
	fmt.Printf("Spotify events added: %d\n", report.Spotify.AddedEvents)
	fmt.Printf("Merged total events: %d\n", report.Output.TotalEvents)
	fmt.Printf("History output: %s\n", historyPath)
	fmt.Printf("Merge report: %s\n", reportPath)
}
