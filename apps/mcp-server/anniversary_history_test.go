package main

import (
	"testing"
	"time"
)

func mustLoadMCPTestLocation(t *testing.T, name string) *time.Location {
	t.Helper()

	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("load location %s: %v", name, err)
	}
	return loc
}

func mustParseMCPTestTime(t *testing.T, loc *time.Location, value string) int64 {
	t.Helper()

	ts, err := time.ParseInLocation("2006-01-02 15:04", value, loc)
	if err != nil {
		t.Fatalf("parse time %s: %v", value, err)
	}
	return ts.UnixMilli()
}

func TestResolveAnniversaryAnchorDefaultsToChicagoToday(t *testing.T) {
	loc := mustLoadMCPTestLocation(t, defaultAnniversaryTimezone)
	now := time.Date(2024, time.March, 1, 4, 30, 0, 0, time.UTC)

	anchor, err := resolveAnniversaryAnchor(map[string]interface{}{}, loc, now)
	if err != nil {
		t.Fatalf("resolve anchor: %v", err)
	}

	if !anchor.DefaultedToToday {
		t.Fatal("expected anchor to default to today")
	}
	if anchor.RequestedDate != "2024-02-29" {
		t.Fatalf("expected Chicago date 2024-02-29, got %s", anchor.RequestedDate)
	}
	if anchor.EffectiveMonthDay != "02-29" {
		t.Fatalf("expected leap-day month/day, got %s", anchor.EffectiveMonthDay)
	}
}

func TestResolveAnniversaryAnchorRejectsBothDateAndMonthDay(t *testing.T) {
	loc := mustLoadMCPTestLocation(t, defaultAnniversaryTimezone)
	_, err := resolveAnniversaryAnchor(map[string]interface{}{
		"date":     "2024-03-29",
		"monthDay": "03-29",
	}, loc, time.Now())
	if err == nil {
		t.Fatal("expected validation error when both date and monthDay are set")
	}
}

func TestBuildAnniversaryHistoryReportWeekCrossesYearBoundary(t *testing.T) {
	loc := mustLoadMCPTestLocation(t, defaultAnniversaryTimezone)
	scrobbles := []lastFMScrobble{
		{Track: "Year End", Artist: "Broadcast", Date: mustParseMCPTestTime(t, loc, "2023-12-31 10:00")},
		{Track: "New Start", Artist: "Broadcast", Date: mustParseMCPTestTime(t, loc, "2024-01-02 09:00")},
		{Track: "Next Sunday", Artist: "Broadcast", Date: mustParseMCPTestTime(t, loc, "2024-01-07 08:00")},
	}

	report, err := buildAnniversaryHistoryReport(scrobbles, anniversaryAnchorSelection{
		EffectiveMonthDay: "01-01",
		Label:             "January 1",
		Timezone:          defaultAnniversaryTimezone,
		WeekStart:         anniversaryWeekStartName,
	}, defaultAnniversaryTopN, loc, "all")
	if err != nil {
		t.Fatalf("build anniversary report: %v", err)
	}

	if report.ThisWeekInHistory.YearsFound != 1 {
		t.Fatalf("expected 1 week result, got %d", report.ThisWeekInHistory.YearsFound)
	}
	results := report.ThisWeekInHistory.Results.([]anniversaryWeekResult)
	if results[0].StartDate != "2023-12-31" || results[0].EndDate != "2024-01-06" {
		t.Fatalf("expected cross-year week window, got %s to %s", results[0].StartDate, results[0].EndDate)
	}
	if results[0].TotalPlays != 2 {
		t.Fatalf("expected 2 plays in boundary week, got %d", results[0].TotalPlays)
	}
}

func TestBuildAnniversaryHistoryReportSortsTrackTiesAndFiltersLeapDay(t *testing.T) {
	loc := mustLoadMCPTestLocation(t, defaultAnniversaryTimezone)
	scrobbles := []lastFMScrobble{
		{Track: "Hyperballad", Artist: "Bjork", Date: mustParseMCPTestTime(t, loc, "2024-02-29 08:00")},
		{Track: "Clipper", Artist: "Autechre", Date: mustParseMCPTestTime(t, loc, "2024-02-29 09:00")},
		{Track: "Not Leap", Artist: "Boards of Canada", Date: mustParseMCPTestTime(t, loc, "2023-02-28 11:00")},
	}

	report, err := buildAnniversaryHistoryReport(scrobbles, anniversaryAnchorSelection{
		EffectiveMonthDay: "02-29",
		Label:             "February 29",
		Timezone:          defaultAnniversaryTimezone,
		WeekStart:         anniversaryWeekStartName,
	}, defaultAnniversaryTopN, loc, "all")
	if err != nil {
		t.Fatalf("build anniversary report: %v", err)
	}

	if report.TodayInHistory.YearsFound != 1 {
		t.Fatalf("expected leap-day-only results, got %d", report.TodayInHistory.YearsFound)
	}
	dayResults := report.TodayInHistory.Results.([]anniversaryDayResult)
	if dayResults[0].Year != 2024 {
		t.Fatalf("expected 2024 leap-day result, got %d", dayResults[0].Year)
	}
	if len(dayResults[0].TopTracks) < 2 {
		t.Fatalf("expected two tied tracks, got %d", len(dayResults[0].TopTracks))
	}
	if dayResults[0].TopTracks[0].Artist != "Autechre" {
		t.Fatalf("expected Autechre to win alphabetical tiebreak, got %s", dayResults[0].TopTracks[0].Artist)
	}
}

func TestMusicAnniversaryHistoryAppliesSourceFilter(t *testing.T) {
	loc := mustLoadMCPTestLocation(t, defaultAnniversaryTimezone)
	scrobbles := []lastFMScrobble{
		{Track: "Lastfm Song", Artist: "Burial", Source: "lastfm", Date: mustParseMCPTestTime(t, loc, "2024-03-29 08:00")},
		{Track: "Spotify Song", Artist: "Burial", Source: "spotify", Date: mustParseMCPTestTime(t, loc, "2024-03-29 09:00")},
	}

	report, err := buildAnniversaryHistoryReport(filterScrobblesBySource(scrobbles, "spotify"), anniversaryAnchorSelection{
		EffectiveMonthDay: "03-29",
		Label:             "March 29",
		Timezone:          defaultAnniversaryTimezone,
		WeekStart:         anniversaryWeekStartName,
	}, defaultAnniversaryTopN, loc, "spotify")
	if err != nil {
		t.Fatalf("build anniversary report: %v", err)
	}

	if report.TodayInHistory.YearsFound != 1 {
		t.Fatalf("expected one spotify result, got %d", report.TodayInHistory.YearsFound)
	}
	dayResults := report.TodayInHistory.Results.([]anniversaryDayResult)
	if got := dayResults[0].TopTracks[0].Track; got != "Spotify Song" {
		t.Fatalf("expected spotify song, got %s", got)
	}
	if report.Source != "spotify" {
		t.Fatalf("expected spotify source, got %s", report.Source)
	}
}
