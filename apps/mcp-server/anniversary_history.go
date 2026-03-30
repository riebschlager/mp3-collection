package main

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultAnniversaryTimezone  = "America/Chicago"
	anniversaryWeekStartName    = "sunday"
	defaultAnniversaryTopN      = 10
	anniversaryMonthDayLayout   = "01-02"
	anniversaryDateLayout       = "2006-01-02"
	anniversaryReferenceYearMCP = 2024
)

type anniversaryTrackEntry struct {
	Track     string `json:"track"`
	Artist    string `json:"artist"`
	Album     string `json:"album"`
	PlayCount int    `json:"playCount"`
}

type anniversaryDayResult struct {
	Year         int                     `json:"year"`
	Date         string                  `json:"date"`
	TotalPlays   int                     `json:"totalPlays"`
	UniqueTracks int                     `json:"uniqueTracks"`
	TopTracks    []anniversaryTrackEntry `json:"topTracks"`
}

type anniversaryWeekResult struct {
	Year         int                     `json:"year"`
	StartDate    string                  `json:"startDate"`
	EndDate      string                  `json:"endDate"`
	TotalPlays   int                     `json:"totalPlays"`
	UniqueTracks int                     `json:"uniqueTracks"`
	ActiveDays   int                     `json:"activeDays"`
	TopTracks    []anniversaryTrackEntry `json:"topTracks"`
}

type anniversaryAnchorSelection struct {
	RequestedDate     string `json:"requestedDate"`
	RequestedMonthDay string `json:"requestedMonthDay"`
	EffectiveMonthDay string `json:"effectiveMonthDay"`
	Label             string `json:"label"`
	Timezone          string `json:"timezone"`
	WeekStart         string `json:"weekStart"`
	DefaultedToToday  bool   `json:"defaultedToToday"`
}

type anniversarySection struct {
	Label      string      `json:"label"`
	YearsFound int         `json:"yearsFound"`
	TotalPlays int         `json:"totalPlays"`
	Results    interface{} `json:"results"`
}

type anniversaryHistoryReport struct {
	Anchor            anniversaryAnchorSelection `json:"anchor"`
	Source            string                     `json:"source"`
	TopN              int                        `json:"topN"`
	TodayInHistory    anniversarySection         `json:"todayInHistory"`
	ThisWeekInHistory anniversarySection         `json:"thisWeekInHistory"`
}

type anniversaryTrackAccumulatorMCP struct {
	Track       string
	Artist      string
	PlayCount   int
	AlbumCounts map[string]int
}

type anniversaryAggregateMCP struct {
	TotalPlays int
	Tracks     map[string]*anniversaryTrackAccumulatorMCP
}

type anniversaryDayBucketMCP struct {
	Date      string
	Year      int
	Aggregate *anniversaryAggregateMCP
}

func newAnniversaryAggregateMCP() *anniversaryAggregateMCP {
	return &anniversaryAggregateMCP{
		Tracks: make(map[string]*anniversaryTrackAccumulatorMCP),
	}
}

func (agg *anniversaryAggregateMCP) addPlay(track, artist, album string) {
	if agg == nil {
		return
	}
	key := buildExactKey(normalizeForMatching(artist), normalizeForMatching(track))
	if key == "|" {
		return
	}
	entry := agg.Tracks[key]
	if entry == nil {
		entry = &anniversaryTrackAccumulatorMCP{
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

func (agg *anniversaryAggregateMCP) merge(other *anniversaryAggregateMCP) {
	if agg == nil || other == nil {
		return
	}
	agg.TotalPlays += other.TotalPlays
	for key, otherEntry := range other.Tracks {
		entry := agg.Tracks[key]
		if entry == nil {
			entry = &anniversaryTrackAccumulatorMCP{
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

func (agg *anniversaryAggregateMCP) uniqueTracks() int {
	if agg == nil {
		return 0
	}
	return len(agg.Tracks)
}

func (agg *anniversaryAggregateMCP) topTracks(limit int) []anniversaryTrackEntry {
	if agg == nil || len(agg.Tracks) == 0 || limit <= 0 {
		return []anniversaryTrackEntry{}
	}
	rows := make([]anniversaryTrackEntry, 0, len(agg.Tracks))
	for _, entry := range agg.Tracks {
		rows = append(rows, anniversaryTrackEntry{
			Track:     entry.Track,
			Artist:    entry.Artist,
			Album:     chooseAnniversaryAlbumMCP(entry.AlbumCounts),
			PlayCount: entry.PlayCount,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].PlayCount != rows[j].PlayCount {
			return rows[i].PlayCount > rows[j].PlayCount
		}
		leftArtist := strings.ToLower(rows[i].Artist)
		rightArtist := strings.ToLower(rows[j].Artist)
		if leftArtist != rightArtist {
			return leftArtist < rightArtist
		}
		leftTrack := strings.ToLower(rows[i].Track)
		rightTrack := strings.ToLower(rows[j].Track)
		if leftTrack != rightTrack {
			return leftTrack < rightTrack
		}
		if rows[i].Artist != rows[j].Artist {
			return rows[i].Artist < rows[j].Artist
		}
		return rows[i].Track < rows[j].Track
	})
	if len(rows) > limit {
		rows = rows[:limit]
	}
	return rows
}

func chooseAnniversaryAlbumMCP(albumCounts map[string]int) string {
	if len(albumCounts) == 0 {
		return ""
	}
	type albumRow struct {
		Album string
		Count int
	}
	rows := make([]albumRow, 0, len(albumCounts))
	for album, count := range albumCounts {
		rows = append(rows, albumRow{Album: album, Count: count})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Count != rows[j].Count {
			return rows[i].Count > rows[j].Count
		}
		leftBlank := strings.TrimSpace(rows[i].Album) == ""
		rightBlank := strings.TrimSpace(rows[j].Album) == ""
		if leftBlank != rightBlank {
			return !leftBlank
		}
		left := strings.ToLower(rows[i].Album)
		right := strings.ToLower(rows[j].Album)
		if left != right {
			return left < right
		}
		return rows[i].Album < rows[j].Album
	})
	return rows[0].Album
}

func musicAnniversaryHistory(args map[string]interface{}) (map[string]interface{}, error) {
	sourceFilter, err := parseSourceFilter(args)
	if err != nil {
		return nil, err
	}
	topN := defaultAnniversaryTopN
	if v, ok := asInt(args["topN"]); ok {
		if v < 1 {
			v = 1
		}
		if v > 100 {
			v = 100
		}
		topN = v
	}

	loc, tzName, err := parseTimezoneArgWithDefault(args, defaultAnniversaryTimezone)
	if err != nil {
		return nil, err
	}
	anchor, err := resolveAnniversaryAnchor(args, loc, time.Now())
	if err != nil {
		return nil, err
	}
	anchor.Timezone = tzName
	anchor.WeekStart = anniversaryWeekStartName

	scrobbles, err := getListeningScrobbles()
	if err != nil {
		return nil, fmt.Errorf("listening history unavailable: %w", err)
	}
	scrobbles = filterScrobblesBySource(scrobbles, sourceFilter)

	report, err := buildAnniversaryHistoryReport(scrobbles, anchor, topN, loc, sourceFilter)
	if err != nil {
		return nil, err
	}

	payload := map[string]interface{}{
		"anchor":            report.Anchor,
		"source":            report.Source,
		"topN":              report.TopN,
		"todayInHistory":    report.TodayInHistory,
		"thisWeekInHistory": report.ThisWeekInHistory,
	}
	return payload, nil
}

func parseTimezoneArgWithDefault(args map[string]interface{}, defaultTZ string) (*time.Location, string, error) {
	tz := strings.TrimSpace(asString(args["timezone"]))
	if tz == "" {
		tz = defaultTZ
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, "", fmt.Errorf("invalid timezone: %q", tz)
	}
	return loc, tz, nil
}

func resolveAnniversaryAnchor(args map[string]interface{}, loc *time.Location, now time.Time) (anniversaryAnchorSelection, error) {
	rawDate := asString(args["date"])
	rawMonthDay := asString(args["monthDay"])
	if rawDate != "" && rawMonthDay != "" {
		return anniversaryAnchorSelection{}, errors.New("provide either date or monthDay, not both")
	}

	if rawDate != "" {
		date, err := parseDate(rawDate)
		if err != nil {
			return anniversaryAnchorSelection{}, fmt.Errorf("date: %w", err)
		}
		monthDay := date.Format(anniversaryMonthDayLayout)
		return anniversaryAnchorSelection{
			RequestedDate:     date.Format(anniversaryDateLayout),
			EffectiveMonthDay: monthDay,
			Label:             anniversaryLabelFromMonthDay(monthDay),
		}, nil
	}

	if rawMonthDay != "" {
		monthDay, err := parseMonthDay(rawMonthDay)
		if err != nil {
			return anniversaryAnchorSelection{}, fmt.Errorf("monthDay: %w", err)
		}
		return anniversaryAnchorSelection{
			RequestedMonthDay: monthDay,
			EffectiveMonthDay: monthDay,
			Label:             anniversaryLabelFromMonthDay(monthDay),
		}, nil
	}

	effective := now.In(loc)
	monthDay := effective.Format(anniversaryMonthDayLayout)
	return anniversaryAnchorSelection{
		RequestedDate:     effective.Format(anniversaryDateLayout),
		EffectiveMonthDay: monthDay,
		Label:             anniversaryLabelFromMonthDay(monthDay),
		DefaultedToToday:  true,
	}, nil
}

func parseMonthDay(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if len(raw) != 5 || raw[2] != '-' {
		return "", errors.New("expected MM-DD")
	}
	month, err := strconv.Atoi(raw[:2])
	if err != nil || month < 1 || month > 12 {
		return "", errors.New("expected MM-DD")
	}
	day, err := strconv.Atoi(raw[3:])
	if err != nil || day < 1 || day > 31 {
		return "", errors.New("expected MM-DD")
	}
	check := time.Date(anniversaryReferenceYearMCP, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	if int(check.Month()) != month || check.Day() != day {
		return "", errors.New("expected a valid MM-DD date")
	}
	return fmt.Sprintf("%02d-%02d", month, day), nil
}

func anniversaryLabelFromMonthDay(monthDay string) string {
	month, day, err := parseMonthDayParts(monthDay)
	if err != nil {
		return monthDay
	}
	return time.Date(anniversaryReferenceYearMCP, month, day, 0, 0, 0, 0, time.UTC).Format("January 2")
}

func parseMonthDayParts(monthDay string) (time.Month, int, error) {
	parsed, err := parseMonthDay(monthDay)
	if err != nil {
		return 0, 0, err
	}
	month, _ := strconv.Atoi(parsed[:2])
	day, _ := strconv.Atoi(parsed[3:])
	return time.Month(month), day, nil
}

func anchorDateForYear(year int, monthDay string, loc *time.Location) (time.Time, bool) {
	month, day, err := parseMonthDayParts(monthDay)
	if err != nil {
		return time.Time{}, false
	}
	date := time.Date(year, month, day, 0, 0, 0, 0, loc)
	if date.Month() != month || date.Day() != day {
		return time.Time{}, false
	}
	return date, true
}

func buildAnniversaryHistoryReport(scrobbles []lastFMScrobble, anchor anniversaryAnchorSelection, topN int, loc *time.Location, source string) (anniversaryHistoryReport, error) {
	dayBuckets := make(map[string]*anniversaryDayBucketMCP)
	yearsSeen := make(map[int]struct{})

	for _, sc := range scrobbles {
		track := strings.TrimSpace(sc.Track)
		artist := strings.TrimSpace(sc.Artist)
		if track == "" || artist == "" {
			continue
		}
		localTime := time.UnixMilli(sc.Date).In(loc)
		dateKey := localTime.Format(anniversaryDateLayout)
		bucket := dayBuckets[dateKey]
		if bucket == nil {
			bucket = &anniversaryDayBucketMCP{
				Date:      dateKey,
				Year:      localTime.Year(),
				Aggregate: newAnniversaryAggregateMCP(),
			}
			dayBuckets[dateKey] = bucket
		}
		bucket.Aggregate.addPlay(track, artist, strings.TrimSpace(sc.Album))
		yearsSeen[localTime.Year()] = struct{}{}
	}

	years := make([]int, 0, len(yearsSeen))
	for year := range yearsSeen {
		years = append(years, year)
	}
	sort.Slice(years, func(i, j int) bool {
		return years[i] > years[j]
	})

	dayResults := make([]anniversaryDayResult, 0)
	weekResults := make([]anniversaryWeekResult, 0)
	totalDayPlays := 0
	totalWeekPlays := 0

	for _, year := range years {
		anchorDate, ok := anchorDateForYear(year, anchor.EffectiveMonthDay, loc)
		if !ok {
			continue
		}

		if bucket := dayBuckets[anchorDate.Format(anniversaryDateLayout)]; bucket != nil && bucket.Aggregate.TotalPlays > 0 {
			dayResults = append(dayResults, anniversaryDayResult{
				Year:         year,
				Date:         bucket.Date,
				TotalPlays:   bucket.Aggregate.TotalPlays,
				UniqueTracks: bucket.Aggregate.uniqueTracks(),
				TopTracks:    bucket.Aggregate.topTracks(topN),
			})
			totalDayPlays += bucket.Aggregate.TotalPlays
		}

		weekStart := anchorDate.AddDate(0, 0, -int(anchorDate.Weekday()))
		weekEnd := weekStart.AddDate(0, 0, 6)
		weekAggregate := newAnniversaryAggregateMCP()
		activeDays := 0
		for cursor := weekStart; !cursor.After(weekEnd); cursor = cursor.AddDate(0, 0, 1) {
			if bucket := dayBuckets[cursor.Format(anniversaryDateLayout)]; bucket != nil && bucket.Aggregate.TotalPlays > 0 {
				weekAggregate.merge(bucket.Aggregate)
				activeDays++
			}
		}
		if weekAggregate.TotalPlays > 0 {
			weekResults = append(weekResults, anniversaryWeekResult{
				Year:         year,
				StartDate:    weekStart.Format(anniversaryDateLayout),
				EndDate:      weekEnd.Format(anniversaryDateLayout),
				TotalPlays:   weekAggregate.TotalPlays,
				UniqueTracks: weekAggregate.uniqueTracks(),
				ActiveDays:   activeDays,
				TopTracks:    weekAggregate.topTracks(topN),
			})
			totalWeekPlays += weekAggregate.TotalPlays
		}
	}

	return anniversaryHistoryReport{
		Anchor: anchor,
		Source: source,
		TopN:   topN,
		TodayInHistory: anniversarySection{
			Label:      fmt.Sprintf("%s in History", anchor.Label),
			YearsFound: len(dayResults),
			TotalPlays: totalDayPlays,
			Results:    dayResults,
		},
		ThisWeekInHistory: anniversarySection{
			Label:      fmt.Sprintf("Week of %s", anchor.Label),
			YearsFound: len(weekResults),
			TotalPlays: totalWeekPlays,
			Results:    weekResults,
		},
	}, nil
}
