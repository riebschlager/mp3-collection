package main

import (
	"log"
	"os"

	"github.com/riebschlager/mp3-collection/backend/internal/db"
	"github.com/riebschlager/mp3-collection/backend/internal/mcp"
)

func main() {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "../data/mp3_collection.db"
	}

	database, err := db.InitDB(dbPath, "schema.sql")
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer database.Close()

	// Initialize and run the MCP server over Stdio
	mcpServer := mcp.NewServer(database)
	if err := mcpServer.ServeStdio(); err != nil {
		log.Fatalf("MCP Server failed: %v", err)
	}
}
