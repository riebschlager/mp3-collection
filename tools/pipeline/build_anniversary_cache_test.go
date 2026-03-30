package main

import (
	"testing"
	"time"
)

func buildAnniversaryFixture(t *testing.T, history *ListeningHistoryData) *AnniversaryCacheData {
	t.Helper()

	loc := mustLoadLocation(t, anniversaryTimezone)
	artifact, err := buildAnniversaryCacheData(history, loc, time.Unix(0, 0), anniversaryTopN)
	if err != nil {
		t.Fatalf("build anniversaries cache: %v", err)
	}
	return artifact
}

func anniversaryAnchor(t *testing.T, artifact *AnniversaryCacheData, monthDay string) AnniversaryAnchorData {
	t.Helper()

	anchor, ok := artifact.Anchors[monthDay]
	if !ok {
		t.Fatalf("missing anchor %s", monthDay)
	}
	return anchor
}

func TestBuildAnniversaryCacheUsesChicagoDayBucketing(t *testing.T) {
	history := &ListeningHistoryData{
		Events: []ListeningEvent{
			{Track: "Late Night", Artist: "Burial", Date: mustParseInLocation(t, time.UTC, "2024-03-01 00:30")},
			{Track: "Morning After", Artist: "Burial", Date: mustParseInLocation(t, time.UTC, "2024-03-01 06:15")},
		},
	}

	artifact := buildAnniversaryFixture(t, history)
	febLeap := anniversaryAnchor(t, artifact, "02-29")
	marchStart := anniversaryAnchor(t, artifact, "03-01")

	if len(febLeap.DayResults) != 1 {
		t.Fatalf("expected one leap-day result, got %d", len(febLeap.DayResults))
	}
	if got := febLeap.DayResults[0].Date; got != "2024-02-29" {
		t.Fatalf("expected 2024-02-29, got %s", got)
	}
	if got := febLeap.DayResults[0].TopTracks[0].Track; got != "Late Night" {
		t.Fatalf("expected Late Night on leap day, got %s", got)
	}

	if len(marchStart.DayResults) != 1 {
		t.Fatalf("expected one March 1 result, got %d", len(marchStart.DayResults))
	}
	if got := marchStart.DayResults[0].TopTracks[0].Track; got != "Morning After" {
		t.Fatalf("expected Morning After on March 1, got %s", got)
	}
}

func TestBuildAnniversaryCacheWeekCanCrossYearBoundary(t *testing.T) {
	loc := mustLoadLocation(t, anniversaryTimezone)
	history := &ListeningHistoryData{
		Events: []ListeningEvent{
			{Track: "Year End", Artist: "Broadcast", Date: mustParseInLocation(t, loc, "2023-12-31 10:00")},
			{Track: "New Start", Artist: "Broadcast", Date: mustParseInLocation(t, loc, "2024-01-02 09:00")},
			{Track: "Next Sunday", Artist: "Broadcast", Date: mustParseInLocation(t, loc, "2024-01-07 08:00")},
		},
	}

	artifact := buildAnniversaryFixture(t, history)
	anchor := anniversaryAnchor(t, artifact, "01-01")

	if len(anchor.WeekResults) == 0 {
		t.Fatalf("expected week results for 01-01")
	}
	result := anchor.WeekResults[0]
	if result.Year != 2024 {
		t.Fatalf("expected anchor year 2024, got %d", result.Year)
	}
	if result.StartDate != "2023-12-31" || result.EndDate != "2024-01-06" {
		t.Fatalf("expected cross-year week range, got %s to %s", result.StartDate, result.EndDate)
	}
	if result.TotalPlays != 2 {
		t.Fatalf("expected 2 plays in anchor week, got %d", result.TotalPlays)
	}
	if result.ActiveDays != 2 {
		t.Fatalf("expected 2 active days, got %d", result.ActiveDays)
	}
	if len(result.TopTracks) != 2 {
		t.Fatalf("expected 2 top tracks, got %d", len(result.TopTracks))
	}
}

func TestBuildAnniversaryCacheSortsTrackTiesDeterministically(t *testing.T) {
	loc := mustLoadLocation(t, anniversaryTimezone)
	history := &ListeningHistoryData{
		Events: []ListeningEvent{
			{Track: "Hyperballad", Artist: "Bjork", Date: mustParseInLocation(t, loc, "2024-01-02 08:00")},
			{Track: "Clipper", Artist: "Autechre", Date: mustParseInLocation(t, loc, "2024-01-02 09:00")},
		},
	}

	artifact := buildAnniversaryFixture(t, history)
	anchor := anniversaryAnchor(t, artifact, "01-02")

	if len(anchor.DayResults) != 1 {
		t.Fatalf("expected one day result, got %d", len(anchor.DayResults))
	}
	topTracks := anchor.DayResults[0].TopTracks
	if len(topTracks) < 2 {
		t.Fatalf("expected two tied tracks, got %d", len(topTracks))
	}
	if topTracks[0].Artist != "Autechre" {
		t.Fatalf("expected Autechre to win alphabetical tie, got %s", topTracks[0].Artist)
	}
	if topTracks[1].Artist != "Bjork" {
		t.Fatalf("expected Bjork second, got %s", topTracks[1].Artist)
	}
}

func TestBuildAnniversaryCacheKeepsLeapDayDistinct(t *testing.T) {
	loc := mustLoadLocation(t, anniversaryTimezone)
	history := &ListeningHistoryData{
		Events: []ListeningEvent{
			{Track: "Leap Song", Artist: "The Clientele", Date: mustParseInLocation(t, loc, "2024-02-29 12:00")},
			{Track: "Regular Song", Artist: "The Clientele", Date: mustParseInLocation(t, loc, "2023-02-28 12:00")},
		},
	}

	artifact := buildAnniversaryFixture(t, history)
	anchor := anniversaryAnchor(t, artifact, "02-29")

	foundLeapDay := false
	for _, monthDay := range artifact.AvailableMonthDays {
		if monthDay == "02-29" {
			foundLeapDay = true
			break
		}
	}
	if !foundLeapDay {
		t.Fatal("expected available month-days to include 02-29")
	}

	if len(anchor.DayResults) != 1 {
		t.Fatalf("expected only one leap-day result, got %d", len(anchor.DayResults))
	}
	if anchor.DayResults[0].Year != 2024 {
		t.Fatalf("expected leap-day result for 2024, got %d", anchor.DayResults[0].Year)
	}
	if len(anchor.WeekResults) != 1 || anchor.WeekResults[0].Year != 2024 {
		t.Fatalf("expected only leap-year week results, got %+v", anchor.WeekResults)
	}
}

func TestBuildAnniversaryCacheIncludesEmptyAnchors(t *testing.T) {
	loc := mustLoadLocation(t, anniversaryTimezone)
	history := &ListeningHistoryData{
		Events: []ListeningEvent{
			{Track: "Only Track", Artist: "Biosphere", Date: mustParseInLocation(t, loc, "2024-01-01 12:00")},
		},
	}

	artifact := buildAnniversaryFixture(t, history)
	anchor := anniversaryAnchor(t, artifact, "03-15")

	if len(anchor.DayResults) != 0 {
		t.Fatalf("expected empty day results for 03-15, got %d", len(anchor.DayResults))
	}
	if len(anchor.WeekResults) != 0 {
		t.Fatalf("expected empty week results for 03-15, got %d", len(anchor.WeekResults))
	}
}
