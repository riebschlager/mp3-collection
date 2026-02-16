package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type LastFmResponse struct {
	RecentTracks struct {
		Track []struct {
			Artist struct {
				Text string `json:"#text"`
			} `json:"artist"`
			Name  string `json:"name"`
			MBID  string `json:"mbid"`
			Album struct {
				Text string `json:"#text"`
				MBID string `json:"mbid"`
			} `json:"album"`
			Date struct {
				UTS string `json:"uts"`
			} `json:"date"`
			Attr struct {
				NowPlaying string `json:"nowplaying"`
			} `json:"@attr"`
		} `json:"track"`
		Attr struct {
			Page       string `json:"page"`
			TotalPages string `json:"totalPages"`
			User       string `json:"user"`
			Total      string `json:"total"`
		} `json:"@attr"`
	} `json:"recenttracks"`
}

func runFetchLastFm() {
	apiKey := os.Getenv("LASTFM_API_KEY")
	if apiKey == "" {
		fmt.Println("Error: LASTFM_API_KEY environment variable is not set.")
		fmt.Println("Get an API key at: https://www.last.fm/api/account/create")
		os.Exit(1)
	}

	username := LastFMUsername()

	fmt.Printf("Fetching recent tracks for user: %s\n", username)

	// Fetch only the first page to get the most recent ones
	tracks, err := fetchRecentTracks(username, apiKey, 1)
	if err != nil {
		fmt.Printf("Error fetching tracks: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Fetched %d recent tracks.\n", len(tracks))

	// Load existing data
	lastfmPath := LastFMStatsPath(username)
	var existingData LastFmData

	data, err := os.ReadFile(lastfmPath)
	if err == nil {
		if err := json.Unmarshal(data, &existingData); err != nil {
			fmt.Printf("Warning: Could not parse existing data: %v\n", err)
			existingData.Username = username
			existingData.Scrobbles = []LastFmScrobble{}
		}
	} else {
		fmt.Printf("Creating new data file: %s\n", lastfmPath)
		existingData.Username = username
		existingData.Scrobbles = []LastFmScrobble{}
	}

	// Create a map of existing scrobbles by date to avoid duplicates
	existingDates := make(map[int64]bool)
	for _, s := range existingData.Scrobbles {
		existingDates[s.Date] = true
	}

	newScrobblesCount := 0
	// Last.fm returns newest first. We want to keep chronological order if the file is oldest first.
	// So we'll process them and then decide how to merge.
	var toAdd []LastFmScrobble
	for _, t := range tracks {
		if t.Date.UTS == "" {
			continue // Skip "now playing" track
		}

		uts, _ := strconv.ParseInt(t.Date.UTS, 10, 64)
		ms := uts * 1000

		if !existingDates[ms] {
			scrobble := LastFmScrobble{
				Track:   t.Name,
				Artist:  t.Artist.Text,
				Album:   t.Album.Text,
				AlbumID: strings.TrimSpace(t.Album.MBID),
				Date:    ms,
			}
			toAdd = append(toAdd, scrobble)
			newScrobblesCount++
		}
	}

	if newScrobblesCount > 0 {
		fmt.Printf("Adding %d new scrobbles to the collection.\n", newScrobblesCount)

		// Since API returns newest first, toAdd is newest first.
		// We want to append them to the end of the existing (oldest first) list.
		// But we must reverse toAdd so it's oldest first among the new ones.
		for i, j := 0, len(toAdd)-1; i < j; i, j = i+1, j-1 {
			toAdd[i], toAdd[j] = toAdd[j], toAdd[i]
		}

		existingData.Scrobbles = append(existingData.Scrobbles, toAdd...)

		// Write back to file
		if err := saveLastFmData(lastfmPath, existingData); err != nil {
			fmt.Printf("Error saving data: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Data updated successfully.")
	} else {
		fmt.Println("No new scrobbles found.")
	}
}

func fetchRecentTracks(username, apiKey string, page int) ([]struct {
	Artist struct {
		Text string `json:"#text"`
	} `json:"artist"`
	Name  string `json:"name"`
	MBID  string `json:"mbid"`
	Album struct {
		Text string `json:"#text"`
		MBID string `json:"mbid"`
	} `json:"album"`
	Date struct {
		UTS string `json:"uts"`
	} `json:"date"`
	Attr struct {
		NowPlaying string `json:"nowplaying"`
	} `json:"@attr"`
}, error) {
	baseUrl := "https://ws.audioscrobbler.com/2.0/"
	params := url.Values{}
	params.Add("method", "user.getrecenttracks")
	params.Add("user", username)
	params.Add("api_key", apiKey)
	params.Add("format", "json")
	params.Add("limit", "200")
	params.Add("page", strconv.Itoa(page))

	resp, err := http.Get(baseUrl + "?" + params.Encode())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result LastFmResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	return result.RecentTracks.Track, nil
}

func saveLastFmData(path string, data LastFmData) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(data)
}
