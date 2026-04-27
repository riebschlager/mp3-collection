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
}

func (s *Server) handleGetArtists(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query("SELECT id, name, slug FROM artists ORDER BY name ASC LIMIT 100")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type Artist struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Slug string `json:"slug"`
	}

	var artists []Artist
	for rows.Next() {
		var a Artist
		if err := rows.Scan(&a.ID, &a.Name, &a.Slug); err != nil {
			continue
		}
		artists = append(artists, a)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(artists)
}

func (s *Server) handleGetArtist(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	
	type Artist struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Slug string `json:"slug"`
	}

	var a Artist
	err := s.db.QueryRow("SELECT id, name, slug FROM artists WHERE id = ?", id).Scan(&a.ID, &a.Name, &a.Slug)
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
		SELECT t.id, t.title, a.name as artist, al.title as album, t.duration_ms
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
		ID         string `json:"id"`
		Title      string `json:"title"`
		Artist     string `json:"artist"`
		Album      string `json:"album"`
		DurationMs int    `json:"duration_ms"`
	}

	var tracks []Track
	for rows.Next() {
		var t Track
		if err := rows.Scan(&t.ID, &t.Title, &t.Artist, &t.Album, &t.DurationMs); err != nil {
			continue
		}
		tracks = append(tracks, t)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tracks)
}
