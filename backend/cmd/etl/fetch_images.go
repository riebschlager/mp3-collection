package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/riebschlager/mp3-collection/backend/internal/db"
	"github.com/spf13/cobra"
)

type lastFmImage struct {
	Size string `json:"size"`
	Text string `json:"#text"`
}

type lastFmArtistInfoResponse struct {
	Artist struct {
		Image []lastFmImage `json:"image"`
	} `json:"artist"`
}

type lastFmAlbumInfoResponse struct {
	Album struct {
		Image []lastFmImage `json:"image"`
	} `json:"album"`
}

var fetchImagesCmd = &cobra.Command{
	Use:   "fetch-images",
	Short: "Fetch artist and album artwork from Last.fm",
	Run: func(cmd *cobra.Command, args []string) {
		apiKey := os.Getenv("LASTFM_API_KEY")
		if apiKey == "" {
			fmt.Println("Error: LASTFM_API_KEY not set")
			os.Exit(1)
		}

		dbPath, _ := cmd.Flags().GetString("db")
		database, err := db.InitDB(dbPath, "schema.sql")
		if err != nil {
			fmt.Printf("Error opening database: %v\n", err)
			os.Exit(1)
		}
		defer database.Close()

		client := &http.Client{Timeout: 10 * time.Second}

		// 1. Fetch Artist Images
		fmt.Println("Fetching missing artist images...")
		rows, _ := database.Query("SELECT id, name FROM artists WHERE image_url IS NULL")
		var artistsToFetch []struct{ ID, Name string }
		for rows.Next() {
			var a struct{ ID, Name string }
			rows.Scan(&a.ID, &a.Name)
			artistsToFetch = append(artistsToFetch, a)
		}
		rows.Close()

		for _, a := range artistsToFetch {
			fmt.Printf("Fetching artist: %s... ", a.Name)
			imageURL := fetchArtistImage(client, apiKey, a.Name)
			if imageURL != "" {
				database.Exec("UPDATE artists SET image_url = ? WHERE id = ?", imageURL, a.ID)
				fmt.Println("Done")
			} else {
				fmt.Println("Not found")
			}
			time.Sleep(100 * time.Millisecond)
		}

		// 2. Fetch Album Images
		fmt.Println("\nFetching missing album images...")
		rows, _ = database.Query(`
			SELECT al.id, al.title, ar.name 
			FROM albums al 
			JOIN artists ar ON al.artist_id = ar.id 
			WHERE al.image_url IS NULL
		`)
		var albumsToFetch []struct{ ID, Title, Artist string }
		for rows.Next() {
			var al struct{ ID, Title, Artist string }
			rows.Scan(&al.ID, &al.Title, &al.Artist)
			albumsToFetch = append(albumsToFetch, al)
		}
		rows.Close()

		for _, al := range albumsToFetch {
			fmt.Printf("Fetching album: %s - %s... ", al.Artist, al.Title)
			imageURL := fetchAlbumImage(client, apiKey, al.Artist, al.Title)
			if imageURL != "" {
				database.Exec("UPDATE albums SET image_url = ? WHERE id = ?", imageURL, al.ID)
				fmt.Println("Done")
			} else {
				fmt.Println("Not found")
			}
			time.Sleep(100 * time.Millisecond)
		}
	},
}

func fetchArtistImage(client *http.Client, apiKey, artist string) string {
	u := fmt.Sprintf("https://ws.audioscrobbler.com/2.0/?method=artist.getinfo&artist=%s&api_key=%s&format=json",
		url.QueryEscape(artist), apiKey)
	resp, err := client.Get(u)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	var res lastFmArtistInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return ""
	}

	for _, img := range res.Artist.Image {
		if img.Size == "extralarge" || img.Size == "large" {
			if img.Text != "" && !strings.Contains(img.Text, "2a96cdf8347e55ef58604093a0a83b3e") {
				return img.Text
			}
		}
	}
	return ""
}

func fetchAlbumImage(client *http.Client, apiKey, artist, album string) string {
	u := fmt.Sprintf("https://ws.audioscrobbler.com/2.0/?method=album.getinfo&artist=%s&album=%s&api_key=%s&format=json",
		url.QueryEscape(artist), url.QueryEscape(album), apiKey)
	resp, err := client.Get(u)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	var res lastFmAlbumInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return ""
	}

	for _, img := range res.Album.Image {
		if img.Size == "extralarge" || img.Size == "large" {
			if img.Text != "" && !strings.Contains(img.Text, "2a96cdf8347e55ef58604093a0a83b3e") {
				return img.Text
			}
		}
	}
	return ""
}

func init() {
	rootCmd.AddCommand(fetchImagesCmd)
	fetchImagesCmd.Flags().String("db", "../data/mp3_collection.db", "Path to SQLite database")
}
