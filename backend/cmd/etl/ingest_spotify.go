package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/riebschlager/mp3-collection/backend/internal/db"
	"github.com/spf13/cobra"
)

type spotifyAudioRow struct {
	TS       string `json:"ts"`
	MsPlayed int64  `json:"ms_played"`
	Track    string `json:"master_metadata_track_name"`
	Artist   string `json:"master_metadata_album_artist_name"`
	Album    string `json:"master_metadata_album_album_name"`
	TrackURI string `json:"spotify_track_uri"`
}

var ingestSpotifyCmd = &cobra.Command{
	Use:   "ingest-spotify",
	Short: "Ingest Spotify history into SQLite",
	Run: func(cmd *cobra.Command, args []string) {
		dbPath, _ := cmd.Flags().GetString("db")
		spotifyDir, _ := cmd.Flags().GetString("dir")

		fmt.Printf("Connecting to database at %s\n", dbPath)
		database, err := db.InitDB(dbPath, "schema.sql")
		if err != nil {
			fmt.Printf("Error opening database: %v\n", err)
			os.Exit(1)
		}
		defer database.Close()

		pattern := filepath.Join(spotifyDir, "Streaming_History_Audio_*.json")
		files, err := filepath.Glob(pattern)
		if err != nil || len(files) == 0 {
			fmt.Println("No Spotify history files found.")
			return
		}
		sort.Strings(files)

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
		
		// To deduplicate against Last.fm, we query if the track was played within +/- 2 minutes
		checkNearby, _ := tx.Prepare(`
			SELECT 1 FROM listening_history 
			WHERE track_id = ? 
			AND source = 'lastfm' 
			AND ABS(strftime('%s', played_at) - strftime('%s', ?)) <= 120
		`)

		addedCount := 0
		dedupedCount := 0
		shortCount := 0
		exactDupesCount := 0

		seenExact := make(map[string]struct{})

		for _, file := range files {
			fmt.Printf("Processing %s...\n", file)
			data, err := os.ReadFile(file)
			if err != nil {
				continue
			}

			var rows []spotifyAudioRow
			if err := json.Unmarshal(data, &rows); err != nil {
				continue
			}

			for _, row := range rows {
				if strings.TrimSpace(row.Track) == "" || strings.TrimSpace(row.Artist) == "" {
					continue
				}
				if row.MsPlayed < 30000 {
					shortCount++
					continue
				}

				parsed, err := time.Parse(time.RFC3339, row.TS)
				if err != nil {
					continue
				}

				artistName := row.Artist
				albumTitle := row.Album
				trackTitle := row.Track

				if albumTitle == "" {
					albumTitle = "Unknown Album"
				}

				artistSlug := slugify(artistName)
				artistID := hashID("artist:" + artistSlug)

				albumSlug := slugify(artistName + "-" + albumTitle)
				albumID := hashID("album:" + albumSlug)

				trackSlug := slugify(artistName + "-" + albumTitle + "-" + trackTitle)
				trackID := hashID("track:" + trackSlug)

				playedAtStr := parsed.Format(time.RFC3339)

				// Exact dedupe within Spotify files (Spotify sometimes includes overlap in chunks)
				sig := trackID + ":" + playedAtStr + ":" + strconv.FormatInt(row.MsPlayed, 10)
				if _, exists := seenExact[sig]; exists {
					exactDupesCount++
					continue
				}
				seenExact[sig] = struct{}{}

				// Dedupe against Last.fm
				var exists int
				err = checkNearby.QueryRow(trackID, playedAtStr).Scan(&exists)
				if err == nil && exists == 1 {
					dedupedCount++
					continue
				}

				insertArtist.Exec(artistID, artistName, artistSlug)
				insertAlbum.Exec(albumID, albumTitle, artistID, albumSlug)
				insertTrack.Exec(trackID, trackTitle, albumID, artistID, "", 0, int(row.MsPlayed), 0, 0, trackSlug)

				historyID := hashID("spotify:" + trackID + ":" + strconv.FormatInt(parsed.Unix(), 10))
				res, err := insertHistory.Exec(historyID, trackID, playedAtStr, "spotify")
				if err == nil {
					rowsAffected, _ := res.RowsAffected()
					addedCount += int(rowsAffected)
				}
			}
		}

		if err := tx.Commit(); err != nil {
			fmt.Printf("Error committing transaction: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Spotify Ingestion Complete:")
		fmt.Printf("- Filtered (Short play): %d\n", shortCount)
		fmt.Printf("- Exact dupes ignored: %d\n", exactDupesCount)
		fmt.Printf("- Deduped against Last.fm: %d\n", dedupedCount)
		fmt.Printf("- Added to DB: %d\n", addedCount)
	},
}

func init() {
	rootCmd.AddCommand(ingestSpotifyCmd)
	ingestSpotifyCmd.Flags().String("db", "../data/mp3_collection.db", "Path to SQLite database")
	ingestSpotifyCmd.Flags().String("dir", "../data/inputs/spotify", "Path to Spotify JSON data")
}
