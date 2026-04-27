package main

import (
	"fmt"
	"os"

	"github.com/riebschlager/mp3-collection/backend/internal/db"
	"github.com/spf13/cobra"
)

var computeErasCmd = &cobra.Command{
	Use:   "compute-eras",
	Short: "Compute year-to-year era similarity cache",
	Run: func(cmd *cobra.Command, args []string) {
		dbPath, _ := cmd.Flags().GetString("db")
		fmt.Printf("Connecting to database at %s\n", dbPath)

		database, err := db.InitDB(dbPath, "schema.sql")
		if err != nil {
			fmt.Printf("Error opening database: %v\n", err)
			os.Exit(1)
		}
		defer database.Close()

		fmt.Println("Clearing existing era_similarities...")
		_, err = database.Exec("DELETE FROM era_similarities")
		if err != nil {
			fmt.Printf("Error clearing table: %v\n", err)
			os.Exit(1)
		}

		sources := []string{"all", "spotify", "lastfm"}
		
		tx, err := database.Begin()
		if err != nil {
			os.Exit(1)
		}
		defer tx.Rollback()
		
		insertStmt, _ := tx.Prepare("INSERT INTO era_similarities (source_filter, year_a, year_b, similarity_score) VALUES (?, ?, ?, ?)")

		for _, source := range sources {
			// Get all artists played in each year
			query := `
				SELECT CAST(strftime('%Y', h.played_at) AS INTEGER) as year, t.artist_id, COUNT(*) as weight
				FROM listening_history h
				JOIN tracks t ON h.track_id = t.id
			`
			if source != "all" {
				query += " WHERE h.source = '" + source + "'"
			}
			query += " GROUP BY year, t.artist_id HAVING year > 1990"

			rows, err := database.Query(query)
			if err != nil {
				continue
			}

			// Map year -> set of artists
			yearArtists := make(map[int]map[string]int)
			for rows.Next() {
				var year int
				var artistID string
				var weight int
				if err := rows.Scan(&year, &artistID, &weight); err == nil {
					if yearArtists[year] == nil {
						yearArtists[year] = make(map[string]int)
					}
					yearArtists[year][artistID] = weight
				}
			}
			rows.Close()

			var years []int
			for y := range yearArtists {
				years = append(years, y)
			}

			count := 0
			for _, y1 := range years {
				for _, y2 := range years {
					if y1 > y2 { // store symmetrical pairs once, but for simplicity let's insert both or use y1 <= y2 constraint in app
						continue
					}
					
					intersection := 0
					union := 0
					
					a1 := yearArtists[y1]
					a2 := yearArtists[y2]

					// Basic Jaccard Index based on sets of artists
					setAll := make(map[string]bool)
					for a := range a1 {
						setAll[a] = true
						if a2[a] > 0 {
							intersection++
						}
					}
					for a := range a2 {
						setAll[a] = true
					}
					union = len(setAll)

					score := 0.0
					if union > 0 {
						score = float64(intersection) / float64(union)
					}

					insertStmt.Exec(source, y1, y2, score)
					// Make it symmetrical
					if y1 != y2 {
						insertStmt.Exec(source, y2, y1, score)
					}
					count++
				}
			}
			fmt.Printf("Inserted %d pairs for source: %s\n", count*2, source)
		}

		if err := tx.Commit(); err != nil {
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(computeErasCmd)
	computeErasCmd.Flags().String("db", "../data/mp3_collection.db", "Path to SQLite database")
}
