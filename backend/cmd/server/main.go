package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/riebschlager/mp3-collection/backend/internal/api"
	"github.com/riebschlager/mp3-collection/backend/internal/db"
)

func main() {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "../data/mp3_collection.db"
	}

	fmt.Printf("Connecting to database at %s\n", dbPath)
	database, err := db.InitDB(dbPath, "schema.sql")
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()

	mux := http.NewServeMux()
	
	// Register API routes
	apiServer := api.NewServer(database)
	apiServer.RegisterRoutes(mux)

	// In a real application, you'd add middleware here (CORS, Logging, etc.)
	corsMux := enableCORS(mux)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Printf("Server listening on port %s\n", port)
	if err := http.ListenAndServe(":"+port, corsMux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func enableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
