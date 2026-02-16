package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	lastFmBaseURL = "https://ws.audioscrobbler.com/2.0/"

	cacheStatusOK       = "ok"
	cacheStatusNotFound = "not_found"
	cacheStatusError    = "error"
)

type ImageCache struct {
	Version   int                         `json:"version"`
	UpdatedAt string                      `json:"updatedAt"`
	Artists   map[string]ArtistCacheEntry `json:"artists"`
	Albums    map[string]AlbumCacheEntry  `json:"albums"`
}

type ArtistCacheEntry struct {
	Artist        string `json:"artist"`
	ImageURL      string `json:"imageUrl,omitempty"`
	Status        string `json:"status"`
	Source        string `json:"source,omitempty"`
	Attempts      int    `json:"attempts"`
	LastAttemptAt string `json:"lastAttemptAt,omitempty"`
	LastSuccessAt string `json:"lastSuccessAt,omitempty"`
	ErrorMessage  string `json:"errorMessage,omitempty"`
}

type AlbumCacheEntry struct {
	Artist        string `json:"artist"`
	Album         string `json:"album"`
	MBID          string `json:"mbid,omitempty"`
	ImageURL      string `json:"imageUrl,omitempty"`
	Status        string `json:"status"`
	Source        string `json:"source,omitempty"`
	Attempts      int    `json:"attempts"`
	LastAttemptAt string `json:"lastAttemptAt,omitempty"`
	LastSuccessAt string `json:"lastSuccessAt,omitempty"`
	ErrorMessage  string `json:"errorMessage,omitempty"`
}

type ArtistImagesOutput struct {
	UpdatedAt    string            `json:"updatedAt"`
	Total        int               `json:"total"`
	ByArtistSlug map[string]string `json:"byArtistSlug"`
}

type AlbumImagesOutput struct {
	UpdatedAt string            `json:"updatedAt"`
	Total     int               `json:"total"`
	ByKey     map[string]string `json:"byKey"`
}

type candidateArtist struct {
	Name    string
	Slug    string
	NormKey string
}

type candidateAlbum struct {
	Artist     string
	Album      string
	ArtistSlug string
	AlbumSlug  string
	NormKey    string
	OutputKey  string
}

type trackChunkLight struct {
	Tracks []struct {
		Artist     string `json:"artist"`
		Album      string `json:"album"`
		ArtistSlug string `json:"artistSlug"`
		AlbumSlug  string `json:"albumSlug"`
		PlayCount  int    `json:"playCount"`
	} `json:"tracks"`
}

type lastFmImage struct {
	Size string `json:"size"`
	Text string `json:"#text"`
}

type lastFmArtistInfoResponse struct {
	Artist struct {
		Name  string        `json:"name"`
		Image []lastFmImage `json:"image"`
	} `json:"artist"`
	Error   int    `json:"error"`
	Message string `json:"message"`
}

type lastFmAlbumInfoResponse struct {
	Album struct {
		Name   string        `json:"name"`
		Artist string        `json:"artist"`
		MBID   string        `json:"mbid"`
		Image  []lastFmImage `json:"image"`
	} `json:"album"`
	Error   int    `json:"error"`
	Message string `json:"message"`
}

type lastFmAPIError struct {
	Code    int
	Message string
}

func (e *lastFmAPIError) Error() string {
	return fmt.Sprintf("last.fm api error %d: %s", e.Code, e.Message)
}

func isLastFmNotFoundError(err error) bool {
	var apiErr *lastFmAPIError
	if !errors.As(err, &apiErr) {
		return false
	}
	// Last.fm commonly uses 6/7 for "not found" style errors.
	return apiErr.Code == 6 || apiErr.Code == 7
}

func envBool(name string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	return v == "1" || v == "true" || v == "yes"
}

func envInt(name string) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func runFetchMetadata() {
	apiKey := strings.TrimSpace(os.Getenv("LASTFM_API_KEY"))
	if apiKey == "" {
		fmt.Println("Error: LASTFM_API_KEY environment variable is not set.")
		fmt.Println("Get an API key at: https://www.last.fm/api/account/create")
		os.Exit(1)
	}

	username := LastFMUsername()

	scope := strings.ToLower(strings.TrimSpace(os.Getenv("LASTFM_IMAGE_SCOPE")))
	if scope != "all" {
		scope = "played"
	}

	forceRefresh := envBool("LASTFM_IMAGE_FORCE_REFRESH")
	refreshMissing := envBool("LASTFM_IMAGE_REFRESH_MISSING")
	maxArtists := envInt("LASTFM_IMAGE_MAX_ARTISTS")
	maxAlbums := envInt("LASTFM_IMAGE_MAX_ALBUMS")

	artistIndexPath := WebDataPath("artists-index.json")
	albumIndexPath := WebDataPath("albums-index.json")
	lastFmPath := LastFMStatsPath(username)
	cachePath := LastFMPath("image-cache.json")
	artistOutputPath := WebDataPath("artist-images.json")
	albumOutputPath := WebDataPath("album-images.json")

	fmt.Printf("Fetching image metadata (scope=%s)\n", scope)
	fmt.Printf("Using Last.fm data: %s\n", lastFmPath)
	fmt.Printf("Using cache: %s\n\n", cachePath)

	artists, albums, err := buildImageCandidates(scope, artistIndexPath, albumIndexPath)
	if err != nil {
		fmt.Printf("Error building candidate list: %v\n", err)
		os.Exit(1)
	}

	if maxArtists > 0 && len(artists) > maxArtists {
		artists = artists[:maxArtists]
	}
	if maxAlbums > 0 && len(albums) > maxAlbums {
		albums = albums[:maxAlbums]
	}

	fmt.Printf("Artist candidates: %d\n", len(artists))
	fmt.Printf("Album candidates:  %d\n\n", len(albums))

	mbidSeeds, err := loadAlbumMBIDSeeds(lastFmPath)
	if err != nil {
		fmt.Printf("Warning: Could not load MBID seeds (%v). Continuing without seeds.\n", err)
		mbidSeeds = make(map[string]string)
	} else {
		fmt.Printf("Loaded %d MBID seed pairs from scrobble history.\n\n", len(mbidSeeds))
	}

	cache, err := loadImageCache(cachePath)
	if err != nil {
		fmt.Printf("Warning: Could not read cache (%v). Starting with empty cache.\n", err)
		cache = newImageCache()
	}

	client := &http.Client{Timeout: 20 * time.Second}
	now := time.Now().UTC().Format(time.RFC3339)

	artistOutput := make(map[string]string)
	albumOutput := make(map[string]string)

	var artistFetches, artistHits, artistMisses, artistErrors int
	for i, candidate := range artists {
		if i > 0 && i%100 == 0 {
			fmt.Printf("Processed artists: %d/%d\n", i, len(artists))
		}

		entry, exists := cache.Artists[candidate.NormKey]
		if !exists {
			entry = ArtistCacheEntry{
				Artist: candidate.Name,
				Status: cacheStatusNotFound,
			}
		}

		if entry.ImageURL != "" && !forceRefresh {
			artistOutput[candidate.Slug] = entry.ImageURL
			artistHits++
			continue
		}
		if exists && entry.Status == cacheStatusNotFound && !refreshMissing && !forceRefresh {
			artistMisses++
			continue
		}

		artistFetches++
		entry.Attempts++
		entry.LastAttemptAt = now
		entry.Artist = candidate.Name

		imageURL, err := fetchLastFmArtistImage(client, apiKey, candidate.Name)
		if err != nil {
			entry.Status = cacheStatusError
			entry.ErrorMessage = err.Error()
			cache.Artists[candidate.NormKey] = entry

			if entry.ImageURL != "" {
				artistOutput[candidate.Slug] = entry.ImageURL
			} else {
				artistErrors++
			}
			time.Sleep(120 * time.Millisecond)
			continue
		}

		if imageURL == "" {
			entry.Status = cacheStatusNotFound
			entry.ErrorMessage = ""
			cache.Artists[candidate.NormKey] = entry
			artistMisses++
			time.Sleep(120 * time.Millisecond)
			continue
		}

		entry.ImageURL = imageURL
		entry.Status = cacheStatusOK
		entry.Source = "lastfm"
		entry.ErrorMessage = ""
		entry.LastSuccessAt = now
		cache.Artists[candidate.NormKey] = entry
		artistOutput[candidate.Slug] = imageURL
		time.Sleep(120 * time.Millisecond)
	}

	var albumFetches, albumHits, albumMisses, albumErrors int
	for i, candidate := range albums {
		if i > 0 && i%100 == 0 {
			fmt.Printf("Processed albums: %d/%d\n", i, len(albums))
		}

		entry, exists := cache.Albums[candidate.NormKey]
		if !exists {
			entry = AlbumCacheEntry{
				Artist: candidate.Artist,
				Album:  candidate.Album,
				Status: cacheStatusNotFound,
			}
		}

		if entry.MBID == "" {
			if seedMBID, ok := mbidSeeds[candidate.NormKey]; ok {
				entry.MBID = seedMBID
			}
		}

		if entry.ImageURL != "" && !forceRefresh {
			albumOutput[candidate.OutputKey] = entry.ImageURL
			albumHits++
			cache.Albums[candidate.NormKey] = entry
			continue
		}
		if exists && entry.Status == cacheStatusNotFound && !refreshMissing && !forceRefresh {
			albumMisses++
			cache.Albums[candidate.NormKey] = entry
			continue
		}

		albumFetches++
		entry.Attempts++
		entry.LastAttemptAt = now
		entry.Artist = candidate.Artist
		entry.Album = candidate.Album

		imageURL, resolvedMBID, err := fetchLastFmAlbumImage(client, apiKey, candidate.Artist, candidate.Album, entry.MBID)
		if resolvedMBID != "" {
			entry.MBID = resolvedMBID
		}

		if err != nil {
			if isLastFmNotFoundError(err) {
				entry.Status = cacheStatusNotFound
				entry.ErrorMessage = ""
				cache.Albums[candidate.NormKey] = entry
				albumMisses++
			} else {
				entry.Status = cacheStatusError
				entry.ErrorMessage = err.Error()
				cache.Albums[candidate.NormKey] = entry
				if entry.ImageURL != "" {
					albumOutput[candidate.OutputKey] = entry.ImageURL
				} else {
					albumErrors++
				}
			}
			time.Sleep(120 * time.Millisecond)
			continue
		}

		if imageURL == "" {
			entry.Status = cacheStatusNotFound
			entry.ErrorMessage = ""
			cache.Albums[candidate.NormKey] = entry
			albumMisses++
			time.Sleep(120 * time.Millisecond)
			continue
		}

		entry.ImageURL = imageURL
		entry.Status = cacheStatusOK
		entry.Source = "lastfm"
		entry.ErrorMessage = ""
		entry.LastSuccessAt = now
		cache.Albums[candidate.NormKey] = entry
		albumOutput[candidate.OutputKey] = imageURL
		time.Sleep(120 * time.Millisecond)
	}

	cache.UpdatedAt = now
	if err := saveImageCache(cachePath, cache); err != nil {
		fmt.Printf("Error writing cache: %v\n", err)
		os.Exit(1)
	}

	artistOutputData := ArtistImagesOutput{
		UpdatedAt:    now,
		Total:        len(artistOutput),
		ByArtistSlug: artistOutput,
	}
	if err := writeJSONFile(artistOutputPath, artistOutputData); err != nil {
		fmt.Printf("Error writing %s: %v\n", artistOutputPath, err)
		os.Exit(1)
	}

	albumOutputData := AlbumImagesOutput{
		UpdatedAt: now,
		Total:     len(albumOutput),
		ByKey:     albumOutput,
	}
	if err := writeJSONFile(albumOutputPath, albumOutputData); err != nil {
		fmt.Printf("Error writing %s: %v\n", albumOutputPath, err)
		os.Exit(1)
	}

	fmt.Println("\nImage metadata fetch complete.")
	fmt.Printf("Artist images: %d (cache hits: %d, fetched: %d, misses: %d, errors: %d)\n", len(artistOutput), artistHits, artistFetches, artistMisses, artistErrors)
	fmt.Printf("Album images:  %d (cache hits: %d, fetched: %d, misses: %d, errors: %d)\n", len(albumOutput), albumHits, albumFetches, albumMisses, albumErrors)
	fmt.Printf("Wrote: %s\n", artistOutputPath)
	fmt.Printf("Wrote: %s\n", albumOutputPath)
}

func buildImageCandidates(scope, artistIndexPath, albumIndexPath string) ([]candidateArtist, []candidateAlbum, error) {
	if scope == "all" {
		return loadCandidatesFromIndexes(artistIndexPath, albumIndexPath)
	}
	return loadCandidatesFromPlayedTracks(WebDataPath("chunks"))
}

func loadCandidatesFromIndexes(artistIndexPath, albumIndexPath string) ([]candidateArtist, []candidateAlbum, error) {
	var artistsIndex ArtistIndex
	if err := readJSONFile(artistIndexPath, &artistsIndex); err != nil {
		return nil, nil, err
	}

	var albumsIndex AlbumIndex
	if err := readJSONFile(albumIndexPath, &albumsIndex); err != nil {
		return nil, nil, err
	}

	artistMap := make(map[string]candidateArtist)
	for _, artist := range artistsIndex.Artists {
		name := strings.TrimSpace(artist.Name)
		if name == "" || name == "Unknown Artist" {
			continue
		}
		slug := strings.TrimSpace(artist.Slug)
		if slug == "" {
			slug = Slugify(name)
		}
		artistMap[slug] = candidateArtist{
			Name:    name,
			Slug:    slug,
			NormKey: normalizeForMatching(name),
		}
	}

	albumMap := make(map[string]candidateAlbum)
	for _, album := range albumsIndex.Albums {
		albumName := strings.TrimSpace(album.Name)
		if albumName == "" || albumName == "Unknown Album" {
			continue
		}
		albumSlug := strings.TrimSpace(album.Slug)
		if albumSlug == "" {
			albumSlug = Slugify(albumName)
		}

		for _, artistName := range album.Artists {
			artistName = strings.TrimSpace(artistName)
			if artistName == "" || artistName == "Unknown Artist" {
				continue
			}
			artistSlug := Slugify(artistName)
			outputKey := artistSlug + "|" + albumSlug
			albumMap[outputKey] = candidateAlbum{
				Artist:     artistName,
				Album:      albumName,
				ArtistSlug: artistSlug,
				AlbumSlug:  albumSlug,
				NormKey:    normalizeForMatching(artistName) + "|" + normalizeForMatching(albumName),
				OutputKey:  outputKey,
			}
		}
	}

	artists := make([]candidateArtist, 0, len(artistMap))
	for _, artist := range artistMap {
		if artist.NormKey == "" {
			continue
		}
		artists = append(artists, artist)
	}
	sort.Slice(artists, func(i, j int) bool {
		return artists[i].Slug < artists[j].Slug
	})

	albums := make([]candidateAlbum, 0, len(albumMap))
	for _, album := range albumMap {
		if album.NormKey == "|" {
			continue
		}
		albums = append(albums, album)
	}
	sort.Slice(albums, func(i, j int) bool {
		return albums[i].OutputKey < albums[j].OutputKey
	})

	return artists, albums, nil
}

func loadCandidatesFromPlayedTracks(chunksDir string) ([]candidateArtist, []candidateAlbum, error) {
	chunkPaths, err := filepath.Glob(filepath.Join(chunksDir, "tracks-*.json"))
	if err != nil {
		return nil, nil, err
	}
	if len(chunkPaths) == 0 {
		return nil, nil, fmt.Errorf("no chunk files found in %s", chunksDir)
	}
	sort.Strings(chunkPaths)

	artistMap := make(map[string]candidateArtist)
	albumMap := make(map[string]candidateAlbum)

	for _, chunkPath := range chunkPaths {
		var chunk trackChunkLight
		if err := readJSONFile(chunkPath, &chunk); err != nil {
			return nil, nil, err
		}

		for _, track := range chunk.Tracks {
			if track.PlayCount <= 0 {
				continue
			}

			artistName := strings.TrimSpace(track.Artist)
			if artistName == "" || artistName == "Unknown Artist" {
				continue
			}
			artistSlug := strings.TrimSpace(track.ArtistSlug)
			if artistSlug == "" {
				artistSlug = Slugify(artistName)
			}
			artistNorm := normalizeForMatching(artistName)
			if artistNorm == "" {
				continue
			}
			artistMap[artistSlug] = candidateArtist{
				Name:    artistName,
				Slug:    artistSlug,
				NormKey: artistNorm,
			}

			albumName := strings.TrimSpace(track.Album)
			if albumName == "" || albumName == "Unknown Album" {
				continue
			}
			albumSlug := strings.TrimSpace(track.AlbumSlug)
			if albumSlug == "" {
				albumSlug = Slugify(albumName)
			}
			albumNorm := normalizeForMatching(albumName)
			if albumNorm == "" {
				continue
			}

			outputKey := artistSlug + "|" + albumSlug
			albumMap[outputKey] = candidateAlbum{
				Artist:     artistName,
				Album:      albumName,
				ArtistSlug: artistSlug,
				AlbumSlug:  albumSlug,
				NormKey:    artistNorm + "|" + albumNorm,
				OutputKey:  outputKey,
			}
		}
	}

	artists := make([]candidateArtist, 0, len(artistMap))
	for _, artist := range artistMap {
		artists = append(artists, artist)
	}
	sort.Slice(artists, func(i, j int) bool {
		return artists[i].Slug < artists[j].Slug
	})

	albums := make([]candidateAlbum, 0, len(albumMap))
	for _, album := range albumMap {
		albums = append(albums, album)
	}
	sort.Slice(albums, func(i, j int) bool {
		return albums[i].OutputKey < albums[j].OutputKey
	})

	return artists, albums, nil
}

func loadAlbumMBIDSeeds(lastFmPath string) (map[string]string, error) {
	var lastFmData LastFmData
	if err := readJSONFile(lastFmPath, &lastFmData); err != nil {
		return nil, err
	}

	counts := make(map[string]map[string]int)
	for _, scrobble := range lastFmData.Scrobbles {
		artist := normalizeForMatching(scrobble.Artist)
		album := normalizeForMatching(scrobble.Album)
		mbid := strings.ToLower(strings.TrimSpace(scrobble.AlbumID))
		if artist == "" || album == "" || mbid == "" {
			continue
		}

		pairKey := artist + "|" + album
		if counts[pairKey] == nil {
			counts[pairKey] = make(map[string]int)
		}
		counts[pairKey][mbid]++
	}

	seeds := make(map[string]string)
	for pairKey, mbidCounts := range counts {
		topMBID := ""
		topCount := 0
		for mbid, count := range mbidCounts {
			if count > topCount || (count == topCount && mbid < topMBID) {
				topMBID = mbid
				topCount = count
			}
		}
		if topMBID != "" {
			seeds[pairKey] = topMBID
		}
	}

	return seeds, nil
}

func newImageCache() ImageCache {
	return ImageCache{
		Version: 1,
		Artists: make(map[string]ArtistCacheEntry),
		Albums:  make(map[string]AlbumCacheEntry),
	}
}

func loadImageCache(path string) (ImageCache, error) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return newImageCache(), nil
	}

	var cache ImageCache
	if err := readJSONFile(path, &cache); err != nil {
		return ImageCache{}, err
	}

	if cache.Artists == nil {
		cache.Artists = make(map[string]ArtistCacheEntry)
	}
	if cache.Albums == nil {
		cache.Albums = make(map[string]AlbumCacheEntry)
	}
	if cache.Version == 0 {
		cache.Version = 1
	}

	return cache, nil
}

func saveImageCache(path string, cache ImageCache) error {
	return writeJSONFile(path, cache)
}

func readJSONFile(path string, out interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func writeJSONFile(path string, value interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}

func doLastFmRequest(client *http.Client, apiKey string, params url.Values) ([]byte, error) {
	params.Set("api_key", apiKey)
	params.Set("format", "json")

	resp, err := client.Get(lastFmBaseURL + "?" + params.Encode())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d from last.fm: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return body, nil
}

func fetchLastFmArtistImage(client *http.Client, apiKey, artist string) (string, error) {
	params := url.Values{}
	params.Set("method", "artist.getinfo")
	params.Set("artist", artist)
	params.Set("autocorrect", "1")

	body, err := doLastFmRequest(client, apiKey, params)
	if err != nil {
		return "", err
	}

	var response lastFmArtistInfoResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return "", err
	}
	if response.Error != 0 {
		return "", &lastFmAPIError{
			Code:    response.Error,
			Message: response.Message,
		}
	}

	return pickBestLastFmImage(response.Artist.Image), nil
}

func fetchLastFmAlbumImage(client *http.Client, apiKey, artist, album, mbid string) (string, string, error) {
	mbid = strings.TrimSpace(mbid)
	if mbid != "" {
		response, err := fetchLastFmAlbumByMBID(client, apiKey, mbid)
		if err == nil {
			imageURL := pickBestLastFmImage(response.Album.Image)
			resolvedMBID := strings.TrimSpace(response.Album.MBID)
			if imageURL != "" {
				return imageURL, resolvedMBID, nil
			}
			mbid = resolvedMBID
		} else if !isLastFmNotFoundError(err) {
			return "", mbid, err
		}
	}

	if strings.TrimSpace(artist) == "" || strings.TrimSpace(album) == "" {
		return "", mbid, nil
	}

	response, err := fetchLastFmAlbumByName(client, apiKey, artist, album)
	if err != nil {
		return "", mbid, err
	}

	imageURL := pickBestLastFmImage(response.Album.Image)
	resolvedMBID := strings.TrimSpace(response.Album.MBID)
	if resolvedMBID == "" {
		resolvedMBID = mbid
	}

	return imageURL, resolvedMBID, nil
}

func fetchLastFmAlbumByMBID(client *http.Client, apiKey, mbid string) (lastFmAlbumInfoResponse, error) {
	params := url.Values{}
	params.Set("method", "album.getinfo")
	params.Set("mbid", mbid)

	body, err := doLastFmRequest(client, apiKey, params)
	if err != nil {
		return lastFmAlbumInfoResponse{}, err
	}

	var response lastFmAlbumInfoResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return lastFmAlbumInfoResponse{}, err
	}
	if response.Error != 0 {
		return lastFmAlbumInfoResponse{}, &lastFmAPIError{
			Code:    response.Error,
			Message: response.Message,
		}
	}

	return response, nil
}

func fetchLastFmAlbumByName(client *http.Client, apiKey, artist, album string) (lastFmAlbumInfoResponse, error) {
	params := url.Values{}
	params.Set("method", "album.getinfo")
	params.Set("artist", artist)
	params.Set("album", album)
	params.Set("autocorrect", "1")

	body, err := doLastFmRequest(client, apiKey, params)
	if err != nil {
		return lastFmAlbumInfoResponse{}, err
	}

	var response lastFmAlbumInfoResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return lastFmAlbumInfoResponse{}, err
	}
	if response.Error != 0 {
		return lastFmAlbumInfoResponse{}, &lastFmAPIError{
			Code:    response.Error,
			Message: response.Message,
		}
	}

	return response, nil
}

func pickBestLastFmImage(images []lastFmImage) string {
	sizeRank := map[string]int{
		"small":      1,
		"medium":     2,
		"large":      3,
		"extralarge": 4,
		"mega":       5,
		"":           0,
	}

	bestURL := ""
	bestRank := -1

	for _, image := range images {
		url := strings.TrimSpace(image.Text)
		if url == "" {
			continue
		}
		rank, ok := sizeRank[strings.ToLower(strings.TrimSpace(image.Size))]
		if !ok {
			rank = 0
		}
		if rank > bestRank {
			bestRank = rank
			bestURL = url
		}
	}

	return bestURL
}
