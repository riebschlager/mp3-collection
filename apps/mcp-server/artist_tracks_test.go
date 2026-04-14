package main

import (
	"strings"
	"testing"
	"time"
)

func mustParseArtistTracksTestTime(t *testing.T, value string) int64 {
	t.Helper()

	ts, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parse time %s: %v", value, err)
	}
	return ts.UnixMilli()
}

func newArtistTracksTestResolver(tracks []resolverTrack, aliases *aliasCatalog) *trackResolver {
	if aliases == nil {
		aliases = newAliasCatalog()
	}

	resolver := &trackResolver{
		exactIndex:  map[string][]int{},
		artistIndex: map[string][]int{},
		trackIndex:  map[string][]int{},
		albumIndex:  map[string][]int{},
		aliases:     aliases,
	}

	for _, tr := range tracks {
		tr.NormArtist = aliases.CanonicalValue("artist", normalizeForMatching(tr.Artist))
		tr.NormTrack = aliases.CanonicalValue("track", normalizeForMatching(tr.Name))
		tr.NormAlbum = aliases.CanonicalValue("album", normalizeForMatching(tr.Album))
		if tr.NormArtist == "" || tr.NormTrack == "" {
			continue
		}

		idx := len(resolver.tracks)
		resolver.tracks = append(resolver.tracks, tr)

		exactKey := buildExactKey(tr.NormArtist, tr.NormTrack)
		resolver.exactIndex[exactKey] = append(resolver.exactIndex[exactKey], idx)
		resolver.artistIndex[tr.NormArtist] = append(resolver.artistIndex[tr.NormArtist], idx)
		resolver.trackIndex[tr.NormTrack] = append(resolver.trackIndex[tr.NormTrack], idx)
		if tr.NormAlbum != "" {
			resolver.albumIndex[tr.NormAlbum] = append(resolver.albumIndex[tr.NormAlbum], idx)
		}
	}

	return resolver
}

func artistTracksFixture(t *testing.T) (*trackResolver, []lastFMScrobble) {
	t.Helper()

	aliases := newAliasCatalog()
	aliases.put("artist", "apc", "a perfect circle")

	resolver := newArtistTracksTestResolver([]resolverTrack{
		{ID: "1", Artist: "A Perfect Circle", Name: "Judith", Album: "Mer de Noms"},
		{ID: "2", Artist: "A Perfect Circle", Name: "3 Libras", Album: "Mer de Noms"},
		{ID: "3", Artist: "A Perfect Circle", Name: "Magdalena", Album: "Mer de Noms"},
	}, aliases)

	scrobbles := []lastFMScrobble{
		{Artist: "APC", Track: "Judith", Album: "Mer de Noms", Source: "lastfm", Date: mustParseArtistTracksTestTime(t, "2024-01-01T10:00:00Z")},
		{Artist: "A Perfect Circle", Track: "Judith", Album: "Mer de Noms", Source: "spotify", Date: mustParseArtistTracksTestTime(t, "2024-01-02T10:00:00Z")},
		{Artist: "A Perfect Circle", Track: "Judith", Album: "Mer de Noms (Deluxe)", Source: "lastfm", Date: mustParseArtistTracksTestTime(t, "2024-01-03T10:00:00Z")},
		{Artist: "A Perfect Circle", Track: "3 Libras", Album: "Mer de Noms", Source: "lastfm", Date: mustParseArtistTracksTestTime(t, "2024-01-04T10:00:00Z")},
		{Artist: "A Perfect Circle", Track: "3 Libras", Album: "Mer de Noms", Source: "lastfm", Date: mustParseArtistTracksTestTime(t, "2024-01-05T10:00:00Z")},
		{Artist: "A Perfect Circle", Track: "Magdalena", Album: "", Source: "lastfm", Date: mustParseArtistTracksTestTime(t, "2024-01-06T10:00:00Z")},
	}

	return resolver, scrobbles
}

func TestBuildArtistTracksReportExactMatchMergesAlbumVariants(t *testing.T) {
	resolver, scrobbles := artistTracksFixture(t)

	report, err := buildArtistTracksReport("A Perfect Circle", scrobbles, resolver, nil, nil, 25)
	if err != nil {
		t.Fatalf("build artist tracks report: %v", err)
	}

	if report.Artist != "A Perfect Circle" {
		t.Fatalf("expected canonical artist display, got %s", report.Artist)
	}
	if report.TotalPlays != 6 {
		t.Fatalf("expected 6 total plays, got %d", report.TotalPlays)
	}
	if report.UniqueTracks != 3 {
		t.Fatalf("expected 3 unique tracks, got %d", report.UniqueTracks)
	}
	if len(report.Tracks) != 3 {
		t.Fatalf("expected 3 ranked tracks, got %d", len(report.Tracks))
	}

	if report.Tracks[0].Track != "Judith" || report.Tracks[0].Count != 3 {
		t.Fatalf("expected Judith with 3 plays first, got %+v", report.Tracks[0])
	}
	if report.Tracks[0].Album != "Mer de Noms" {
		t.Fatalf("expected most frequent observed album for Judith, got %s", report.Tracks[0].Album)
	}
	if report.Tracks[2].Track != "Magdalena" || report.Tracks[2].Album != "Mer de Noms" {
		t.Fatalf("expected resolver album fallback for Magdalena, got %+v", report.Tracks[2])
	}
}

func TestBuildArtistTracksReportAliasQueryAppliesSourceFilterAndTieBreak(t *testing.T) {
	resolver, scrobbles := artistTracksFixture(t)

	report, err := buildArtistTracksReport("apc", filterScrobblesBySource(scrobbles, "lastfm"), resolver, nil, nil, 2)
	if err != nil {
		t.Fatalf("build artist tracks report: %v", err)
	}

	if report.TotalPlays != 5 {
		t.Fatalf("expected 5 lastfm plays, got %d", report.TotalPlays)
	}
	if report.UniqueTracks != 3 {
		t.Fatalf("expected unique tracks to count full scoped result set, got %d", report.UniqueTracks)
	}
	if len(report.Tracks) != 2 {
		t.Fatalf("expected topN limit of 2, got %d", len(report.Tracks))
	}
	if report.Tracks[0].Track != "3 Libras" || report.Tracks[1].Track != "Judith" {
		t.Fatalf("expected alphabetical tie break for equally played tracks, got %+v", report.Tracks)
	}
}

func TestResolveArtistTracksArtistFuzzyAndAmbiguous(t *testing.T) {
	t.Run("fuzzy_match", func(t *testing.T) {
		resolver := newArtistTracksTestResolver([]resolverTrack{
			{ID: "1", Artist: "A Perfect Circle", Name: "Judith", Album: "Mer de Noms"},
			{ID: "2", Artist: "Radiohead", Name: "Paranoid Android", Album: "OK Computer"},
		}, nil)
		scrobbles := []lastFMScrobble{
			{Artist: "A Perfect Circle", Track: "Judith", Date: mustParseArtistTracksTestTime(t, "2024-01-01T10:00:00Z")},
			{Artist: "Radiohead", Track: "Paranoid Android", Date: mustParseArtistTracksTestTime(t, "2024-01-01T11:00:00Z")},
		}

		match, err := resolveArtistTracksArtist("a perfect circl", scrobbles, resolver)
		if err != nil {
			t.Fatalf("resolve fuzzy artist: %v", err)
		}
		if match.Display != "A Perfect Circle" {
			t.Fatalf("expected A Perfect Circle, got %+v", match)
		}
	})

	t.Run("ambiguous_match", func(t *testing.T) {
		resolver := newArtistTracksTestResolver([]resolverTrack{
			{ID: "1", Artist: "The Cure", Name: "Plainsong", Album: "Disintegration"},
			{ID: "2", Artist: "The Curd", Name: "Replica", Album: "Signals"},
		}, nil)
		scrobbles := []lastFMScrobble{
			{Artist: "The Cure", Track: "Plainsong", Date: mustParseArtistTracksTestTime(t, "2024-01-01T10:00:00Z")},
			{Artist: "The Curd", Track: "Replica", Date: mustParseArtistTracksTestTime(t, "2024-01-01T11:00:00Z")},
		}

		_, err := resolveArtistTracksArtist("the cur", scrobbles, resolver)
		if err == nil {
			t.Fatal("expected ambiguous match error")
		}
		if !strings.Contains(err.Error(), "ambiguous") {
			t.Fatalf("expected ambiguous error, got %v", err)
		}
	})
}

func TestParseArtistTracksPeriod(t *testing.T) {
	start, end, period, err := parseArtistTracksPeriod(map[string]interface{}{})
	if err != nil {
		t.Fatalf("parse empty period: %v", err)
	}
	if start != nil || end != nil || len(period) != 0 {
		t.Fatalf("expected empty optional range, got start=%v end=%v period=%v", start, end, period)
	}

	start, end, period, err = parseArtistTracksPeriod(map[string]interface{}{"startDate": "2024-01-03"})
	if err != nil {
		t.Fatalf("parse start-only period: %v", err)
	}
	if start == nil || end != nil || period["startDate"] != "2024-01-03" {
		t.Fatalf("expected start-only period, got start=%v end=%v period=%v", start, end, period)
	}

	start, end, period, err = parseArtistTracksPeriod(map[string]interface{}{"endDate": "2024-01-03"})
	if err != nil {
		t.Fatalf("parse end-only period: %v", err)
	}
	if start != nil || end == nil || period["endDate"] != "2024-01-03" {
		t.Fatalf("expected end-only period, got start=%v end=%v period=%v", start, end, period)
	}

	_, _, _, err = parseArtistTracksPeriod(map[string]interface{}{
		"startDate": "2024-01-05",
		"endDate":   "2024-01-03",
	})
	if err == nil {
		t.Fatal("expected reversed date range to fail")
	}
}

func TestBuildArtistTracksReportRespectsOptionalDateBounds(t *testing.T) {
	resolver, scrobbles := artistTracksFixture(t)

	start := time.Date(2024, time.January, 3, 0, 0, 0, 0, time.UTC)
	report, err := buildArtistTracksReport("A Perfect Circle", scrobbles, resolver, &start, nil, 25)
	if err != nil {
		t.Fatalf("build start-bounded report: %v", err)
	}
	if report.TotalPlays != 4 {
		t.Fatalf("expected 4 plays on or after Jan 3, got %d", report.TotalPlays)
	}
	if report.Tracks[0].Track != "3 Libras" || report.Tracks[0].Count != 2 {
		t.Fatalf("expected 3 Libras to lead after startDate filter, got %+v", report.Tracks[0])
	}

	end := time.Date(2024, time.January, 2, 0, 0, 0, 0, time.UTC)
	report, err = buildArtistTracksReport("A Perfect Circle", scrobbles, resolver, nil, &end, 25)
	if err != nil {
		t.Fatalf("build end-bounded report: %v", err)
	}
	if report.TotalPlays != 2 || report.UniqueTracks != 1 {
		t.Fatalf("expected only Judith before or on Jan 2, got %+v", report)
	}
}

func TestBuildArtistTracksReportReturnsEmptyForValidArtistOutsideWindow(t *testing.T) {
	resolver, scrobbles := artistTracksFixture(t)

	start := time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, time.February, 29, 0, 0, 0, 0, time.UTC)
	report, err := buildArtistTracksReport("A Perfect Circle", scrobbles, resolver, &start, &end, 25)
	if err != nil {
		t.Fatalf("build empty-window report: %v", err)
	}
	if report.TotalPlays != 0 || report.UniqueTracks != 0 || len(report.Tracks) != 0 {
		t.Fatalf("expected empty report for date window, got %+v", report)
	}
}
