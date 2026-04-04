package main

import "testing"

func TestBuildArchiveBrowserData(t *testing.T) {
	year1999 := 1999
	year2001 := 2001
	trackOne := 1
	trackTwo := 2

	data := buildArchiveBrowserData([]WebTrack{
		{
			ID:                "track-1",
			Name:              "First Song",
			Artist:            "The Apples",
			ArtistSlug:        "the-apples",
			Album:             "Blue Record",
			AlbumSlug:         "blue-record",
			Genre:             "Rock",
			Year:              &year1999,
			TrackNumber:       &trackOne,
			PlayCount:         7,
			DateAdded:         "12/14/01 18:18",
			Duration:          210,
			DurationFormatted: "3:30",
		},
		{
			ID:                "track-2",
			Name:              "Second Song",
			Artist:            "The Apples",
			ArtistSlug:        "the-apples",
			Album:             "Blue Record",
			AlbumSlug:         "blue-record",
			Genre:             "rock",
			Year:              &year2001,
			TrackNumber:       &trackTwo,
			PlayCount:         3,
			DateAdded:         "12/15/01 18:18",
			Duration:          180,
			DurationFormatted: "3:00",
		},
		{
			ID:                "track-3",
			Name:              "Third Song",
			Artist:            "The Berries",
			ArtistSlug:        "the-berries",
			Album:             "Golden Hour",
			AlbumSlug:         "golden-hour",
			Genre:             "",
			Duration:          120,
			DurationFormatted: "2:00",
		},
	})

	if data.Meta.TotalTracks != 3 {
		t.Fatalf("expected 3 tracks, got %d", data.Meta.TotalTracks)
	}
	if data.Meta.TotalArtists != 2 {
		t.Fatalf("expected 2 artists, got %d", data.Meta.TotalArtists)
	}
	if data.Meta.TotalAlbums != 2 {
		t.Fatalf("expected 2 albums, got %d", data.Meta.TotalAlbums)
	}
	if data.Meta.TotalGenres != 2 {
		t.Fatalf("expected 2 genres, got %d", data.Meta.TotalGenres)
	}

	if data.Genres[0].Name != "Rock" {
		t.Fatalf("expected merged genre display to prefer Rock, got %q", data.Genres[0].Name)
	}
	if data.Genres[1].Name != unknownGenreLabel {
		t.Fatalf("expected unknown genre label, got %q", data.Genres[1].Name)
	}

	if got := data.Albums[0].Year; got == nil || *got != 1999 {
		t.Fatalf("expected earliest album year 1999, got %v", got)
	}

	if data.Tracks[0].GenreSlug != "rock" {
		t.Fatalf("expected first track genre slug rock, got %q", data.Tracks[0].GenreSlug)
	}
	if data.Tracks[0].DateAddedUnix == 0 {
		t.Fatal("expected first track dateAddedUnix to be populated")
	}
	if data.Tracks[2].GenreSlug != "unknown-genre" {
		t.Fatalf("expected unknown genre slug, got %q", data.Tracks[2].GenreSlug)
	}
}

func TestParseArchiveDate(t *testing.T) {
	if got := parseArchiveDate("12/14/01 18:18"); got == 0 {
		t.Fatal("expected parsed unix timestamp for short year format")
	}
	if got := parseArchiveDate("12/14/2001 18:18"); got == 0 {
		t.Fatal("expected parsed unix timestamp for long year format")
	}
	if got := parseArchiveDate("not-a-date"); got != 0 {
		t.Fatalf("expected zero timestamp for invalid input, got %d", got)
	}
}
