package main

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

const unknownGenreLabel = "Unknown Genre"

type ArchiveBrowserMeta struct {
	TotalTracks            int    `json:"totalTracks"`
	TotalArtists           int    `json:"totalArtists"`
	TotalAlbums            int    `json:"totalAlbums"`
	TotalGenres            int    `json:"totalGenres"`
	TotalDurationSeconds   int64  `json:"totalDurationSeconds"`
	TotalDurationFormatted string `json:"totalDurationFormatted"`
}

type ArchiveBrowserGenre struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	ArtistCount int    `json:"artistCount"`
	AlbumCount  int    `json:"albumCount"`
	TrackCount  int    `json:"trackCount"`
}

type ArchiveBrowserArtist struct {
	Slug       string   `json:"slug"`
	Name       string   `json:"name"`
	AlbumCount int      `json:"albumCount"`
	TrackCount int      `json:"trackCount"`
	GenreSlugs []string `json:"genreSlugs"`
}

type ArchiveBrowserAlbum struct {
	Slug        string   `json:"slug"`
	Name        string   `json:"name"`
	ArtistSlugs []string `json:"artistSlugs"`
	GenreSlugs  []string `json:"genreSlugs"`
	TrackCount  int      `json:"trackCount"`
	Year        *int     `json:"year,omitempty"`
}

type ArchiveBrowserTrack struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	ArtistSlug        string `json:"artistSlug"`
	AlbumSlug         string `json:"albumSlug"`
	GenreSlug         string `json:"genreSlug"`
	Year              *int   `json:"year,omitempty"`
	TrackNumber       *int   `json:"trackNumber,omitempty"`
	DiscNumber        *int   `json:"discNumber,omitempty"`
	PlayCount         int    `json:"playCount"`
	DateAdded         string `json:"dateAdded,omitempty"`
	DateAddedUnix     int64  `json:"dateAddedUnix,omitempty"`
	Duration          int    `json:"duration"`
	DurationFormatted string `json:"durationFormatted"`
}

type ArchiveBrowserData struct {
	Meta    ArchiveBrowserMeta     `json:"meta"`
	Genres  []ArchiveBrowserGenre  `json:"genres"`
	Artists []ArchiveBrowserArtist `json:"artists"`
	Albums  []ArchiveBrowserAlbum  `json:"albums"`
	Tracks  []ArchiveBrowserTrack  `json:"tracks"`
}

type genreAggregate struct {
	nameVariants map[string]int
	artistSlugs  map[string]bool
	albumSlugs   map[string]bool
	trackCount   int
}

type artistAggregate struct {
	name       string
	albumSlugs map[string]bool
	genreSlugs map[string]bool
	trackCount int
}

type albumAggregate struct {
	name        string
	artistSlugs map[string]bool
	genreSlugs  map[string]bool
	trackCount  int
	year        *int
}

func buildArchiveBrowserData(tracks []WebTrack) ArchiveBrowserData {
	genreAggregates := make(map[string]*genreAggregate)
	artistAggregates := make(map[string]*artistAggregate)
	albumAggregates := make(map[string]*albumAggregate)
	browserTracks := make([]ArchiveBrowserTrack, 0, len(tracks))

	var totalDuration int64

	for _, track := range tracks {
		genreName := track.Genre
		if genreName == "" {
			genreName = unknownGenreLabel
		}
		genreSlug := Slugify(genreName)

		genreEntry, ok := genreAggregates[genreSlug]
		if !ok {
			genreEntry = &genreAggregate{
				nameVariants: make(map[string]int),
				artistSlugs:  make(map[string]bool),
				albumSlugs:   make(map[string]bool),
			}
			genreAggregates[genreSlug] = genreEntry
		}
		genreEntry.nameVariants[genreName]++
		genreEntry.artistSlugs[track.ArtistSlug] = true
		genreEntry.albumSlugs[track.AlbumSlug] = true
		genreEntry.trackCount++

		artistEntry, ok := artistAggregates[track.ArtistSlug]
		if !ok {
			artistEntry = &artistAggregate{
				name:       track.Artist,
				albumSlugs: make(map[string]bool),
				genreSlugs: make(map[string]bool),
			}
			artistAggregates[track.ArtistSlug] = artistEntry
		}
		artistEntry.albumSlugs[track.AlbumSlug] = true
		artistEntry.genreSlugs[genreSlug] = true
		artistEntry.trackCount++

		albumEntry, ok := albumAggregates[track.AlbumSlug]
		if !ok {
			albumEntry = &albumAggregate{
				name:        track.Album,
				artistSlugs: make(map[string]bool),
				genreSlugs:  make(map[string]bool),
			}
			albumAggregates[track.AlbumSlug] = albumEntry
		}
		albumEntry.artistSlugs[track.ArtistSlug] = true
		albumEntry.genreSlugs[genreSlug] = true
		albumEntry.trackCount++
		if track.Year != nil {
			if albumEntry.year == nil || *track.Year < *albumEntry.year {
				year := *track.Year
				albumEntry.year = &year
			}
		}

		dateAddedUnix := parseArchiveDate(track.DateAdded)
		browserTracks = append(browserTracks, ArchiveBrowserTrack{
			ID:                track.ID,
			Name:              track.Name,
			ArtistSlug:        track.ArtistSlug,
			AlbumSlug:         track.AlbumSlug,
			GenreSlug:         genreSlug,
			Year:              track.Year,
			TrackNumber:       track.TrackNumber,
			DiscNumber:        track.DiscNumber,
			PlayCount:         track.PlayCount,
			DateAdded:         track.DateAdded,
			DateAddedUnix:     dateAddedUnix,
			Duration:          track.Duration,
			DurationFormatted: track.DurationFormatted,
		})

		totalDuration += int64(track.Duration)
	}

	genres := make([]ArchiveBrowserGenre, 0, len(genreAggregates))
	for slug, entry := range genreAggregates {
		genres = append(genres, ArchiveBrowserGenre{
			Slug:        slug,
			Name:        preferredArchiveLabel(entry.nameVariants),
			ArtistCount: len(entry.artistSlugs),
			AlbumCount:  len(entry.albumSlugs),
			TrackCount:  entry.trackCount,
		})
	}
	sort.Slice(genres, func(i, j int) bool {
		return strings.ToLower(genres[i].Name) < strings.ToLower(genres[j].Name)
	})

	artists := make([]ArchiveBrowserArtist, 0, len(artistAggregates))
	for slug, entry := range artistAggregates {
		artists = append(artists, ArchiveBrowserArtist{
			Slug:       slug,
			Name:       entry.name,
			AlbumCount: len(entry.albumSlugs),
			TrackCount: entry.trackCount,
			GenreSlugs: sortedKeys(entry.genreSlugs),
		})
	}
	sort.Slice(artists, func(i, j int) bool {
		return strings.ToLower(artists[i].Name) < strings.ToLower(artists[j].Name)
	})

	albums := make([]ArchiveBrowserAlbum, 0, len(albumAggregates))
	for slug, entry := range albumAggregates {
		albums = append(albums, ArchiveBrowserAlbum{
			Slug:        slug,
			Name:        entry.name,
			ArtistSlugs: sortedKeys(entry.artistSlugs),
			GenreSlugs:  sortedKeys(entry.genreSlugs),
			TrackCount:  entry.trackCount,
			Year:        entry.year,
		})
	}
	sort.Slice(albums, func(i, j int) bool {
		return strings.ToLower(albums[i].Name) < strings.ToLower(albums[j].Name)
	})

	return ArchiveBrowserData{
		Meta: ArchiveBrowserMeta{
			TotalTracks:            len(browserTracks),
			TotalArtists:           len(artists),
			TotalAlbums:            len(albums),
			TotalGenres:            len(genres),
			TotalDurationSeconds:   totalDuration,
			TotalDurationFormatted: formatArchiveDuration(totalDuration),
		},
		Genres:  genres,
		Artists: artists,
		Albums:  albums,
		Tracks:  browserTracks,
	}
}

func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func preferredArchiveLabel(variants map[string]int) string {
	type scoredVariant struct {
		name  string
		count int
		score int
	}

	scored := make([]scoredVariant, 0, len(variants))
	for name, count := range variants {
		score := 0
		if name == unknownGenreLabel {
			score += 100
		}
		if strings.IndexFunc(name, func(r rune) bool { return r >= 'A' && r <= 'Z' }) >= 0 {
			score += 10
		}
		if name == strings.ToLower(name) {
			score -= 1
		}
		scored = append(scored, scoredVariant{name: name, count: count, score: score})
	}

	sort.Slice(scored, func(i, j int) bool {
		if scored[i].count != scored[j].count {
			return scored[i].count > scored[j].count
		}
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return strings.ToLower(scored[i].name) < strings.ToLower(scored[j].name)
	})

	return scored[0].name
}

func parseArchiveDate(value string) int64 {
	if value == "" {
		return 0
	}

	layouts := []string{
		"1/2/06 15:04",
		"1/2/2006 15:04",
		"1/2/06",
		"1/2/2006",
	}

	for _, layout := range layouts {
		t, err := time.ParseInLocation(layout, value, time.Local)
		if err == nil {
			return t.Unix()
		}
	}

	return 0
}

func formatArchiveDuration(seconds int64) string {
	if seconds <= 0 {
		return "0m"
	}

	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	if hours == 0 {
		return FormatDuration(int(seconds))
	}

	return strings.TrimSpace(strings.Join([]string{
		formatArchiveDurationPart(hours, "h"),
		formatArchiveDurationPart(minutes, "m"),
	}, " "))
}

func formatArchiveDurationPart(value int64, suffix string) string {
	if value == 0 {
		return ""
	}
	return fmt.Sprintf("%d%s", value, suffix)
}
