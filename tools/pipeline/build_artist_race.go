package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	artistRaceTimezone              = "America/Chicago"
	artistRaceEntityType            = "artist"
	artistRaceDisplayCount          = 12
	artistRaceDefaultGranularity    = "week"
	artistRaceDefaultWindowKey      = "13w"
	artistRaceStickyCandidateCount  = 24
	artistRaceStickyPromotionMargin = 5
)

var artistRaceWindowConfigs = []artistRaceWindowConfig{
	{
		Key:      "all",
		Label:    "All Time",
		Metric:   "cumulative",
		Duration: 0,
	},
	{
		Key:      "4w",
		Label:    "Last 4 Weeks",
		Metric:   "trailing-window",
		Duration: 4 * 7 * 24 * time.Hour,
	},
	{
		Key:      "13w",
		Label:    "Last 13 Weeks",
		Metric:   "trailing-window",
		Duration: 13 * 7 * 24 * time.Hour,
	},
	{
		Key:      "52w",
		Label:    "Last 52 Weeks",
		Metric:   "trailing-window",
		Duration: 52 * 7 * 24 * time.Hour,
	},
}

var artistRaceGranularities = []string{"week", "month"}

type ArtistRaceLeader struct {
	Rank       int    `json:"rank"`
	RawRank    int    `json:"rawRank"`
	Artist     string `json:"artist"`
	ArtistSlug string `json:"artistSlug"`
	PlayCount  int    `json:"playCount"`
}

type ArtistRaceFrame struct {
	Key                  string             `json:"key"`
	Label                string             `json:"label"`
	FrameStart           int64              `json:"frameStart"`
	FrameEnd             int64              `json:"frameEnd"`
	WindowTotalScrobbles int                `json:"windowTotalScrobbles"`
	Leaders              []ArtistRaceLeader `json:"leaders"`
}

type ArtistRaceVariantData struct {
	GeneratedAt         string            `json:"generatedAt"`
	Timezone            string            `json:"timezone"`
	EntityType          string            `json:"entityType"`
	Granularity         string            `json:"granularity"`
	WindowKey           string            `json:"windowKey"`
	Metric              string            `json:"metric"`
	OrderingMode        string            `json:"orderingMode"`
	OrderingMarginPlays int               `json:"orderingMarginPlays,omitempty"`
	DisplayCount        int               `json:"displayCount"`
	TotalFrames         int               `json:"totalFrames"`
	TotalScrobbles      int               `json:"totalScrobbles"`
	Frames              []ArtistRaceFrame `json:"frames"`
}

type ArtistRaceVariantManifest struct {
	Granularity   string `json:"granularity"`
	WindowKey     string `json:"windowKey"`
	Label         string `json:"label"`
	Path          string `json:"path"`
	TotalFrames   int    `json:"totalFrames"`
	FirstFrameKey string `json:"firstFrameKey"`
	LastFrameKey  string `json:"lastFrameKey"`
}

type ArtistRaceManifest struct {
	GeneratedAt        string                      `json:"generatedAt"`
	Timezone           string                      `json:"timezone"`
	EntityType         string                      `json:"entityType"`
	DisplayCount       int                         `json:"displayCount"`
	DefaultGranularity string                      `json:"defaultGranularity"`
	DefaultWindowKey   string                      `json:"defaultWindowKey"`
	Variants           []ArtistRaceVariantManifest `json:"variants"`
}

type artistRaceBucket struct {
	Key        string
	Label      string
	FrameStart int64
	FrameEnd   int64
}

type artistRaceWindowConfig struct {
	Key      string
	Label    string
	Metric   string
	Duration time.Duration
}

type artistRaceLeaderboardEntry struct {
	Artist     string
	ArtistSlug string
	PlayCount  int
	RawRank    int
}

type artistRaceVariantOutput struct {
	Path string
	Data *ArtistRaceVariantData
}

func normalizeArtistRaceEvents(history *ListeningHistoryData) ([]ListeningEvent, error) {
	if history == nil {
		return nil, fmt.Errorf("listening history is required")
	}

	events := make([]ListeningEvent, 0, len(history.Events))
	for _, event := range history.Events {
		if strings.TrimSpace(event.Artist) == "" {
			continue
		}
		events = append(events, event)
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("no listening events with artist names found")
	}

	sort.Slice(events, func(i, j int) bool {
		if events[i].Date != events[j].Date {
			return events[i].Date < events[j].Date
		}
		if events[i].Artist != events[j].Artist {
			return events[i].Artist < events[j].Artist
		}
		return events[i].Track < events[j].Track
	})

	return events, nil
}

func localStartOfDay(t time.Time, loc *time.Location) time.Time {
	local := t.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
}

func weekStartForTime(t time.Time, loc *time.Location) time.Time {
	start := localStartOfDay(t, loc)
	weekday := int(start.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	return start.AddDate(0, 0, -(weekday - 1))
}

func monthStartForTime(t time.Time, loc *time.Location) time.Time {
	local := t.In(loc)
	return time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, loc)
}

func artistRaceBucketStartForTimestamp(timestampMs int64, granularity string, loc *time.Location) (time.Time, error) {
	t := time.UnixMilli(timestampMs)
	switch granularity {
	case "week":
		return weekStartForTime(t, loc), nil
	case "month":
		return monthStartForTime(t, loc), nil
	default:
		return time.Time{}, fmt.Errorf("unsupported granularity %q", granularity)
	}
}

func artistRaceBucketAdvance(start time.Time, granularity string) (time.Time, error) {
	switch granularity {
	case "week":
		return start.AddDate(0, 0, 7), nil
	case "month":
		return start.AddDate(0, 1, 0), nil
	default:
		return time.Time{}, fmt.Errorf("unsupported granularity %q", granularity)
	}
}

func artistRaceWeekKey(start time.Time) string {
	year, week := start.ISOWeek()
	return fmt.Sprintf("%04d-W%02d", year, week)
}

func artistRaceWeekLabel(start time.Time) string {
	end := start.AddDate(0, 0, 6)
	if start.Year() == end.Year() && start.Month() == end.Month() {
		return fmt.Sprintf("%s %d-%d, %d", start.Format("Jan"), start.Day(), end.Day(), start.Year())
	}
	if start.Year() == end.Year() {
		return fmt.Sprintf("%s %d-%s %d, %d", start.Format("Jan"), start.Day(), end.Format("Jan"), end.Day(), start.Year())
	}
	return fmt.Sprintf("%s %d, %d-%s %d, %d", start.Format("Jan"), start.Day(), start.Year(), end.Format("Jan"), end.Day(), end.Year())
}

func artistRaceBucketKey(start time.Time, granularity string) string {
	if granularity == "week" {
		return artistRaceWeekKey(start)
	}
	return start.Format("2006-01")
}

func artistRaceBucketLabel(start time.Time, granularity string) string {
	if granularity == "week" {
		return artistRaceWeekLabel(start)
	}
	return start.Format("Jan 2006")
}

func enumerateArtistRaceBuckets(firstEventMs, lastEventMs int64, granularity string, loc *time.Location) ([]artistRaceBucket, error) {
	firstStart, err := artistRaceBucketStartForTimestamp(firstEventMs, granularity, loc)
	if err != nil {
		return nil, err
	}
	lastStart, err := artistRaceBucketStartForTimestamp(lastEventMs, granularity, loc)
	if err != nil {
		return nil, err
	}

	buckets := make([]artistRaceBucket, 0, 64)
	for cursor := firstStart; !cursor.After(lastStart); {
		next, err := artistRaceBucketAdvance(cursor, granularity)
		if err != nil {
			return nil, err
		}

		frameEnd := next.Add(-time.Millisecond).UnixMilli()
		if cursor.Equal(lastStart) {
			frameEnd = lastEventMs
		}

		buckets = append(buckets, artistRaceBucket{
			Key:        artistRaceBucketKey(cursor, granularity),
			Label:      artistRaceBucketLabel(cursor, granularity),
			FrameStart: cursor.UnixMilli(),
			FrameEnd:   frameEnd,
		})

		cursor = next
	}

	return buckets, nil
}

func sortArtistRaceLeaders(entries []artistRaceLeaderboardEntry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].PlayCount != entries[j].PlayCount {
			return entries[i].PlayCount > entries[j].PlayCount
		}

		left := strings.ToLower(entries[i].Artist)
		right := strings.ToLower(entries[j].Artist)
		if left != right {
			return left < right
		}
		return entries[i].Artist < entries[j].Artist
	})
}

func buildArtistRaceLeaderboardEntries(counts map[string]int, artistNames map[string]string) []artistRaceLeaderboardEntry {
	entries := make([]artistRaceLeaderboardEntry, 0, len(counts))
	for artist, playCount := range counts {
		name := strings.TrimSpace(artistNames[artist])
		if name == "" || playCount <= 0 {
			continue
		}
		entries = append(entries, artistRaceLeaderboardEntry{
			Artist:     name,
			ArtistSlug: Slugify(name),
			PlayCount:  playCount,
		})
	}

	sortArtistRaceLeaders(entries)
	for index := range entries {
		entries[index].RawRank = index + 1
	}
	return entries
}

func snapshotArtistRaceExactLeaders(entries []artistRaceLeaderboardEntry, displayCount int) []ArtistRaceLeader {
	if len(entries) > displayCount {
		entries = entries[:displayCount]
	}
	leaders := make([]ArtistRaceLeader, 0, len(entries))
	for index, entry := range entries {
		leaders = append(leaders, ArtistRaceLeader{
			Rank:       index + 1,
			RawRank:    entry.RawRank,
			Artist:     entry.Artist,
			ArtistSlug: entry.ArtistSlug,
			PlayCount:  entry.PlayCount,
		})
	}
	return leaders
}

func artistRaceUsesStickyOrdering(granularity string, window artistRaceWindowConfig) bool {
	return granularity == "week" && window.Metric == "trailing-window"
}

func buildStickyArtistRaceLeaders(entries []artistRaceLeaderboardEntry, previous []ArtistRaceLeader, displayCount int) []ArtistRaceLeader {
	if len(entries) == 0 {
		return []ArtistRaceLeader{}
	}

	entryByArtist := make(map[string]artistRaceLeaderboardEntry, len(entries))
	for _, entry := range entries {
		entryByArtist[entry.Artist] = entry
	}

	candidateCount := artistRaceStickyCandidateCount
	if candidateCount > len(entries) {
		candidateCount = len(entries)
	}

	orderedArtists := make([]string, 0, len(previous)+candidateCount)
	seen := make(map[string]struct{}, len(previous)+candidateCount)

	for _, leader := range previous {
		if _, ok := entryByArtist[leader.Artist]; !ok {
			continue
		}
		if _, ok := seen[leader.Artist]; ok {
			continue
		}
		orderedArtists = append(orderedArtists, leader.Artist)
		seen[leader.Artist] = struct{}{}
	}

	for _, entry := range entries[:candidateCount] {
		if _, ok := seen[entry.Artist]; ok {
			continue
		}
		orderedArtists = append(orderedArtists, entry.Artist)
		seen[entry.Artist] = struct{}{}
	}

	swapped := true
	for swapped {
		swapped = false
		for index := 1; index < len(orderedArtists); index++ {
			challenger := entryByArtist[orderedArtists[index]]
			incumbent := entryByArtist[orderedArtists[index-1]]
			if challenger.PlayCount-incumbent.PlayCount < artistRaceStickyPromotionMargin {
				continue
			}

			orderedArtists[index-1], orderedArtists[index] = orderedArtists[index], orderedArtists[index-1]
			swapped = true
		}
	}

	leaderCount := displayCount
	if leaderCount > len(orderedArtists) {
		leaderCount = len(orderedArtists)
	}

	leaders := make([]ArtistRaceLeader, 0, leaderCount)
	for index := 0; index < leaderCount; index++ {
		entry := entryByArtist[orderedArtists[index]]
		leaders = append(leaders, ArtistRaceLeader{
			Rank:       index + 1,
			RawRank:    entry.RawRank,
			Artist:     entry.Artist,
			ArtistSlug: entry.ArtistSlug,
			PlayCount:  entry.PlayCount,
		})
	}

	return leaders
}

func buildArtistRaceVariantData(events []ListeningEvent, loc *time.Location, granularity string, window artistRaceWindowConfig, displayCount int, generatedAt time.Time) (*ArtistRaceVariantData, error) {
	if len(events) == 0 {
		return nil, fmt.Errorf("artist race events are required")
	}

	buckets, err := enumerateArtistRaceBuckets(events[0].Date, events[len(events)-1].Date, granularity, loc)
	if err != nil {
		return nil, err
	}
	if len(buckets) == 0 {
		return nil, fmt.Errorf("unable to enumerate %s buckets", granularity)
	}

	counts := make(map[string]int)
	artistNames := make(map[string]string)
	frames := make([]ArtistRaceFrame, 0, len(buckets))
	addIndex := 0
	removeIndex := 0
	windowScrobbles := 0
	stickyOrdering := artistRaceUsesStickyOrdering(granularity, window)
	orderingMode := "exact"
	orderingMarginPlays := 0
	if stickyOrdering {
		orderingMode = "sticky-hysteresis"
		orderingMarginPlays = artistRaceStickyPromotionMargin
	}
	var previousDisplayed []ArtistRaceLeader

	for _, bucket := range buckets {
		for addIndex < len(events) && events[addIndex].Date <= bucket.FrameEnd {
			event := events[addIndex]
			artist := strings.TrimSpace(event.Artist)
			if artist != "" {
				counts[artist]++
				artistNames[artist] = artist
				windowScrobbles++
			}
			addIndex++
		}

		if window.Duration > 0 {
			windowStart := bucket.FrameEnd - window.Duration.Milliseconds() + 1
			for removeIndex < addIndex && events[removeIndex].Date < windowStart {
				artist := strings.TrimSpace(events[removeIndex].Artist)
				if artist != "" {
					counts[artist]--
					if counts[artist] <= 0 {
						delete(counts, artist)
					}
					windowScrobbles--
				}
				removeIndex++
			}
		}

		rawEntries := buildArtistRaceLeaderboardEntries(counts, artistNames)
		leaders := snapshotArtistRaceExactLeaders(rawEntries, displayCount)
		if stickyOrdering {
			leaders = buildStickyArtistRaceLeaders(rawEntries, previousDisplayed, displayCount)
		}

		frames = append(frames, ArtistRaceFrame{
			Key:                  bucket.Key,
			Label:                bucket.Label,
			FrameStart:           bucket.FrameStart,
			FrameEnd:             bucket.FrameEnd,
			WindowTotalScrobbles: windowScrobbles,
			Leaders:              leaders,
		})
		previousDisplayed = leaders
	}

	return &ArtistRaceVariantData{
		GeneratedAt:         generatedAt.UTC().Format(time.RFC3339),
		Timezone:            loc.String(),
		EntityType:          artistRaceEntityType,
		Granularity:         granularity,
		WindowKey:           window.Key,
		Metric:              window.Metric,
		OrderingMode:        orderingMode,
		OrderingMarginPlays: orderingMarginPlays,
		DisplayCount:        displayCount,
		TotalFrames:         len(frames),
		TotalScrobbles:      len(events),
		Frames:              frames,
	}, nil
}

func artistRaceVariantLabel(granularity string, window artistRaceWindowConfig) string {
	prefix := "Weekly"
	if granularity == "month" {
		prefix = "Monthly"
	}
	return fmt.Sprintf("%s, %s", prefix, window.Label)
}

func buildArtistRaceArtifacts(history *ListeningHistoryData, loc *time.Location, displayCount int, generatedAt time.Time) (*ArtistRaceManifest, []artistRaceVariantOutput, error) {
	events, err := normalizeArtistRaceEvents(history)
	if err != nil {
		return nil, nil, err
	}

	manifest := &ArtistRaceManifest{
		GeneratedAt:        generatedAt.UTC().Format(time.RFC3339),
		Timezone:           loc.String(),
		EntityType:         artistRaceEntityType,
		DisplayCount:       displayCount,
		DefaultGranularity: artistRaceDefaultGranularity,
		DefaultWindowKey:   artistRaceDefaultWindowKey,
		Variants:           make([]ArtistRaceVariantManifest, 0, len(artistRaceGranularities)*len(artistRaceWindowConfigs)),
	}

	outputs := make([]artistRaceVariantOutput, 0, len(manifest.Variants))

	for _, granularity := range artistRaceGranularities {
		for _, window := range artistRaceWindowConfigs {
			variant, err := buildArtistRaceVariantData(events, loc, granularity, window, displayCount, generatedAt)
			if err != nil {
				return nil, nil, fmt.Errorf("build %s/%s artist race: %w", granularity, window.Key, err)
			}

			path := filepath.ToSlash(filepath.Join("artist-race", fmt.Sprintf("%s-%s.json", granularity, window.Key)))
			manifest.Variants = append(manifest.Variants, ArtistRaceVariantManifest{
				Granularity:   granularity,
				WindowKey:     window.Key,
				Label:         artistRaceVariantLabel(granularity, window),
				Path:          path,
				TotalFrames:   variant.TotalFrames,
				FirstFrameKey: variant.Frames[0].Key,
				LastFrameKey:  variant.Frames[len(variant.Frames)-1].Key,
			})
			outputs = append(outputs, artistRaceVariantOutput{
				Path: path,
				Data: variant,
			})
		}
	}

	return manifest, outputs, nil
}

func runBuildArtistRace() {
	fmt.Println("Building artist race variants from merged listening history...")

	history, err := loadListeningHistoryOrBuild()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading listening history: %v\n", err)
		os.Exit(1)
	}

	loc, err := time.LoadLocation(artistRaceTimezone)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading timezone %s: %v\n", artistRaceTimezone, err)
		os.Exit(1)
	}

	generatedAt := time.Now()
	manifest, outputs, err := buildArtistRaceArtifacts(history, loc, artistRaceDisplayCount, generatedAt)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error building artist race: %v\n", err)
		os.Exit(1)
	}

	outputDir := WebDataPath("artist-race")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	writeJSON(filepath.Join(outputDir, "index.json"), manifest)
	for _, output := range outputs {
		writeJSON(WebDataPath(output.Path), output.Data)
	}
	if err := os.Remove(WebDataPath("artist-race.json")); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error removing legacy artist race artifact: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Artist race variants written to %s\n", outputDir)
}
