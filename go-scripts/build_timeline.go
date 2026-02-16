package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type TimelineScrobble struct {
	Track     string
	Artist    string
	Album     string
	Timestamp int64
}

type TimelineTrackEntry struct {
	Track     string `json:"track"`
	Artist    string `json:"artist"`
	Album     string `json:"album"`
	PlayCount int    `json:"playCount"`
}

type MonthData struct {
	Month          string               `json:"month"` // "2005-01"
	TotalScrobbles int                  `json:"totalScrobbles"`
	UniqueTracks   int                  `json:"uniqueTracks"`
	TopTracks      []TimelineTrackEntry `json:"topTracks"`
}

type YearData struct {
	Year           int                  `json:"year"`
	TotalScrobbles int                  `json:"totalScrobbles"`
	UniqueTracks   int                  `json:"uniqueTracks"`
	TopTracks      []TimelineTrackEntry `json:"topTracks"`
	Months         []MonthData          `json:"months"`
}

type TimelineData struct {
	FirstScrobble  int64      `json:"firstScrobble"`
	LastScrobble   int64      `json:"lastScrobble"`
	TotalScrobbles int        `json:"totalScrobbles"`
	Years          []YearData `json:"years"`
}

func runBuildTimeline() {
	outputPath := filepath.Join(ProjectRoot, "web-data", "timeline.json")

	fmt.Println("Building timeline data from merged listening history...")

	history, err := loadListeningHistoryOrBuild()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading listening history: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Processing %d listening events...\n\n", len(history.Events))

	// Group scrobbles by year and month
	yearMap := make(map[int]map[string][]TimelineScrobble)
	var firstScrobble, lastScrobble int64

	validScrobbles := 0

	for _, event := range history.Events {
		if event.Track == "" || event.Artist == "" {
			continue
		}
		validScrobbles++

		// Track first and last scrobbles
		if validScrobbles == 1 || event.Date < firstScrobble {
			firstScrobble = event.Date
		}
		if validScrobbles == 1 || event.Date > lastScrobble {
			lastScrobble = event.Date
		}

		t := time.Unix(event.Date/1000, 0)
		year := t.Year()
		month := fmt.Sprintf("%04d-%02d", year, t.Month())

		if yearMap[year] == nil {
			yearMap[year] = make(map[string][]TimelineScrobble)
		}

		yearMap[year][month] = append(yearMap[year][month], TimelineScrobble{
			Track:     event.Track,
			Artist:    event.Artist,
			Album:     event.Album,
			Timestamp: event.Date,
		})
	}

	// Build year data
	var years []YearData
	for year, months := range yearMap {
		yearData := YearData{
			Year: year,
		}

		// Track counts for the year
		yearTracks := make(map[string]*TimelineTrackEntry)
		var monthDataList []MonthData

		// Process each month
		for monthKey, scrobbles := range months {
			monthTracks := make(map[string]*TimelineTrackEntry)

			for _, s := range scrobbles {
				key := s.Artist + "|" + s.Track

				if entry, exists := monthTracks[key]; exists {
					entry.PlayCount++
				} else {
					monthTracks[key] = &TimelineTrackEntry{
						Track:     s.Track,
						Artist:    s.Artist,
						Album:     s.Album,
						PlayCount: 1,
					}
				}

				// Also count for year
				if entry, exists := yearTracks[key]; exists {
					entry.PlayCount++
				} else {
					yearTracks[key] = &TimelineTrackEntry{
						Track:     s.Track,
						Artist:    s.Artist,
						Album:     s.Album,
						PlayCount: 1,
					}
				}
			}

			// Convert month tracks to slice and sort
			var monthTopTracks []TimelineTrackEntry
			for _, entry := range monthTracks {
				monthTopTracks = append(monthTopTracks, *entry)
			}
			sort.Slice(monthTopTracks, func(i, j int) bool {
				return monthTopTracks[i].PlayCount > monthTopTracks[j].PlayCount
			})

			// Keep top 20 for month
			if len(monthTopTracks) > 20 {
				monthTopTracks = monthTopTracks[:20]
			}

			monthDataList = append(monthDataList, MonthData{
				Month:          monthKey,
				TotalScrobbles: len(scrobbles),
				UniqueTracks:   len(monthTracks),
				TopTracks:      monthTopTracks,
			})

			yearData.TotalScrobbles += len(scrobbles)
		}

		// Sort months chronologically
		sort.Slice(monthDataList, func(i, j int) bool {
			return monthDataList[i].Month < monthDataList[j].Month
		})

		// Convert year tracks to slice and sort
		var yearTopTracks []TimelineTrackEntry
		for _, entry := range yearTracks {
			yearTopTracks = append(yearTopTracks, *entry)
		}
		sort.Slice(yearTopTracks, func(i, j int) bool {
			return yearTopTracks[i].PlayCount > yearTopTracks[j].PlayCount
		})

		// Keep top 50 for year
		if len(yearTopTracks) > 50 {
			yearTopTracks = yearTopTracks[:50]
		}

		yearData.UniqueTracks = len(yearTracks)
		yearData.TopTracks = yearTopTracks
		yearData.Months = monthDataList

		years = append(years, yearData)
	}

	// Sort years
	sort.Slice(years, func(i, j int) bool {
		return years[i].Year < years[j].Year
	})

	// Build final timeline
	timeline := TimelineData{
		FirstScrobble:  firstScrobble,
		LastScrobble:   lastScrobble,
		TotalScrobbles: validScrobbles,
		Years:          years,
	}

	// Write output
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output directory: %v\n", err)
		os.Exit(1)
	}

	writeJSON(outputPath, timeline)

	// Print summary
	fmt.Println("Timeline data generated successfully!")
	fmt.Printf("Time range: %s to %s\n",
		time.Unix(firstScrobble/1000, 0).Format("2006-01-02"),
		time.Unix(lastScrobble/1000, 0).Format("2006-01-02"))
	fmt.Printf("Total years with scrobbles: %d\n", len(years))
	fmt.Printf("Output: %s\n\n", outputPath)

	// Show yearly breakdown
	fmt.Println("Yearly breakdown:")
	for _, year := range years {
		fmt.Printf("  %d: %d scrobbles, %d unique tracks\n",
			year.Year, year.TotalScrobbles, year.UniqueTracks)
	}
}
