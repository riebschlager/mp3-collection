package main

import (
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/riebschlager/mp3-collection/backend/internal/db"
	"github.com/spf13/cobra"
)

type transitionEvent struct {
	TrackID  string
	ArtistID string
	PlayedAt time.Time
}

type edgeAccumulator struct {
	SourceID string
	TargetID string
	Weight   int
}

var computeTransitionsCmd = &cobra.Command{
	Use:   "compute-transitions",
	Short: "Compute track and artist transition graphs",
	Run: func(cmd *cobra.Command, args []string) {
		dbPath, _ := cmd.Flags().GetString("db")
		fmt.Printf("Connecting to database at %s\n", dbPath)

		database, err := db.InitDB(dbPath, "schema.sql")
		if err != nil {
			fmt.Printf("Error opening database: %v\n", err)
			os.Exit(1)
		}
		defer database.Close()

		fmt.Println("Loading chronological listening history...")
		rows, err := database.Query(`
			SELECT h.track_id, t.artist_id, h.played_at
			FROM listening_history h
			JOIN tracks t ON h.track_id = t.id
			ORDER BY h.played_at ASC
		`)
		if err != nil {
			fmt.Printf("Error querying history: %v\n", err)
			os.Exit(1)
		}
		defer rows.Close()

		var events []transitionEvent
		for rows.Next() {
			var ev transitionEvent
			var playedAtStr string
			if err := rows.Scan(&ev.TrackID, &ev.ArtistID, &playedAtStr); err == nil {
				parsed, _ := time.Parse(time.RFC3339, playedAtStr)
				ev.PlayedAt = parsed
				events = append(events, ev)
			}
		}

		fmt.Printf("Loaded %d events. Computing transitions (30 min max gap)...\n", len(events))

		trackEdges := make(map[string]*edgeAccumulator)
		artistEdges := make(map[string]*edgeAccumulator)
		
		trackOutDegrees := make(map[string]int)
		artistOutDegrees := make(map[string]int)

		var prev *transitionEvent
		sessionGap := 30 * time.Minute

		for i := range events {
			cur := &events[i]
			if prev != nil {
				gap := cur.PlayedAt.Sub(prev.PlayedAt)
				if gap <= sessionGap && gap >= 0 {
					// Track transition
					if prev.TrackID != cur.TrackID { // No self loops
						tk := prev.TrackID + ">>" + cur.TrackID
						if trackEdges[tk] == nil {
							trackEdges[tk] = &edgeAccumulator{SourceID: prev.TrackID, TargetID: cur.TrackID}
						}
						trackEdges[tk].Weight++
						trackOutDegrees[prev.TrackID]++
					}

					// Artist transition
					if prev.ArtistID != cur.ArtistID { // No self loops
						ak := prev.ArtistID + ">>" + cur.ArtistID
						if artistEdges[ak] == nil {
							artistEdges[ak] = &edgeAccumulator{SourceID: prev.ArtistID, TargetID: cur.ArtistID}
						}
						artistEdges[ak].Weight++
						artistOutDegrees[prev.ArtistID]++
					}
				}
			}
			prev = cur
		}

		fmt.Println("Clearing existing transition_edges...")
		_, err = database.Exec("DELETE FROM transition_edges")
		if err != nil {
			fmt.Printf("Error clearing table: %v\n", err)
			os.Exit(1)
		}

		tx, err := database.Begin()
		if err != nil {
			fmt.Printf("Error starting tx: %v\n", err)
			os.Exit(1)
		}
		defer tx.Rollback()

		insertStmt, _ := tx.Prepare("INSERT INTO transition_edges (source_type, source_id, target_id, weight, probability) VALUES (?, ?, ?, ?, ?)")

		insertEdges := func(scope string, edges map[string]*edgeAccumulator, outDegrees map[string]int) int {
			// Convert to slice and sort by weight DESC to apply limits
			var allEdges []*edgeAccumulator
			for _, e := range edges {
				allEdges = append(allEdges, e)
			}
			sort.Slice(allEdges, func(i, j int) bool {
				return allEdges[i].Weight > allEdges[j].Weight
			})

			inserted := 0
			for _, e := range allEdges {
				if e.Weight < 2 { // min edge weight
					continue
				}
				if inserted >= 2500 { // max edges per scope
					break
				}
				
				prob := float64(e.Weight) / float64(outDegrees[e.SourceID])
				insertStmt.Exec(scope, e.SourceID, e.TargetID, e.Weight, prob)
				inserted++
			}
			return inserted
		}

		trackCount := insertEdges("track", trackEdges, trackOutDegrees)
		artistCount := insertEdges("artist", artistEdges, artistOutDegrees)

		if err := tx.Commit(); err != nil {
			fmt.Printf("Error committing tx: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Materialized Analytics Tables Updated:\n")
		fmt.Printf("- Inserted %d track edges\n", trackCount)
		fmt.Printf("- Inserted %d artist edges\n", artistCount)
	},
}

func init() {
	rootCmd.AddCommand(computeTransitionsCmd)
	computeTransitionsCmd.Flags().String("db", "../data/mp3_collection.db", "Path to SQLite database")
}
