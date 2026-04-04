package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// WebTrack represents the enriched track object for the web app
type WebTrack struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Artist            string `json:"artist"`
	ArtistSlug        string `json:"artistSlug"`
	Composer          string `json:"composer,omitempty"` // omitempty for string? Python safe_str returns None if empty.
	Album             string `json:"album"`
	AlbumSlug         string `json:"albumSlug"`
	Grouping          string `json:"grouping,omitempty"`
	Genre             string `json:"genre,omitempty"`
	Year              *int   `json:"year"` // Nullable
	Size              int    `json:"size"`
	Duration          int    `json:"duration"`
	DurationFormatted string `json:"durationFormatted"`
	BitRate           *int   `json:"bitRate"`    // Nullable
	SampleRate        *int   `json:"sampleRate"` // Nullable
	TrackNumber       *int   `json:"trackNumber"`
	TrackCount        *int   `json:"trackCount"`
	DiscNumber        *int   `json:"discNumber"`
	DiscCount         *int   `json:"discCount"`
	LastPlayed        string `json:"lastPlayed,omitempty"`
	Rating            int    `json:"rating"`
	PlayCount         int    `json:"playCount"`
	DateAdded         string `json:"dateAdded,omitempty"`
	DateModified      string `json:"dateModified,omitempty"`
	Location          string `json:"location,omitempty"`
	Kind              string `json:"kind,omitempty"`
	VolumeAdjustment  *int   `json:"volumeAdjustment"` // Nullable (if 0 -> None)
	Equalizer         string `json:"equalizer,omitempty"`
	Comments          string `json:"comments,omitempty"`
}

type ChunkData struct {
	Chunk       int        `json:"chunk"`
	TotalChunks int        `json:"totalChunks"`
	Count       int        `json:"count"`
	Tracks      []WebTrack `json:"tracks"`
}

type ArtistIndexEntry struct {
	Slug       string   `json:"slug"`
	Name       string   `json:"name"`
	AlbumCount int      `json:"albumCount"`
	TrackCount int      `json:"trackCount"`
	Albums     []string `json:"albums"`
}

type ArtistIndex struct {
	Total   int                `json:"total"`
	Artists []ArtistIndexEntry `json:"artists"`
}

type AlbumIndexEntry struct {
	Slug        string   `json:"slug"`
	Name        string   `json:"name"`
	ArtistCount int      `json:"artistCount"`
	TrackCount  int      `json:"trackCount"`
	Artists     []string `json:"artists"`
}

type AlbumIndex struct {
	Total  int               `json:"total"`
	Albums []AlbumIndexEntry `json:"albums"`
}

type Stats struct {
	TotalSizeBytes         int64   `json:"totalSizeBytes"`
	TotalSizeGB            float64 `json:"totalSizeGB"`
	TotalDurationSeconds   int64   `json:"totalDurationSeconds"`
	TotalDurationHours     float64 `json:"totalDurationHours"`
	TotalDurationFormatted string  `json:"totalDurationFormatted"`
	AvgBitRate             float64 `json:"avgBitRate"`
	TracksWithRating       int     `json:"tracksWithRating"`
}

type Metadata struct {
	TotalTracks  int      `json:"totalTracks"`
	TotalArtists int      `json:"totalArtists"`
	TotalAlbums  int      `json:"totalAlbums"`
	Genres       []string `json:"genres"`
	Years        []int    `json:"years"`
	Stats        Stats    `json:"stats"`
}

// PlayCountLookup represents the structure for looking up playcounts
type PlayCountLookup struct {
	Artist    string
	Track     string
	PlayCount int
	FirstPlay int64
	LastPlay  int64
}

// NormalizeForMatching normalizes a string for matching (same as in process_lastfm.go)
func normalizeForMatching(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " & ", " and ")
	s = strings.ReplaceAll(s, "&", "and")

	featuringPatterns := []string{
		" feat. ", " feat ", " ft. ", " ft ", " featuring ",
	}
	for _, pattern := range featuringPatterns {
		if idx := strings.Index(s, pattern); idx != -1 {
			s = s[:idx]
		}
	}

	// Remove anything in parentheses or brackets
	s = strings.NewReplacer(
		"(", "", ")", "",
		"[", "", "]", "",
	).Replace(s)

	// Remove non-alphanumeric characters except spaces
	var result strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == ' ' {
			result.WriteRune(r)
		}
	}
	s = result.String()

	// Collapse multiple spaces
	s = strings.Join(strings.Fields(s), " ")

	return strings.TrimSpace(s)
}

// LoadPlayCounts loads the playcounts.json file and creates a lookup map
func loadPlayCounts(playCountPath string) (map[string]PlayCountLookup, error) {
	data, err := os.ReadFile(playCountPath)
	if err != nil {
		return nil, err
	}

	var playCountData struct {
		TotalScrobbles int `json:"totalScrobbles"`
		UniqueTracks   int `json:"uniqueTracks"`
		PlayCounts     []struct {
			Artist    string `json:"artist"`
			Track     string `json:"track"`
			PlayCount int    `json:"playCount"`
			FirstPlay int64  `json:"firstPlay"`
			LastPlay  int64  `json:"lastPlay"`
		} `json:"playCounts"`
	}

	if err := json.Unmarshal(data, &playCountData); err != nil {
		return nil, err
	}

	// Create lookup map with normalized keys
	lookup := make(map[string]PlayCountLookup)
	for _, pc := range playCountData.PlayCounts {
		normalizedArtist := normalizeForMatching(pc.Artist)
		normalizedTrack := normalizeForMatching(pc.Track)
		key := normalizedArtist + "|" + normalizedTrack

		lookup[key] = PlayCountLookup{
			Artist:    pc.Artist,
			Track:     pc.Track,
			PlayCount: pc.PlayCount,
			FirstPlay: pc.FirstPlay,
			LastPlay:  pc.LastPlay,
		}
	}

	return lookup, nil
}

func runBuildWebData() {
	csvPath := CompiledPath("compiled_itunes_library.csv")
	playCountPath := DataPath("playcounts.json")
	outputDir := Paths.WebDataDir
	chunksDir := filepath.Join(outputDir, "chunks")

	if err := os.MkdirAll(chunksDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Reading CSV from: %s\n", csvPath)
	fmt.Printf("Output directory: %s\n\n", outputDir)

	// Load playcounts
	fmt.Println("Loading listening-history playcounts...")
	playCountLookup, err := loadPlayCounts(playCountPath)
	if err != nil {
		fmt.Printf("Warning: Could not load playcounts (%v). Continuing without playcount data.\n\n", err)
		playCountLookup = make(map[string]PlayCountLookup)
	} else {
		fmt.Printf("Loaded %d unique track playcounts\n\n", len(playCountLookup))
	}

	rows, err := ReadCSV(csvPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading CSV: %v\n", err)
		os.Exit(1)
	}

	var tracks []WebTrack

	// Index maps
	type ArtistData struct {
		Name     string
		Albums   map[string]bool
		TrackIDs []string
	}
	artistsMap := make(map[string]*ArtistData)

	type AlbumData struct {
		Name     string
		Artists  map[string]bool
		TrackIDs []string
	}
	albumsMap := make(map[string]*AlbumData)

	genresSet := make(map[string]bool)
	yearsSet := make(map[int]bool)

	var totalSize int64
	var totalDuration int64
	var totalBitrateSum int64
	var bitrateCount int64
	var tracksWithPlayCount int

	for _, row := range rows {
		trackName := strings.TrimSpace(row["Name"])
		if trackName == "" {
			continue
		}

		artistName := SanitizeArtistName(SafeStr(row["Artist"]))
		if artistName == "" {
			artistName = "Unknown Artist"
		}
		albumName := SanitizeAlbumName(SafeStr(row["Album"]))
		if albumName == "" {
			albumName = "Unknown Album"
		}

		if !IsValidName(artistName) || !IsValidName(albumName) {
			continue
		}

		composer := SafeStr(row["Composer"])
		genre := SanitizeGenre(SafeStr(row["Genre"]))
		yearVal := SanitizeYear(row["Year"])

		size := SafeInt(row["Size"])
		duration := SafeInt(row["Time"])
		bitRate := SafeInt(row["Bit Rate"])
		sampleRate := SafeInt(row["Sample Rate"])

		trackNumber := SafeInt(row["Track Number"])
		trackCount := SafeInt(row["Track Count"])
		discNumber := SafeInt(row["Disc Number"])
		discCount := SafeInt(row["Disc Count"])

		rating := SafeInt(row["My Rating"])

		dateAdded := SafeStr(row["Date Added"])
		dateModified := SafeStr(row["Date Modified"])
		lastPlayed := SafeStr(row["Last Played"])
		location := SafeStr(row["Location"])
		kind := SafeStr(row["Kind"])
		grouping := SafeStr(row["Grouping"])
		comments := SafeStr(row["Comments"])
		volumeAdjustment := SafeInt(row["Volume Adjustment"])
		equalizer := SafeStr(row["Equalizer"])

		trackID := fmt.Sprintf("track-%05d", len(tracks))
		artistSlug := Slugify(artistName)
		albumSlug := Slugify(albumName)

		// Helpers for nullable fields
		intPtr := func(v int) *int {
			if v > 0 {
				return &v
			}
			return nil
		}

		// Year: 0 -> nil
		var yearPtr *int
		if yearVal > 0 {
			yearPtr = &yearVal
		}

		// Volume adj: if 0 -> nil
		var volPtr *int
		if volumeAdjustment != 0 {
			volPtr = &volumeAdjustment
		}

		// Look up playcount
		playCount := 0
		normalizedArtist := normalizeForMatching(artistName)
		normalizedTrack := normalizeForMatching(trackName)
		lookupKey := normalizedArtist + "|" + normalizedTrack
		if pcEntry, found := playCountLookup[lookupKey]; found {
			playCount = pcEntry.PlayCount
			tracksWithPlayCount++
		}

		track := WebTrack{
			ID:                trackID,
			Name:              trackName,
			Artist:            artistName,
			ArtistSlug:        artistSlug,
			Composer:          composer,
			Album:             albumName,
			AlbumSlug:         albumSlug,
			Grouping:          grouping,
			Genre:             genre,
			Year:              yearPtr,
			Size:              size,
			Duration:          duration,
			DurationFormatted: FormatDuration(duration),
			BitRate:           intPtr(bitRate),
			SampleRate:        intPtr(sampleRate),
			TrackNumber:       intPtr(trackNumber),
			TrackCount:        intPtr(trackCount),
			DiscNumber:        intPtr(discNumber),
			DiscCount:         intPtr(discCount),
			LastPlayed:        lastPlayed,
			Rating:            rating,
			PlayCount:         playCount,
			DateAdded:         dateAdded,
			DateModified:      dateModified,
			Location:          location,
			Kind:              kind,
			VolumeAdjustment:  volPtr,
			Equalizer:         equalizer,
			Comments:          comments,
		}

		tracks = append(tracks, track)

		// Update indices
		if _, ok := artistsMap[artistSlug]; !ok {
			artistsMap[artistSlug] = &ArtistData{
				Name:   artistName,
				Albums: make(map[string]bool),
			}
		}
		artistsMap[artistSlug].Albums[albumName] = true
		artistsMap[artistSlug].TrackIDs = append(artistsMap[artistSlug].TrackIDs, trackID)

		if _, ok := albumsMap[albumSlug]; !ok {
			albumsMap[albumSlug] = &AlbumData{
				Name:    albumName,
				Artists: make(map[string]bool),
			}
		}
		albumsMap[albumSlug].Artists[artistName] = true
		albumsMap[albumSlug].TrackIDs = append(albumsMap[albumSlug].TrackIDs, trackID)

		// Metadata stats
		if genre != "" {
			genresSet[genre] = true
		}
		if yearVal > 0 {
			yearsSet[yearVal] = true
		}

		totalSize += int64(size)
		totalDuration += int64(duration)
		if bitRate > 0 {
			totalBitrateSum += int64(bitRate)
			bitrateCount++
		}
	}

	fmt.Printf("Processed %d tracks\n", len(tracks))
	fmt.Printf("Found %d unique artists\n", len(artistsMap))
	fmt.Printf("Found %d unique albums\n", len(albumsMap))
	fmt.Printf("Found %d genres\n", len(genresSet))
	fmt.Printf("Found %d years\n\n", len(yearsSet))

	// Write chunks
	chunkSize := 1000
	totalChunks := (len(tracks) + chunkSize - 1) / chunkSize
	fmt.Printf("Creating %d track chunks...\n", totalChunks)

	for i := 0; i < totalChunks; i++ {
		start := i * chunkSize
		end := start + chunkSize
		if end > len(tracks) {
			end = len(tracks)
		}
		chunkTracks := tracks[start:end]

		chunkData := ChunkData{
			Chunk:       i + 1,
			TotalChunks: totalChunks,
			Count:       len(chunkTracks),
			Tracks:      chunkTracks,
		}

		chunkPath := filepath.Join(chunksDir, fmt.Sprintf("tracks-%03d.json", i+1))
		writeJSON(chunkPath, chunkData)

		if (i+1)%10 == 0 || i == totalChunks-1 {
			fmt.Printf("  Wrote chunk %d/%d\n", i+1, totalChunks)
		}
	}

	// Write Artist Index
	fmt.Println("\nCreating artist index...")
	var artistIndexList []ArtistIndexEntry
	for slug, data := range artistsMap {
		var albums []string
		for alb := range data.Albums {
			albums = append(albums, alb)
		}
		sort.Strings(albums)

		artistIndexList = append(artistIndexList, ArtistIndexEntry{
			Slug:       slug,
			Name:       data.Name,
			AlbumCount: len(albums),
			TrackCount: len(data.TrackIDs),
			Albums:     albums,
		})
	}
	sort.Slice(artistIndexList, func(i, j int) bool {
		return strings.ToLower(artistIndexList[i].Name) < strings.ToLower(artistIndexList[j].Name)
	})

	artistIndex := ArtistIndex{
		Total:   len(artistIndexList),
		Artists: artistIndexList,
	}
	writeJSON(filepath.Join(outputDir, "artists-index.json"), artistIndex)
	fmt.Printf("  Wrote %d artists to artists-index.json\n", len(artistIndexList))

	// Write Album Index
	fmt.Println("\nCreating album index...")
	var albumIndexList []AlbumIndexEntry
	for slug, data := range albumsMap {
		var artists []string
		for art := range data.Artists {
			artists = append(artists, art)
		}
		sort.Strings(artists)

		albumIndexList = append(albumIndexList, AlbumIndexEntry{
			Slug:        slug,
			Name:        data.Name,
			ArtistCount: len(artists),
			TrackCount:  len(data.TrackIDs),
			Artists:     artists,
		})
	}
	sort.Slice(albumIndexList, func(i, j int) bool {
		return strings.ToLower(albumIndexList[i].Name) < strings.ToLower(albumIndexList[j].Name)
	})

	albumIndex := AlbumIndex{
		Total:  len(albumIndexList),
		Albums: albumIndexList,
	}
	writeJSON(filepath.Join(outputDir, "albums-index.json"), albumIndex)
	fmt.Printf("  Wrote %d albums to albums-index.json\n", len(albumIndexList))

	fmt.Println("\nCreating archive browser dataset...")
	archiveBrowser := buildArchiveBrowserData(tracks)
	writeJSON(filepath.Join(outputDir, "archive-browser.json"), archiveBrowser)
	fmt.Printf("  Wrote archive-browser.json with %d tracks, %d artists, %d albums, %d genres\n",
		archiveBrowser.Meta.TotalTracks,
		archiveBrowser.Meta.TotalArtists,
		archiveBrowser.Meta.TotalAlbums,
		archiveBrowser.Meta.TotalGenres,
	)

	// Metadata
	fmt.Println("\nCreating metadata file...")
	var genres []string
	for g := range genresSet {
		genres = append(genres, g)
	}
	sort.Strings(genres)

	var years []int
	for y := range yearsSet {
		years = append(years, y)
	}
	sort.Ints(years)

	avgBitRate := 0.0
	if bitrateCount > 0 {
		avgBitRate = float64(totalBitrateSum) / float64(bitrateCount)
	}

	totalHours := float64(totalDuration) / 3600.0
	totalSizeGB := float64(totalSize) / (1024 * 1024 * 1024)

	tracksRated := 0
	for _, t := range tracks {
		if t.Rating > 0 {
			tracksRated++
		}
	}

	durationFormatted := fmt.Sprintf("%dh %dm", int(totalHours), int((totalHours-math.Floor(totalHours))*60))

	metadata := Metadata{
		TotalTracks:  len(tracks),
		TotalArtists: len(artistIndexList),
		TotalAlbums:  len(albumIndexList),
		Genres:       genres,
		Years:        years,
		Stats: Stats{
			TotalSizeBytes:         totalSize,
			TotalSizeGB:            math.Round(totalSizeGB*100) / 100,
			TotalDurationSeconds:   totalDuration,
			TotalDurationHours:     math.Round(totalHours*10) / 10,
			TotalDurationFormatted: durationFormatted,
			AvgBitRate:             math.Round(avgBitRate*10) / 10,
			TracksWithRating:       tracksRated,
		},
	}
	writeJSON(filepath.Join(outputDir, "metadata.json"), metadata)
	fmt.Println("  Wrote metadata.json")

	// Summary
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("BUILD COMPLETE!")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Total Tracks:     %d\n", metadata.TotalTracks)
	fmt.Printf("Total Artists:    %d\n", metadata.TotalArtists)
	fmt.Printf("Total Albums:     %d\n", metadata.TotalAlbums)
	fmt.Printf("Total Size:       %.2f GB\n", metadata.Stats.TotalSizeGB)
	fmt.Printf("Total Duration:   %s\n", metadata.Stats.TotalDurationFormatted)
	fmt.Printf("Avg Bit Rate:     %.1f kbps\n", metadata.Stats.AvgBitRate)
	fmt.Printf("Tracks Rated:     %d\n", metadata.Stats.TracksWithRating)
	fmt.Printf("Tracks w/ Plays:  %d (%.1f%%)\n", tracksWithPlayCount, float64(tracksWithPlayCount)/float64(metadata.TotalTracks)*100)
	fmt.Println(strings.Repeat("=", 60))

	absOut, _ := filepath.Abs(outputDir)
	fmt.Printf("\nOutput directory: %s\n", absOut)
	fmt.Printf("Track chunks:     %d files in chunks/\n", totalChunks)
	fmt.Println("Index files:      artists-index.json, albums-index.json, archive-browser.json, metadata.json")
	fmt.Println("\nReady for Astro build!")
}

func writeJSON(path string, data interface{}) {
	file, err := os.Create(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating file %s: %v\n", path, err)
		os.Exit(1)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(data); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing JSON to %s: %v\n", path, err)
		os.Exit(1)
	}
}
