package main

import (
	"testing"
	"time"
)

func mustLoadLocation(t *testing.T, name string) *time.Location {
	t.Helper()

	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("load location %s: %v", name, err)
	}
	return loc
}

func mustParseInLocation(t *testing.T, loc *time.Location, value string) int64 {
	t.Helper()

	ts, err := time.ParseInLocation("2006-01-02 15:04", value, loc)
	if err != nil {
		t.Fatalf("parse time %s: %v", value, err)
	}
	return ts.UnixMilli()
}

func TestBuildArtistRaceDataUsesChicagoMonthBucketing(t *testing.T) {
	loc := mustLoadLocation(t, artistRaceTimezone)
	history := &ListeningHistoryData{
		Events: []ListeningEvent{
			{Artist: "Late Night", Date: mustParseInLocation(t, time.UTC, "2024-03-01 00:30")},
			{Artist: "Morning After", Date: mustParseInLocation(t, time.UTC, "2024-03-01 06:15")},
		},
	}

	artifact, err := buildArtistRaceData(history, loc, artistRaceDisplayCount, time.Unix(0, 0))
	if err != nil {
		t.Fatalf("build artist race: %v", err)
	}

	if artifact.FirstMonth != "2024-02" {
		t.Fatalf("expected first month 2024-02, got %s", artifact.FirstMonth)
	}
	if artifact.LastMonth != "2024-03" {
		t.Fatalf("expected last month 2024-03, got %s", artifact.LastMonth)
	}
	if got := artifact.Frames[0].Leaders[0].Artist; got != "Late Night" {
		t.Fatalf("expected February leader to be Late Night, got %s", got)
	}
}

func TestBuildArtistRaceDataCountsOnlyIncrease(t *testing.T) {
	loc := mustLoadLocation(t, artistRaceTimezone)
	history := &ListeningHistoryData{
		Events: []ListeningEvent{
			{Artist: "Broadcast", Date: mustParseInLocation(t, loc, "2024-01-02 08:00")},
			{Artist: "Broadcast", Date: mustParseInLocation(t, loc, "2024-02-03 09:00")},
			{Artist: "Broadcast", Date: mustParseInLocation(t, loc, "2024-03-04 10:00")},
			{Artist: "Bardo Pond", Date: mustParseInLocation(t, loc, "2024-03-05 11:00")},
		},
	}

	artifact, err := buildArtistRaceData(history, loc, artistRaceDisplayCount, time.Unix(0, 0))
	if err != nil {
		t.Fatalf("build artist race: %v", err)
	}

	previous := 0
	for _, frame := range artifact.Frames {
		for _, leader := range frame.Leaders {
			if leader.Artist == "Broadcast" {
				if leader.PlayCount < previous {
					t.Fatalf("expected cumulative count to grow, previous=%d current=%d", previous, leader.PlayCount)
				}
				previous = leader.PlayCount
			}
		}
	}
}

func TestBuildArtistRaceDataSortsTiesByArtistName(t *testing.T) {
	loc := mustLoadLocation(t, artistRaceTimezone)
	history := &ListeningHistoryData{
		Events: []ListeningEvent{
			{Artist: "Bjork", Date: mustParseInLocation(t, loc, "2024-01-02 08:00")},
			{Artist: "Autechre", Date: mustParseInLocation(t, loc, "2024-01-03 08:00")},
		},
	}

	artifact, err := buildArtistRaceData(history, loc, artistRaceDisplayCount, time.Unix(0, 0))
	if err != nil {
		t.Fatalf("build artist race: %v", err)
	}

	if len(artifact.Frames) == 0 || len(artifact.Frames[0].Leaders) < 2 {
		t.Fatalf("expected at least two leaders in first frame")
	}
	if artifact.Frames[0].Leaders[0].Artist != "Autechre" {
		t.Fatalf("expected Autechre to win alphabetical tiebreak, got %s", artifact.Frames[0].Leaders[0].Artist)
	}
}

func TestBuildArtistRaceDataIgnoresBlankArtists(t *testing.T) {
	loc := mustLoadLocation(t, artistRaceTimezone)
	history := &ListeningHistoryData{
		Events: []ListeningEvent{
			{Artist: "  ", Date: mustParseInLocation(t, loc, "2024-01-02 08:00")},
			{Artist: "Biosphere", Date: mustParseInLocation(t, loc, "2024-01-03 08:00")},
		},
	}

	artifact, err := buildArtistRaceData(history, loc, artistRaceDisplayCount, time.Unix(0, 0))
	if err != nil {
		t.Fatalf("build artist race: %v", err)
	}

	if artifact.TotalScrobbles != 1 {
		t.Fatalf("expected 1 counted scrobble, got %d", artifact.TotalScrobbles)
	}
	if got := artifact.Frames[0].Leaders[0].Artist; got != "Biosphere" {
		t.Fatalf("expected Biosphere leader, got %s", got)
	}
}

func TestBuildArtistRaceDataIncludesPartialFinalMonth(t *testing.T) {
	loc := mustLoadLocation(t, artistRaceTimezone)
	lastEvent := mustParseInLocation(t, loc, "2024-03-12 14:45")
	history := &ListeningHistoryData{
		Events: []ListeningEvent{
			{Artist: "Burial", Date: mustParseInLocation(t, loc, "2024-01-02 08:00")},
			{Artist: "Burial", Date: lastEvent},
		},
	}

	artifact, err := buildArtistRaceData(history, loc, artistRaceDisplayCount, time.Unix(0, 0))
	if err != nil {
		t.Fatalf("build artist race: %v", err)
	}

	lastFrame := artifact.Frames[len(artifact.Frames)-1]
	if lastFrame.Month != "2024-03" {
		t.Fatalf("expected final month 2024-03, got %s", lastFrame.Month)
	}
	if lastFrame.FrameEnd != lastEvent {
		t.Fatalf("expected final frame end %d, got %d", lastEvent, lastFrame.FrameEnd)
	}
}
