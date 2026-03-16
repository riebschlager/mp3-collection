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
	artistRaceTimezone     = "America/Chicago"
	artistRaceMetric       = "cumulative"
	artistRaceEntityType   = "artist"
	artistRaceDisplayCount = 12
)

type ArtistRaceLeader struct {
	Rank       int    `json:"rank"`
	Artist     string `json:"artist"`
	ArtistSlug string `json:"artistSlug"`
	PlayCount  int    `json:"playCount"`
}

type ArtistRaceFrame struct {
	Month    string             `json:"month"`
	Label    string             `json:"label"`
	FrameEnd int64              `json:"frameEnd"`
	Leaders  []ArtistRaceLeader `json:"leaders"`
}

type ArtistRaceData struct {
	GeneratedAt    string            `json:"generatedAt"`
	Timezone       string            `json:"timezone"`
	Metric         string            `json:"metric"`
	EntityType     string            `json:"entityType"`
	FirstMonth     string            `json:"firstMonth"`
	LastMonth      string            `json:"lastMonth"`
	TotalFrames    int               `json:"totalFrames"`
	TotalScrobbles int               `json:"totalScrobbles"`
	DisplayCount   int               `json:"displayCount"`
	Frames         []ArtistRaceFrame `json:"frames"`
}

type artistRaceMonthBucket struct {
	key       string
	label     string
	monthEnd  int64
	lastEvent int64
}

type artistRaceLeaderboardEntry struct {
	Artist     string
	ArtistSlug string
	PlayCount  int
}

func monthKeyInLocation(timestampMs int64, loc *time.Location) string {
	return time.UnixMilli(timestampMs).In(loc).Format("2006-01")
}

func monthLabelFromKey(monthKey string) string {
	t, err := time.Parse("2006-01", monthKey)
	if err != nil {
		return monthKey
	}
	return t.Format("Jan 2006")
}

func monthEndTimestamp(monthKey string, loc *time.Location) int64 {
	monthStart, err := time.ParseInLocation("2006-01", monthKey, loc)
	if err != nil {
		return 0
	}
	nextMonth := monthStart.AddDate(0, 1, 0)
	return nextMonth.Add(-time.Millisecond).UnixMilli()
}

func enumerateMonthBuckets(firstMonth, lastMonth string, loc *time.Location, lastEventMs int64) []artistRaceMonthBucket {
	start, err := time.ParseInLocation("2006-01", firstMonth, loc)
	if err != nil {
		return nil
	}
	end, err := time.ParseInLocation("2006-01", lastMonth, loc)
	if err != nil {
		return nil
	}

	buckets := make([]artistRaceMonthBucket, 0, 24)
	for cursor := start; !cursor.After(end); cursor = cursor.AddDate(0, 1, 0) {
		key := cursor.Format("2006-01")
		frameEnd := monthEndTimestamp(key, loc)
		if key == lastMonth {
			frameEnd = lastEventMs
		}
		buckets = append(buckets, artistRaceMonthBucket{
			key:       key,
			label:     cursor.Format("Jan 2006"),
			monthEnd:  frameEnd,
			lastEvent: frameEnd,
		})
	}
	return buckets
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

func snapshotArtistRaceLeaders(counts map[string]int, artistNames map[string]string, displayCount int) []ArtistRaceLeader {
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
	if len(entries) > displayCount {
		entries = entries[:displayCount]
	}

	leaders := make([]ArtistRaceLeader, 0, len(entries))
	for index, entry := range entries {
		leaders = append(leaders, ArtistRaceLeader{
			Rank:       index + 1,
			Artist:     entry.Artist,
			ArtistSlug: entry.ArtistSlug,
			PlayCount:  entry.PlayCount,
		})
	}
	return leaders
}

func buildArtistRaceData(history *ListeningHistoryData, loc *time.Location, displayCount int, generatedAt time.Time) (*ArtistRaceData, error) {
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

	firstMonth := monthKeyInLocation(events[0].Date, loc)
	lastMonth := monthKeyInLocation(events[len(events)-1].Date, loc)
	buckets := enumerateMonthBuckets(firstMonth, lastMonth, loc, events[len(events)-1].Date)
	if len(buckets) == 0 {
		return nil, fmt.Errorf("unable to enumerate month buckets")
	}

	counts := make(map[string]int)
	artistNames := make(map[string]string)
	frames := make([]ArtistRaceFrame, 0, len(buckets))
	eventIndex := 0

	for _, bucket := range buckets {
		for eventIndex < len(events) {
			event := events[eventIndex]
			eventMonth := monthKeyInLocation(event.Date, loc)
			if eventMonth != bucket.key {
				break
			}

			artist := strings.TrimSpace(event.Artist)
			if artist != "" {
				counts[artist]++
				artistNames[artist] = artist
				if event.Date > bucket.lastEvent {
					bucket.lastEvent = event.Date
				}
			}
			eventIndex++
		}

		frames = append(frames, ArtistRaceFrame{
			Month:    bucket.key,
			Label:    bucket.label,
			FrameEnd: bucket.lastEvent,
			Leaders:  snapshotArtistRaceLeaders(counts, artistNames, displayCount),
		})
	}

	return &ArtistRaceData{
		GeneratedAt:    generatedAt.UTC().Format(time.RFC3339),
		Timezone:       loc.String(),
		Metric:         artistRaceMetric,
		EntityType:     artistRaceEntityType,
		FirstMonth:     firstMonth,
		LastMonth:      lastMonth,
		TotalFrames:    len(frames),
		TotalScrobbles: len(events),
		DisplayCount:   displayCount,
		Frames:         frames,
	}, nil
}

func runBuildArtistRace() {
	fmt.Println("Building artist race data from merged listening history...")

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

	artifact, err := buildArtistRaceData(history, loc, artistRaceDisplayCount, time.Now())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error building artist race: %v\n", err)
		os.Exit(1)
	}

	outputPath := WebDataPath("artist-race.json")
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	writeJSON(outputPath, artifact)

	fmt.Println("Artist race data generated successfully!")
	fmt.Printf("Time range: %s to %s\n", artifact.FirstMonth, artifact.LastMonth)
	fmt.Printf("Frames: %d\n", artifact.TotalFrames)
	fmt.Printf("Total scrobbles counted: %d\n", artifact.TotalScrobbles)
	fmt.Printf("Output: %s\n", outputPath)
}
