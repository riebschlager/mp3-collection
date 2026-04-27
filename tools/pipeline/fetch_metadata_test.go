package main

import "testing"

const lastFmPlaceholderTestURL = "https://lastfm.freetls.fastly.net/i/u/300x300/2a96cbd8b46e442fc41c2b86b821562f.png"

func TestPickBestLastFmImageSkipsKnownPlaceholder(t *testing.T) {
	got := pickBestLastFmImage([]lastFmImage{
		{Size: "mega", Text: lastFmPlaceholderTestURL},
		{Size: "large", Text: "https://lastfm.freetls.fastly.net/i/u/300x300/real-cover.jpg"},
		{Size: "medium", Text: "https://lastfm.freetls.fastly.net/i/u/174s/smaller-cover.jpg"},
	})

	want := "https://lastfm.freetls.fastly.net/i/u/300x300/real-cover.jpg"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestPurgePlaceholderImagesPreservesUsableAlbumImages(t *testing.T) {
	cache := newImageCache()
	cache.Artists["artist"] = ArtistCacheEntry{
		Artist:        "Artist",
		ImageURL:      lastFmPlaceholderTestURL,
		Status:        cacheStatusOK,
		Source:        "lastfm",
		LastSuccessAt: "2026-01-01T00:00:00Z",
	}
	cache.Albums["artist|placeholder"] = AlbumCacheEntry{
		Artist:        "Artist",
		Album:         "Placeholder",
		ImageURL:      lastFmPlaceholderTestURL,
		Status:        cacheStatusOK,
		Source:        "artist_fallback:genre_like",
		LastSuccessAt: "2026-01-01T00:00:00Z",
	}
	cache.Albums["artist|real"] = AlbumCacheEntry{
		Artist:        "Artist",
		Album:         "Real",
		ImageURL:      "https://lastfm.freetls.fastly.net/i/u/300x300/real-cover.jpg",
		Status:        cacheStatusOK,
		Source:        "lastfm",
		LastSuccessAt: "2026-01-01T00:00:00Z",
	}

	artists, albums := purgePlaceholderImages(&cache)
	if artists != 1 || albums != 1 {
		t.Fatalf("expected 1 artist and 1 album placeholder purge, got artists=%d albums=%d", artists, albums)
	}
	if cache.Artists["artist"].ImageURL != "" || cache.Artists["artist"].Status != cacheStatusNotFound {
		t.Fatalf("expected artist placeholder to be cleared and marked not_found: %+v", cache.Artists["artist"])
	}
	if cache.Albums["artist|placeholder"].ImageURL != "" || cache.Albums["artist|placeholder"].Status != cacheStatusNotFound {
		t.Fatalf("expected album placeholder to be cleared and marked not_found: %+v", cache.Albums["artist|placeholder"])
	}
	if cache.Albums["artist|real"].ImageURL == "" || cache.Albums["artist|real"].Status != cacheStatusOK {
		t.Fatalf("expected usable Last.fm album image to be preserved: %+v", cache.Albums["artist|real"])
	}
}

func TestRankMusicBrainzReleaseGroupsRequiresAlbumAndArtistMatch(t *testing.T) {
	groups := []musicBrainzReleaseGroup{
		testReleaseGroup("rg-good", "Moon Safari", "Air", 95),
		testReleaseGroup("rg-wrong-artist", "Moon Safari", "Other Artist", 100),
		testReleaseGroup("rg-wrong-title", "Talkie Walkie", "Air", 100),
	}

	ranked := rankMusicBrainzReleaseGroups(groups, "Air", "Moon Safari", 1998)
	if len(ranked) != 1 {
		t.Fatalf("expected exactly one ranked release group, got %d", len(ranked))
	}
	if ranked[0].Group.ID != "rg-good" {
		t.Fatalf("expected rg-good, got %s", ranked[0].Group.ID)
	}
}

func testReleaseGroup(id, title, artist string, score int) musicBrainzReleaseGroup {
	group := musicBrainzReleaseGroup{
		ID:               id,
		Title:            title,
		Score:            score,
		PrimaryType:      "Album",
		FirstReleaseDate: "1998-01-16",
		ArtistCredit: []musicBrainzArtistCredit{
			{Name: artist},
		},
	}
	group.ArtistCredit[0].Artist.Name = artist
	return group
}
