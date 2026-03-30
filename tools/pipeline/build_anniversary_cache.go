package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	anniversaryTimezone        = "America/Chicago"
	anniversaryWeekStart       = "sunday"
	anniversaryTopN            = 10
	anniversaryReferenceYear   = 2024
	anniversaryReferenceLayout = "2006-01-02"
	anniversaryMonthDayLayout  = "01-02"
)

type AnniversaryTrackEntry struct {
	Track     string `json:"track"`
	Artist    string `json:"artist"`
	Album     string `json:"album"`
	PlayCount int    `json:"playCount"`
}

type AnniversaryDayResult struct {
	Year         int                     `json:"year"`
	Date         string                  `json:"date"`
	TotalPlays   int                     `json:"totalPlays"`
	UniqueTracks int                     `json:"uniqueTracks"`
	TopTracks    []AnniversaryTrackEntry `json:"topTracks"`
}

type AnniversaryWeekResult struct {
	Year         int                     `json:"year"`
	StartDate    string                  `json:"startDate"`
	EndDate      string                  `json:"endDate"`
	TotalPlays   int                     `json:"totalPlays"`
	UniqueTracks int                     `json:"uniqueTracks"`
	ActiveDays   int                     `json:"activeDays"`
	TopTracks    []AnniversaryTrackEntry `json:"topTracks"`
}

type AnniversaryAnchorData struct {
	Label       string                  `json:"label"`
	DayResults  []AnniversaryDayResult  `json:"dayResults"`
	WeekResults []AnniversaryWeekResult `json:"weekResults"`
}

type AnniversaryCacheData struct {
	GeneratedAt        string                           `json:"generatedAt"`
	Timezone           string                           `json:"timezone"`
	WeekStart          string                           `json:"weekStart"`
	TopN               int                              `json:"topN"`
	AvailableMonthDays []string                         `json:"availableMonthDays"`
	Anchors            map[string]AnniversaryAnchorData `json:"anchors"`
}

type anniversaryTrackAccumulator struct {
	Track       string
	Artist      string
	PlayCount   int
	AlbumCounts map[string]int
}

type anniversaryAggregate struct {
	TotalPlays int
	Tracks     map[string]*anniversaryTrackAccumulator
}

type anniversaryDayBucket struct {
	Date      string
	Year      int
	MonthDay  string
	Aggregate *anniversaryAggregate
}

func newAnniversaryAggregate() *anniversaryAggregate {
	return &anniversaryAggregate{
		Tracks: make(map[string]*anniversaryTrackAccumulator),
	}
}

func anniversaryTrackKey(artist, track string) string {
	return artist + "\x00" + track
}

func (agg *anniversaryAggregate) addPlay(track, artist, album string) {
	if agg == nil {
		return
	}
	key := anniversaryTrackKey(artist, track)
	entry := agg.Tracks[key]
	if entry == nil {
		entry = &anniversaryTrackAccumulator{
			Track:       track,
			Artist:      artist,
			AlbumCounts: make(map[string]int),
		}
		agg.Tracks[key] = entry
	}
	entry.PlayCount++
	entry.AlbumCounts[album]++
	agg.TotalPlays++
}

func (agg *anniversaryAggregate) merge(other *anniversaryAggregate) {
	if agg == nil || other == nil {
		return
	}
	agg.TotalPlays += other.TotalPlays
	for key, otherEntry := range other.Tracks {
		entry := agg.Tracks[key]
		if entry == nil {
			entry = &anniversaryTrackAccumulator{
				Track:       otherEntry.Track,
				Artist:      otherEntry.Artist,
				AlbumCounts: make(map[string]int),
			}
			agg.Tracks[key] = entry
		}
		entry.PlayCount += otherEntry.PlayCount
		for album, count := range otherEntry.AlbumCounts {
			entry.AlbumCounts[album] += count
		}
	}
}

func (agg *anniversaryAggregate) uniqueTracks() int {
	if agg == nil {
		return 0
	}
	return len(agg.Tracks)
}

func (agg *anniversaryAggregate) topTracks(limit int) []AnniversaryTrackEntry {
	if agg == nil || len(agg.Tracks) == 0 || limit <= 0 {
		return []AnniversaryTrackEntry{}
	}
	entries := make([]AnniversaryTrackEntry, 0, len(agg.Tracks))
	for _, entry := range agg.Tracks {
		entries = append(entries, AnniversaryTrackEntry{
			Track:     entry.Track,
			Artist:    entry.Artist,
			Album:     chooseAnniversaryAlbum(entry.AlbumCounts),
			PlayCount: entry.PlayCount,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].PlayCount != entries[j].PlayCount {
			return entries[i].PlayCount > entries[j].PlayCount
		}
		leftArtist := strings.ToLower(entries[i].Artist)
		rightArtist := strings.ToLower(entries[j].Artist)
		if leftArtist != rightArtist {
			return leftArtist < rightArtist
		}
		leftTrack := strings.ToLower(entries[i].Track)
		rightTrack := strings.ToLower(entries[j].Track)
		if leftTrack != rightTrack {
			return leftTrack < rightTrack
		}
		if entries[i].Artist != entries[j].Artist {
			return entries[i].Artist < entries[j].Artist
		}
		return entries[i].Track < entries[j].Track
	})
	if len(entries) > limit {
		entries = entries[:limit]
	}
	return entries
}

func chooseAnniversaryAlbum(albumCounts map[string]int) string {
	if len(albumCounts) == 0 {
		return ""
	}
	type albumEntry struct {
		Album string
		Count int
	}
	entries := make([]albumEntry, 0, len(albumCounts))
	for album, count := range albumCounts {
		entries = append(entries, albumEntry{Album: album, Count: count})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Count != entries[j].Count {
			return entries[i].Count > entries[j].Count
		}
		leftBlank := strings.TrimSpace(entries[i].Album) == ""
		rightBlank := strings.TrimSpace(entries[j].Album) == ""
		if leftBlank != rightBlank {
			return !leftBlank
		}
		left := strings.ToLower(entries[i].Album)
		right := strings.ToLower(entries[j].Album)
		if left != right {
			return left < right
		}
		return entries[i].Album < entries[j].Album
	})
	return entries[0].Album
}

func enumerateAnniversaryMonthDays() []string {
	start := time.Date(anniversaryReferenceYear, time.January, 1, 0, 0, 0, 0, time.UTC)
	monthDays := make([]string, 0, 366)
	for cursor := start; cursor.Year() == anniversaryReferenceYear; cursor = cursor.AddDate(0, 0, 1) {
		monthDays = append(monthDays, cursor.Format(anniversaryMonthDayLayout))
	}
	return monthDays
}

func parseAnniversaryMonthDay(monthDay string) (time.Month, int, error) {
	parts := strings.Split(monthDay, "-")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid month-day %q", monthDay)
	}
	month, err := strconv.Atoi(parts[0])
	if err != nil || month < 1 || month > 12 {
		return 0, 0, fmt.Errorf("invalid month in %q", monthDay)
	}
	day, err := strconv.Atoi(parts[1])
	if err != nil || day < 1 || day > 31 {
		return 0, 0, fmt.Errorf("invalid day in %q", monthDay)
	}
	return time.Month(month), day, nil
}

func anniversaryAnchorDate(year int, monthDay string, loc *time.Location) (time.Time, bool) {
	month, day, err := parseAnniversaryMonthDay(monthDay)
	if err != nil {
		return time.Time{}, false
	}
	date := time.Date(year, month, day, 0, 0, 0, 0, loc)
	if date.Month() != month || date.Day() != day {
		return time.Time{}, false
	}
	return date, true
}

func anniversaryLabel(monthDay string) string {
	month, day, err := parseAnniversaryMonthDay(monthDay)
	if err != nil {
		return monthDay
	}
	return time.Date(anniversaryReferenceYear, month, day, 0, 0, 0, 0, time.UTC).Format("January 2")
}

func buildAnniversaryCacheData(history *ListeningHistoryData, loc *time.Location, generatedAt time.Time, topN int) (*AnniversaryCacheData, error) {
	if history == nil {
		return nil, fmt.Errorf("listening history is required")
	}
	if loc == nil {
		return nil, fmt.Errorf("timezone location is required")
	}
	if topN <= 0 {
		topN = anniversaryTopN
	}

	dayBuckets := make(map[string]*anniversaryDayBucket)
	observedYears := make(map[int]struct{})

	for _, event := range history.Events {
		track := strings.TrimSpace(event.Track)
		artist := strings.TrimSpace(event.Artist)
		if track == "" || artist == "" {
			continue
		}

		localTime := time.UnixMilli(event.Date).In(loc)
		dateKey := localTime.Format(anniversaryReferenceLayout)
		monthDay := localTime.Format(anniversaryMonthDayLayout)
		bucket := dayBuckets[dateKey]
		if bucket == nil {
			bucket = &anniversaryDayBucket{
				Date:      dateKey,
				Year:      localTime.Year(),
				MonthDay:  monthDay,
				Aggregate: newAnniversaryAggregate(),
			}
			dayBuckets[dateKey] = bucket
		}
		bucket.Aggregate.addPlay(track, artist, strings.TrimSpace(event.Album))
		observedYears[localTime.Year()] = struct{}{}
	}

	if len(dayBuckets) == 0 {
		return nil, fmt.Errorf("no listening events with track and artist names found")
	}

	years := make([]int, 0, len(observedYears))
	for year := range observedYears {
		years = append(years, year)
	}
	sort.Slice(years, func(i, j int) bool {
		return years[i] > years[j]
	})

	monthDays := enumerateAnniversaryMonthDays()
	anchors := make(map[string]AnniversaryAnchorData, len(monthDays))
	for _, monthDay := range monthDays {
		dayResults := make([]AnniversaryDayResult, 0)
		weekResults := make([]AnniversaryWeekResult, 0)

		for _, year := range years {
			anchorDate, ok := anniversaryAnchorDate(year, monthDay, loc)
			if !ok {
				continue
			}

			if bucket := dayBuckets[anchorDate.Format(anniversaryReferenceLayout)]; bucket != nil && bucket.Aggregate.TotalPlays > 0 {
				dayResults = append(dayResults, AnniversaryDayResult{
					Year:         year,
					Date:         bucket.Date,
					TotalPlays:   bucket.Aggregate.TotalPlays,
					UniqueTracks: bucket.Aggregate.uniqueTracks(),
					TopTracks:    bucket.Aggregate.topTracks(topN),
				})
			}

			weekStart := anchorDate.AddDate(0, 0, -int(anchorDate.Weekday()))
			weekEnd := weekStart.AddDate(0, 0, 6)
			weekAggregate := newAnniversaryAggregate()
			activeDays := 0
			for cursor := weekStart; !cursor.After(weekEnd); cursor = cursor.AddDate(0, 0, 1) {
				if bucket := dayBuckets[cursor.Format(anniversaryReferenceLayout)]; bucket != nil && bucket.Aggregate.TotalPlays > 0 {
					weekAggregate.merge(bucket.Aggregate)
					activeDays++
				}
			}
			if weekAggregate.TotalPlays > 0 {
				weekResults = append(weekResults, AnniversaryWeekResult{
					Year:         year,
					StartDate:    weekStart.Format(anniversaryReferenceLayout),
					EndDate:      weekEnd.Format(anniversaryReferenceLayout),
					TotalPlays:   weekAggregate.TotalPlays,
					UniqueTracks: weekAggregate.uniqueTracks(),
					ActiveDays:   activeDays,
					TopTracks:    weekAggregate.topTracks(topN),
				})
			}
		}

		anchors[monthDay] = AnniversaryAnchorData{
			Label:       anniversaryLabel(monthDay),
			DayResults:  dayResults,
			WeekResults: weekResults,
		}
	}

	return &AnniversaryCacheData{
		GeneratedAt:        generatedAt.UTC().Format(time.RFC3339),
		Timezone:           loc.String(),
		WeekStart:          anniversaryWeekStart,
		TopN:               topN,
		AvailableMonthDays: monthDays,
		Anchors:            anchors,
	}, nil
}

func runBuildAnniversaryCache() {
	fmt.Println("Building anniversaries cache from merged listening history...")

	history, err := loadListeningHistoryOrBuild()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading listening history: %v\n", err)
		os.Exit(1)
	}

	loc, err := time.LoadLocation(anniversaryTimezone)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading timezone %s: %v\n", anniversaryTimezone, err)
		os.Exit(1)
	}

	artifact, err := buildAnniversaryCacheData(history, loc, time.Now(), anniversaryTopN)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error building anniversaries cache: %v\n", err)
		os.Exit(1)
	}

	outputPath := WebDataPath("anniversary-cache.json")
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	writeJSON(outputPath, artifact)

	fmt.Printf("Anniversary cache generated successfully with %d anchors.\n", len(artifact.AvailableMonthDays))
	fmt.Printf("Timezone: %s\n", artifact.Timezone)
	fmt.Printf("Output: %s\n", outputPath)
}
