package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
)

type Server struct {
	db *sql.DB
}

func NewServer(db *sql.DB) *Server {
	return &Server{db: db}
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/artists", s.handleGetArtists)
	mux.HandleFunc("GET /api/v1/artists/{id}", s.handleGetArtist)
	mux.HandleFunc("GET /api/v1/tracks", s.handleGetTracks)
	mux.HandleFunc("GET /api/v1/stats/summary", s.handleGetSummary)
	mux.HandleFunc("GET /api/v1/stats/top-artists", s.handleGetTopArtists)
	mux.HandleFunc("GET /api/v1/stats/top-tracks", s.handleGetTopTracks)
}

func (s *Server) handleGetArtists(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query("SELECT id, name, slug, image_url FROM artists ORDER BY name ASC LIMIT 100")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type Artist struct {
		ID       string  `json:"id"`
		Name     string  `json:"name"`
		Slug     string  `json:"slug"`
		ImageURL *string `json:"image_url"`
	}

	var artists []Artist
	for rows.Next() {
		var a Artist
		if err := rows.Scan(&a.ID, &a.Name, &a.Slug, &a.ImageURL); err != nil {
			continue
		}
		artists = append(artists, a)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(artists)
}

func (s *Server) handleGetArtist(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	
	var a struct {
		ID       string  `json:"id"`
		Name     string  `json:"name"`
		Slug     string  `json:"slug"`
		ImageURL *string `json:"image_url"`
	}
	err := s.db.QueryRow("SELECT id, name, slug, image_url FROM artists WHERE id = ?", id).Scan(&a.ID, &a.Name, &a.Slug, &a.ImageURL)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	} else if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(a)
}

func (s *Server) handleGetTracks(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}

	rows, err := s.db.Query(`
		SELECT t.id, t.title, a.name as artist, al.title as album, t.duration_ms, al.image_url
		FROM tracks t
		JOIN artists a ON t.artist_id = a.id
		JOIN albums al ON t.album_id = al.id
		ORDER BY t.title ASC
		LIMIT ?
	`, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type Track struct {
		ID         string  `json:"id"`
		Title      string  `json:"title"`
		Artist     string  `json:"artist"`
		Album      string  `json:"album"`
		DurationMs int     `json:"duration_ms"`
		ImageURL   *string `json:"image_url"`
	}

	var tracks []Track
	for rows.Next() {
		var t Track
		if err := rows.Scan(&t.ID, &t.Title, &t.Artist, &t.Album, &t.DurationMs, &t.ImageURL); err != nil {
			continue
		}
		tracks = append(tracks, t)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tracks)
}

func (s *Server) handleGetSummary(w http.ResponseWriter, r *http.Request) {
	var totalTracks, totalArtists, totalAlbums, totalScrobbles int
	s.db.QueryRow("SELECT COUNT(*) FROM tracks").Scan(&totalTracks)
	s.db.QueryRow("SELECT COUNT(*) FROM artists").Scan(&totalArtists)
	s.db.QueryRow("SELECT COUNT(*) FROM albums").Scan(&totalAlbums)
	s.db.QueryRow("SELECT COUNT(*) FROM listening_history").Scan(&totalScrobbles)

	res := map[string]int{
		"total_tracks":    totalTracks,
		"total_artists":   totalArtists,
		"total_albums":    totalAlbums,
		"total_scrobbles": totalScrobbles,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func (s *Server) handleGetTopArtists(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(`
		SELECT a.id, a.name, a.image_url, COUNT(*) as play_count
		FROM listening_history h
		JOIN tracks t ON h.track_id = t.id
		JOIN artists a ON t.artist_id = a.id
		GROUP BY a.id
		ORDER BY play_count DESC
		LIMIT 20
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var artists []map[string]interface{}
	for rows.Next() {
		var id, name string
		var imageURL sql.NullString
		var playCount int
		rows.Scan(&id, &name, &imageURL, &playCount)
		
		var img *string
		if imageURL.Valid {
			img = &imageURL.String
		}

		artists = append(artists, map[string]interface{}{
			"id":         id,
			"name":       name,
			"image_url":  img,
			"play_count": playCount,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(artists)
}

func (s *Server) handleGetTopTracks(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(`
		SELECT t.id, t.title, ar.name as artist, al.title as album, al.image_url, COUNT(*) as play_count
		FROM listening_history h
		JOIN tracks t ON h.track_id = t.id
		JOIN artists ar ON t.artist_id = ar.id
		JOIN albums al ON t.album_id = al.id
		GROUP BY t.id
		ORDER BY play_count DESC
		LIMIT 20
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var tracks []map[string]interface{}
	for rows.Next() {
		var id, title, artist, album string
		var imageURL sql.NullString
		var playCount int
		rows.Scan(&id, &title, &artist, &album, &imageURL, &playCount)

		var img *string
		if imageURL.Valid {
			img = &imageURL.String
		}

		tracks = append(tracks, map[string]interface{}{
			"id":         id,
			"title":      title,
			"artist":     artist,
			"album":      album,
			"image_url":  img,
			"play_count": playCount,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tracks)
}
