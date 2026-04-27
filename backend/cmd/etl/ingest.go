package main

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/riebschlager/mp3-collection/backend/internal/db"
	"github.com/spf13/cobra"
)

var ingestCmd = &cobra.Command{
	Use:   "ingest-itunes",
	Short: "Ingest compiled iTunes CSV into SQLite",
	Run: func(cmd *cobra.Command, args []string) {
		dbPath, _ := cmd.Flags().GetString("db")
		csvPath, _ := cmd.Flags().GetString("csv")

		fmt.Printf("Ingesting iTunes data from %s to %s\n", csvPath, dbPath)
		database, err := db.InitDB(dbPath, "schema.sql")
		if err != nil {
			fmt.Printf("Error opening database: %v\n", err)
			os.Exit(1)
		}
		defer database.Close()

		f, err := os.Open(csvPath)
		if err != nil {
			fmt.Printf("Error opening CSV: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()

		reader := csv.NewReader(f)
		header, err := reader.Read()
		if err != nil {
			fmt.Printf("Error reading CSV header: %v\n", err)
			os.Exit(1)
		}

		// Begin transaction
		tx, err := database.Begin()
		if err != nil {
			fmt.Printf("Error starting transaction: %v\n", err)
			os.Exit(1)
		}
		defer tx.Rollback()

		insertArtist, _ := tx.Prepare("INSERT OR IGNORE INTO artists (id, name, slug) VALUES (?, ?, ?)")
		insertAlbum, _ := tx.Prepare("INSERT OR IGNORE INTO albums (id, title, artist_id, slug) VALUES (?, ?, ?, ?)")
		insertTrack, _ := tx.Prepare("INSERT OR IGNORE INTO tracks (id, title, album_id, artist_id, genre, year, duration_ms, track_number, disc_number, slug) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")

		count := 0
		for {
			record, err := reader.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				continue
			}

			row := make(map[string]string)
			for i, h := range header {
				if i < len(record) {
					row[h] = record[i]
				}
			}

			// Core fields
			artistName := row["Artist"]
			albumTitle := row["Album"]
			trackTitle := row["Name"]
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

			year, _ := strconv.Atoi(row["Year"])
			durationSeconds, _ := strconv.Atoi(row["Time"])
			durationMs := durationSeconds * 1000
			trackNum, _ := strconv.Atoi(row["Track Number"])
			discNum, _ := strconv.Atoi(row["Disc Number"])
			genre := row["Genre"]

			insertArtist.Exec(artistID, artistName, artistSlug)
			insertAlbum.Exec(albumID, albumTitle, artistID, albumSlug)
			insertTrack.Exec(trackID, trackTitle, albumID, artistID, genre, year, durationMs, trackNum, discNum, trackSlug)

			count++
			if count%1000 == 0 {
				fmt.Printf("Processed %d tracks...\n", count)
			}
		}

		if err := tx.Commit(); err != nil {
			fmt.Printf("Error committing transaction: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Successfully ingested %d tracks.\n", count)
	},
}

func init() {
	rootCmd.AddCommand(ingestCmd)
	ingestCmd.Flags().String("db", "../data/mp3_collection.db", "Path to SQLite database")
	ingestCmd.Flags().String("csv", "../data/derived/compiled/compiled_itunes_library.csv", "Path to compiled CSV")
}

func slugify(text string) string {
	text = strings.ToLower(text)
	reg := regexp.MustCompile(`[^\w\s-]`)
	text = reg.ReplaceAllString(text, "")
	regSpace := regexp.MustCompile(`[-\s]+`)
	text = regSpace.ReplaceAllString(text, "-")
	return strings.Trim(text, "-")
}

func hashID(input string) string {
	hash := sha256.Sum256([]byte(input))
	return hex.EncodeToString(hash[:])[0:16]
}
