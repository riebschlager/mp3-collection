package mcp

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type MusicMCPServer struct {
	db     *sql.DB
	server *server.MCPServer
}

func NewServer(db *sql.DB) *MusicMCPServer {
	s := server.NewMCPServer("music-intel-mcp-v2", "0.2.0")
	ms := &MusicMCPServer{
		db:     db,
		server: s,
	}
	ms.registerTools()
	return ms
}

func (s *MusicMCPServer) ServeStdio() error {
	log.Println("Starting MCP server on stdio")
	return server.ServeStdio(s.server)
}

func (s *MusicMCPServer) registerTools() {
	// Tool 1: Listening Summary
	summaryTool := mcp.NewTool("music_listening_summary",
		mcp.WithDescription("Get a high-level summary of the user's listening history"),
	)
	s.server.AddTool(summaryTool, s.handleListeningSummary)

	// Tool 2: Artist Search
	artistTool := mcp.NewTool("music_artist_search",
		mcp.WithDescription("Search for an artist in the database"),
		mcp.WithString("query", mcp.Required(), mcp.Description("The artist name to search for")),
	)
	s.server.AddTool(artistTool, s.handleArtistSearch)
}

func (s *MusicMCPServer) handleListeningSummary(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var totalTracks int
	var totalArtists int
	var totalScrobbles int

	s.db.QueryRow("SELECT COUNT(*) FROM tracks").Scan(&totalTracks)
	s.db.QueryRow("SELECT COUNT(*) FROM artists").Scan(&totalArtists)
	s.db.QueryRow("SELECT COUNT(*) FROM listening_history").Scan(&totalScrobbles)

	result := fmt.Sprintf(`Listening History Summary:
- Total Unique Tracks: %d
- Total Unique Artists: %d
- Total Scrobbles: %d`, totalTracks, totalArtists, totalScrobbles)

	return mcp.NewToolResultText(result), nil
}

func (s *MusicMCPServer) handleArtistSearch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, ok := req.Params.Arguments.(map[string]interface{})
	if !ok {
		return mcp.NewToolResultError("Invalid arguments"), nil
	}
	
	query, ok := args["query"].(string)
	if !ok || query == "" {
		return mcp.NewToolResultError("Query string is required"), nil
	}

	rows, err := s.db.Query("SELECT name FROM artists WHERE name LIKE ? LIMIT 10", "%"+query+"%")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Database error: %v", err)), nil
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil {
			results = append(results, name)
		}
	}

	if len(results) == 0 {
		return mcp.NewToolResultText("No artists found matching: " + query), nil
	}

	output := "Found Artists:\n"
	for _, name := range results {
		output += "- " + name + "\n"
	}

	return mcp.NewToolResultText(output), nil
}
