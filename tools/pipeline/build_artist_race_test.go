package main

import (
	"fmt"
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

func appendArtistEvents(t *testing.T, events []ListeningEvent, loc *time.Location, artist, start string, count int) []ListeningEvent {
	t.Helper()

	base, err := time.ParseInLocation("2006-01-02 15:04", start, loc)
	if err != nil {
		t.Fatalf("parse artist event time %s: %v", start, err)
	}

	for index := 0; index < count; index++ {
		events = append(events, ListeningEvent{
			Artist: artist,
			Track:  fmt.Sprintf("%s-%02d", artist, index+1),
			Date:   base.Add(time.Duration(index) * time.Minute).UnixMilli(),
		})
	}

	return events
}

func mustBuildArtistRaceArtifacts(t *testing.T, history *ListeningHistoryData) (*ArtistRaceManifest, []artistRaceVariantOutput) {
	t.Helper()

	loc := mustLoadLocation(t, artistRaceTimezone)
	manifest, outputs, err := buildArtistRaceArtifacts(history, loc, artistRaceDisplayCount, time.Unix(0, 0))
	if err != nil {
		t.Fatalf("build artist race artifacts: %v", err)
	}
	return manifest, outputs
}

func findArtistRaceVariant(t *testing.T, outputs []artistRaceVariantOutput, granularity, windowKey string) *ArtistRaceVariantData {
	t.Helper()

	wantPath := "artist-race/" + granularity + "-" + windowKey + ".json"
	for _, output := range outputs {
		if output.Path == wantPath {
			return output.Data
		}
	}

	t.Fatalf("variant not found: %s %s", granularity, windowKey)
	return nil
}

func findArtistRaceLeader(frame ArtistRaceFrame, artist string) *ArtistRaceLeader {
	for index := range frame.Leaders {
		if frame.Leaders[index].Artist == artist {
			return &frame.Leaders[index]
		}
	}
	return nil
}

func TestBuildArtistRaceArtifactsUsesChicagoISOWeekBucketing(t *testing.T) {
	loc := mustLoadLocation(t, artistRaceTimezone)
	history := &ListeningHistoryData{
		Events: []ListeningEvent{
			{Artist: "Late Sunday", Date: mustParseInLocation(t, loc, "2023-12-31 22:30")},
			{Artist: "New Monday", Date: mustParseInLocation(t, loc, "2024-01-01 09:00")},
		},
	}

	_, outputs := mustBuildArtistRaceArtifacts(t, history)
	variant := findArtistRaceVariant(t, outputs, "week", "all")

	if variant.TotalFrames != 2 {
		t.Fatalf("expected 2 weekly frames, got %d", variant.TotalFrames)
	}
	if variant.Frames[0].Key != "2023-W52" {
		t.Fatalf("expected first weekly frame 2023-W52, got %s", variant.Frames[0].Key)
	}
	if variant.Frames[1].Key != "2024-W01" {
		t.Fatalf("expected second weekly frame 2024-W01, got %s", variant.Frames[1].Key)
	}
	if got := variant.Frames[0].Leaders[0].Artist; got != "Late Sunday" {
		t.Fatalf("expected first week leader Late Sunday, got %s", got)
	}
}

func TestBuildArtistRaceArtifactsKeepsCumulativeCountsMonotonic(t *testing.T) {
	loc := mustLoadLocation(t, artistRaceTimezone)
	history := &ListeningHistoryData{
		Events: []ListeningEvent{
			{Artist: "Broadcast", Date: mustParseInLocation(t, loc, "2024-01-02 08:00")},
			{Artist: "Broadcast", Date: mustParseInLocation(t, loc, "2024-02-03 09:00")},
			{Artist: "Broadcast", Date: mustParseInLocation(t, loc, "2024-03-04 10:00")},
			{Artist: "Bardo Pond", Date: mustParseInLocation(t, loc, "2024-03-05 11:00")},
		},
	}

	_, outputs := mustBuildArtistRaceArtifacts(t, history)
	variant := findArtistRaceVariant(t, outputs, "month", "all")

	previous := 0
	for _, frame := range variant.Frames {
		for _, leader := range frame.Leaders {
			if leader.Artist != "Broadcast" {
				continue
			}
			if leader.PlayCount < previous {
				t.Fatalf("expected cumulative count to grow, previous=%d current=%d", previous, leader.PlayCount)
			}
			previous = leader.PlayCount
		}
	}
}

func TestBuildArtistRaceArtifactsDropExpiredTrailingWindowEvents(t *testing.T) {
	loc := mustLoadLocation(t, artistRaceTimezone)
	history := &ListeningHistoryData{
		Events: []ListeningEvent{
			{Artist: "Broadcast", Date: mustParseInLocation(t, loc, "2024-01-01 09:00")},
			{Artist: "Broadcast", Date: mustParseInLocation(t, loc, "2024-01-08 09:00")},
			{Artist: "Burial", Date: mustParseInLocation(t, loc, "2024-02-12 09:00")},
		},
	}

	_, outputs := mustBuildArtistRaceArtifacts(t, history)
	variant := findArtistRaceVariant(t, outputs, "week", "4w")
	lastFrame := variant.Frames[len(variant.Frames)-1]

	if lastFrame.Key != "2024-W07" {
		t.Fatalf("expected final weekly frame 2024-W07, got %s", lastFrame.Key)
	}
	if lastFrame.WindowTotalScrobbles != 1 {
		t.Fatalf("expected 1 scrobble in trailing window, got %d", lastFrame.WindowTotalScrobbles)
	}
	if len(lastFrame.Leaders) != 1 || lastFrame.Leaders[0].Artist != "Burial" {
		t.Fatalf("expected Burial to be the only trailing-window leader, got %+v", lastFrame.Leaders)
	}
}

func TestBuildArtistRaceArtifactsIncludeEmptyPeriodsBetweenEvents(t *testing.T) {
	loc := mustLoadLocation(t, artistRaceTimezone)
	history := &ListeningHistoryData{
		Events: []ListeningEvent{
			{Artist: "Biosphere", Date: mustParseInLocation(t, loc, "2024-01-02 08:00")},
			{Artist: "Biosphere", Date: mustParseInLocation(t, loc, "2024-03-14 08:00")},
		},
	}

	_, outputs := mustBuildArtistRaceArtifacts(t, history)
	variant := findArtistRaceVariant(t, outputs, "month", "all")

	if variant.TotalFrames != 3 {
		t.Fatalf("expected 3 monthly frames with empty February included, got %d", variant.TotalFrames)
	}
	if variant.Frames[1].Key != "2024-02" {
		t.Fatalf("expected middle monthly frame 2024-02, got %s", variant.Frames[1].Key)
	}
	if variant.Frames[1].Leaders[0].PlayCount != 1 {
		t.Fatalf("expected February cumulative leader count to remain 1, got %d", variant.Frames[1].Leaders[0].PlayCount)
	}
}

func TestBuildArtistRaceArtifactsIncludePartialFinalFrameEnd(t *testing.T) {
	loc := mustLoadLocation(t, artistRaceTimezone)
	lastEvent := mustParseInLocation(t, loc, "2024-03-12 14:45")
	history := &ListeningHistoryData{
		Events: []ListeningEvent{
			{Artist: "Burial", Date: mustParseInLocation(t, loc, "2024-01-02 08:00")},
			{Artist: "Burial", Date: lastEvent},
		},
	}

	_, outputs := mustBuildArtistRaceArtifacts(t, history)
	monthVariant := findArtistRaceVariant(t, outputs, "month", "all")
	weekVariant := findArtistRaceVariant(t, outputs, "week", "all")

	if got := monthVariant.Frames[len(monthVariant.Frames)-1].FrameEnd; got != lastEvent {
		t.Fatalf("expected final monthly frame end %d, got %d", lastEvent, got)
	}
	if got := weekVariant.Frames[len(weekVariant.Frames)-1].FrameEnd; got != lastEvent {
		t.Fatalf("expected final weekly frame end %d, got %d", lastEvent, got)
	}
}

func TestBuildArtistRaceArtifactsSortTiesAlphabetically(t *testing.T) {
	loc := mustLoadLocation(t, artistRaceTimezone)
	history := &ListeningHistoryData{
		Events: []ListeningEvent{
			{Artist: "Bjork", Date: mustParseInLocation(t, loc, "2024-01-02 08:00")},
			{Artist: "Autechre", Date: mustParseInLocation(t, loc, "2024-01-03 08:00")},
		},
	}

	_, outputs := mustBuildArtistRaceArtifacts(t, history)
	variant := findArtistRaceVariant(t, outputs, "week", "all")

	if len(variant.Frames) == 0 || len(variant.Frames[0].Leaders) < 2 {
		t.Fatalf("expected at least two leaders in first frame")
	}
	if variant.Frames[0].Leaders[0].Artist != "Autechre" {
		t.Fatalf("expected Autechre to win alphabetical tiebreak, got %s", variant.Frames[0].Leaders[0].Artist)
	}
}

func TestBuildArtistRaceArtifactsKeepStickyWeeklyOrderWithinMargin(t *testing.T) {
	loc := mustLoadLocation(t, artistRaceTimezone)
	events := []ListeningEvent{}
	events = appendArtistEvents(t, events, loc, "Autechre", "2024-01-01 09:00", 10)
	events = appendArtistEvents(t, events, loc, "Burial", "2024-01-08 09:00", 14)
	history := &ListeningHistoryData{Events: events}

	_, outputs := mustBuildArtistRaceArtifacts(t, history)
	variant := findArtistRaceVariant(t, outputs, "week", "13w")
	if variant.OrderingMode != "sticky-hysteresis" {
		t.Fatalf("expected sticky ordering mode, got %s", variant.OrderingMode)
	}
	if variant.OrderingMarginPlays != artistRaceStickyPromotionMargin {
		t.Fatalf("expected sticky ordering margin %d, got %d", artistRaceStickyPromotionMargin, variant.OrderingMarginPlays)
	}
	if len(variant.Frames) < 2 {
		t.Fatalf("expected at least two weekly frames, got %d", len(variant.Frames))
	}

	frame := variant.Frames[1]
	if frame.Leaders[0].Artist != "Autechre" {
		t.Fatalf("expected Autechre to stay ahead within sticky margin, got %s", frame.Leaders[0].Artist)
	}

	autechre := findArtistRaceLeader(frame, "Autechre")
	burial := findArtistRaceLeader(frame, "Burial")
	if autechre == nil || burial == nil {
		t.Fatalf("expected both Autechre and Burial in frame leaders: %+v", frame.Leaders)
	}
	if autechre.Rank != 1 || autechre.RawRank != 2 {
		t.Fatalf("expected Autechre display/raw ranks 1/2, got %+v", *autechre)
	}
	if burial.Rank != 2 || burial.RawRank != 1 {
		t.Fatalf("expected Burial display/raw ranks 2/1, got %+v", *burial)
	}
}

func TestBuildArtistRaceArtifactsPromoteStickyWeeklyOrderAtMargin(t *testing.T) {
	loc := mustLoadLocation(t, artistRaceTimezone)
	events := []ListeningEvent{}
	events = appendArtistEvents(t, events, loc, "Autechre", "2024-01-01 09:00", 10)
	events = appendArtistEvents(t, events, loc, "Burial", "2024-01-08 09:00", 14)
	events = appendArtistEvents(t, events, loc, "Burial", "2024-01-15 09:00", 1)
	history := &ListeningHistoryData{Events: events}

	_, outputs := mustBuildArtistRaceArtifacts(t, history)
	variant := findArtistRaceVariant(t, outputs, "week", "13w")
	if len(variant.Frames) < 3 {
		t.Fatalf("expected at least three weekly frames, got %d", len(variant.Frames))
	}

	frame := variant.Frames[2]
	if frame.Leaders[0].Artist != "Burial" {
		t.Fatalf("expected Burial to promote after clearing sticky margin, got %s", frame.Leaders[0].Artist)
	}

	burial := findArtistRaceLeader(frame, "Burial")
	if burial == nil || burial.Rank != 1 || burial.RawRank != 1 {
		t.Fatalf("expected Burial display/raw ranks 1/1, got %+v", burial)
	}
}

func TestBuildArtistRaceArtifactsOrderingMetadataMatchesVariantStrategy(t *testing.T) {
	loc := mustLoadLocation(t, artistRaceTimezone)
	history := &ListeningHistoryData{
		Events: []ListeningEvent{
			{Artist: "Biosphere", Date: mustParseInLocation(t, loc, "2024-01-02 08:00")},
			{Artist: "Burial", Date: mustParseInLocation(t, loc, "2024-02-14 08:00")},
		},
	}

	_, outputs := mustBuildArtistRaceArtifacts(t, history)
	weeklyTrailing := findArtistRaceVariant(t, outputs, "week", "13w")
	weeklyAll := findArtistRaceVariant(t, outputs, "week", "all")
	monthlyTrailing := findArtistRaceVariant(t, outputs, "month", "13w")

	if weeklyTrailing.OrderingMode != "sticky-hysteresis" || weeklyTrailing.OrderingMarginPlays != artistRaceStickyPromotionMargin {
		t.Fatalf("expected weekly trailing variant to use sticky ordering, got mode=%s margin=%d", weeklyTrailing.OrderingMode, weeklyTrailing.OrderingMarginPlays)
	}
	if weeklyAll.OrderingMode != "exact" || weeklyAll.OrderingMarginPlays != 0 {
		t.Fatalf("expected weekly all-time variant to use exact ordering, got mode=%s margin=%d", weeklyAll.OrderingMode, weeklyAll.OrderingMarginPlays)
	}
	if monthlyTrailing.OrderingMode != "exact" || monthlyTrailing.OrderingMarginPlays != 0 {
		t.Fatalf("expected monthly trailing variant to use exact ordering, got mode=%s margin=%d", monthlyTrailing.OrderingMode, monthlyTrailing.OrderingMarginPlays)
	}
}

func TestBuildArtistRaceArtifactsManifestIncludesExpectedVariants(t *testing.T) {
	loc := mustLoadLocation(t, artistRaceTimezone)
	history := &ListeningHistoryData{
		Events: []ListeningEvent{
			{Artist: "Biosphere", Date: mustParseInLocation(t, loc, "2024-01-02 08:00")},
			{Artist: "Burial", Date: mustParseInLocation(t, loc, "2024-03-14 08:00")},
		},
	}

	manifest, outputs := mustBuildArtistRaceArtifacts(t, history)

	if manifest.DefaultGranularity != artistRaceDefaultGranularity {
		t.Fatalf("expected default granularity %s, got %s", artistRaceDefaultGranularity, manifest.DefaultGranularity)
	}
	if manifest.DefaultWindowKey != artistRaceDefaultWindowKey {
		t.Fatalf("expected default window key %s, got %s", artistRaceDefaultWindowKey, manifest.DefaultWindowKey)
	}
	if len(manifest.Variants) != len(artistRaceGranularities)*len(artistRaceWindowConfigs) {
		t.Fatalf("expected %d variants, got %d", len(artistRaceGranularities)*len(artistRaceWindowConfigs), len(manifest.Variants))
	}
	if len(outputs) != len(manifest.Variants) {
		t.Fatalf("expected %d outputs, got %d", len(manifest.Variants), len(outputs))
	}

	week13Found := false
	for _, variant := range manifest.Variants {
		if variant.Granularity == "week" && variant.WindowKey == "13w" {
			week13Found = true
			if variant.Path != "artist-race/week-13w.json" {
				t.Fatalf("expected week-13w path, got %s", variant.Path)
			}
			if variant.FirstFrameKey == "" || variant.LastFrameKey == "" {
				t.Fatalf("expected populated frame keys in manifest entry: %+v", variant)
			}
		}
	}

	if !week13Found {
		t.Fatalf("expected week/13w variant in manifest")
	}
}
