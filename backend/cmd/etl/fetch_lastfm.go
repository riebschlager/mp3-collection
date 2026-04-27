package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
	"github.com/riebschlager/mp3-collection/backend/internal/db"
	"github.com/spf13/cobra"
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

var fetchLastFmCmd = &cobra.Command{
	Use:   "fetch-lastfm",
	Short: "Fetch recent scrobbles from Last.fm and ingest into SQLite",
	Run: func(cmd *cobra.Command, args []string) {
		godotenv.Load("../.env")

		apiKey := os.Getenv("LASTFM_API_KEY")
		if apiKey == "" {
			fmt.Println("Error: LASTFM_API_KEY environment variable is not set.")
			os.Exit(1)
		}

		username := os.Getenv("LASTFM_USERNAME")
		if username == "" {
			username = "riebschlager"
		}

		dbPath, _ := cmd.Flags().GetString("db")
		fmt.Printf("Connecting to database at %s\n", dbPath)
		database, err := db.InitDB(dbPath, "schema.sql")
		if err != nil {
			fmt.Printf("Error opening database: %v\n", err)
			os.Exit(1)
		}
		defer database.Close()

		// Get the most recent scrobble timestamp from DB to know when to stop fetching
		var maxDateStr sqlNullString // using basic string for ease
		err = database.QueryRow("SELECT MAX(played_at) FROM listening_history WHERE source = 'lastfm'").Scan(&maxDateStr)
		
		var maxDate time.Time
		if err == nil && maxDateStr.Valid && maxDateStr.String != "" {
			maxDate, _ = time.Parse(time.RFC3339, maxDateStr.String)
			fmt.Printf("Most recent scrobble in DB is from: %s\n", maxDate)
		} else {
			fmt.Println("No existing Last.fm scrobbles found in DB. Fetching recent history...")
			// To avoid fetching 20 years of history in one run if DB is empty,
			// we limit to a few pages unless configured otherwise.
			// The original script fetched just 1 page, so we will do the same by default.
		}

		// Prepare statements
		tx, err := database.Begin()
		if err != nil {
			fmt.Printf("Error starting transaction: %v\n", err)
			os.Exit(1)
		}
		defer tx.Rollback()

		insertArtist, _ := tx.Prepare("INSERT OR IGNORE INTO artists (id, name, slug) VALUES (?, ?, ?)")
		insertAlbum, _ := tx.Prepare("INSERT OR IGNORE INTO albums (id, title, artist_id, slug) VALUES (?, ?, ?, ?)")
		insertTrack, _ := tx.Prepare("INSERT OR IGNORE INTO tracks (id, title, album_id, artist_id, genre, year, duration_ms, track_number, disc_number, slug) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
		insertHistory, _ := tx.Prepare("INSERT OR IGNORE INTO listening_history (id, track_id, played_at, source) VALUES (?, ?, ?, ?)")

		fmt.Printf("Fetching recent tracks for user: %s\n", username)

		page := 1
		pagesToFetch, _ := cmd.Flags().GetInt("pages")
		addedCount := 0

		for page <= pagesToFetch {
			tracks, totalPages, err := fetchRecentTracksPage(username, apiKey, page)
			if err != nil {
				fmt.Printf("Error fetching tracks: %v\n", err)
				os.Exit(1)
			}

			if len(tracks) == 0 {
				break
			}

			fmt.Printf("Fetched page %d of %s (%d tracks)\n", page, totalPages, len(tracks))

			for _, t := range tracks {
				if t.Date.UTS == "" {
					continue // Skip "now playing"
				}

				uts, _ := strconv.ParseInt(t.Date.UTS, 10, 64)
				playedAt := time.Unix(uts, 0).UTC()

				// Stop if we've reached tracks we already have
				if !maxDate.IsZero() && playedAt.Before(maxDate) {
					fmt.Println("Reached already synced scrobbles. Stopping.")
					page = pagesToFetch // Break outer loop
					break
				}

				artistName := t.Artist.Text
				albumTitle := t.Album.Text
				trackTitle := t.Name

				if artistName == "" || trackTitle == "" {
					continue
				}
				if albumTitle == "" {
					albumTitle = "Unknown Album"
				}

				artistSlug := slugify(artistName)
				artistID := hashID("artist:" + artistSlug)

				albumSlug := slugify(artistName + "-" + albumTitle)
				albumID := hashID("album:" + albumSlug)

				trackSlug := slugify(artistName + "-" + albumTitle + "-" + trackTitle)
				trackID := hashID("track:" + trackSlug)

				// Ensure stub records exist so foreign keys don't fail
				insertArtist.Exec(artistID, artistName, artistSlug)
				insertAlbum.Exec(albumID, albumTitle, artistID, albumSlug)
				insertTrack.Exec(trackID, trackTitle, albumID, artistID, "", 0, 0, 0, 0, trackSlug)

				historyID := hashID("lastfm:" + trackID + ":" + strconv.FormatInt(uts, 10))
				res, err := insertHistory.Exec(historyID, trackID, playedAt.Format(time.RFC3339), "lastfm")
				if err == nil {
					rowsAffected, _ := res.RowsAffected()
					addedCount += int(rowsAffected)
				}
			}

			page++
			// Be nice to API
			time.Sleep(500 * time.Millisecond)
		}

		if err := tx.Commit(); err != nil {
			fmt.Printf("Error committing transaction: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Successfully added %d new Last.fm scrobbles to SQLite.\n", addedCount)
	},
}

type sqlNullString struct {
	String string
	Valid  bool
}

func (s *sqlNullString) Scan(value interface{}) error {
	if value == nil {
		s.String, s.Valid = "", false
		return nil
	}
	s.Valid = true
	switch v := value.(type) {
	case []byte:
		s.String = string(v)
	case string:
		s.String = v
	}
	return nil
}

func init() {
	rootCmd.AddCommand(fetchLastFmCmd)
	fetchLastFmCmd.Flags().String("db", "../data/mp3_collection.db", "Path to SQLite database")
	fetchLastFmCmd.Flags().Int("pages", 1, "Number of pages to fetch (200 tracks per page)")
}

func fetchRecentTracksPage(username, apiKey string, page int) ([]struct {
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
}, string, error) {
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
		return nil, "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	var result LastFmResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, "", err
	}

	return result.RecentTracks.Track, result.RecentTracks.Attr.TotalPages, nil
}
