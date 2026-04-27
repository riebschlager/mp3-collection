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

	// Tool 3: Compare Eras
	compareErasTool := mcp.NewTool("music_compare_eras",
		mcp.WithDescription("Compare the similarity of music listening between two years"),
		mcp.WithString("year_a", mcp.Required(), mcp.Description("The first year (e.g. 2015)")),
		mcp.WithString("year_b", mcp.Required(), mcp.Description("The second year (e.g. 2023)")),
	)
	s.server.AddTool(compareErasTool, s.handleCompareEras)

	// Tool 4: Listening Patterns (Top Artists by Year)
	patternsTool := mcp.NewTool("music_listening_patterns",
		mcp.WithDescription("Get the top artists played in a specific year"),
		mcp.WithString("year", mcp.Required(), mcp.Description("The year to analyze (e.g. 2015)")),
	)
	s.server.AddTool(patternsTool, s.handleListeningPatterns)

	// Tool 5: Streaks and Bursts (Top Artists by Month)
	streaksTool := mcp.NewTool("music_streaks_and_bursts",
		mcp.WithDescription("Get the top artists played in a specific month to identify listening bursts"),
		mcp.WithString("month", mcp.Required(), mcp.Description("The month to analyze (e.g. 2015-05)")),
	)
	s.server.AddTool(streaksTool, s.handleStreaksAndBursts)
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

func (s *MusicMCPServer) handleCompareEras(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, ok := req.Params.Arguments.(map[string]interface{})
	if !ok {
		return mcp.NewToolResultError("Invalid arguments"), nil
	}
	
	yearA, okA := args["year_a"].(string)
	yearB, okB := args["year_b"].(string)
	if !okA || !okB || yearA == "" || yearB == "" {
		return mcp.NewToolResultError("year_a and year_b are required"), nil
	}

	var score float64
	err := s.db.QueryRow("SELECT similarity_score FROM era_similarities WHERE source_filter = 'all' AND year_a = ? AND year_b = ?", yearA, yearB).Scan(&score)
	if err != nil {
		if err == sql.ErrNoRows {
			return mcp.NewToolResultText(fmt.Sprintf("No similarity data found between %s and %s.", yearA, yearB)), nil
		}
		return mcp.NewToolResultError(fmt.Sprintf("Database error: %v", err)), nil
	}

	result := fmt.Sprintf("Similarity Score between %s and %s: %.2f%%\n\n", yearA, yearB, score*100)
	if score > 0.5 {
		result += "These eras are highly similar in taste."
	} else if score > 0.2 {
		result += "These eras share some common taste but have notable differences."
	} else {
		result += "These eras are vastly different in taste."
	}

	return mcp.NewToolResultText(result), nil
}

func (s *MusicMCPServer) handleListeningPatterns(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, ok := req.Params.Arguments.(map[string]interface{})
	if !ok {
		return mcp.NewToolResultError("Invalid arguments"), nil
	}
	
	year, okYear := args["year"].(string)
	if !okYear || year == "" {
		return mcp.NewToolResultError("year is required"), nil
	}

	query := `
		SELECT a.name, COUNT(*) as play_count 
		FROM listening_history h 
		JOIN tracks t ON h.track_id = t.id 
		JOIN artists a ON t.artist_id = a.id 
		WHERE strftime('%Y', h.played_at) = ? 
		GROUP BY a.id 
		ORDER BY play_count DESC 
		LIMIT 10
	`
	rows, err := s.db.Query(query, year)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Database error: %v", err)), nil
	}
	defer rows.Close()

	output := fmt.Sprintf("Top Artists in %s:\n\n", year)
	count := 0
	for rows.Next() {
		var name string
		var playCount int
		if err := rows.Scan(&name, &playCount); err == nil {
			output += fmt.Sprintf("- %s (%d plays)\n", name, playCount)
			count++
		}
	}

	if count == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("No listening history found for %s.", year)), nil
	}

	return mcp.NewToolResultText(output), nil
}

func (s *MusicMCPServer) handleStreaksAndBursts(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, ok := req.Params.Arguments.(map[string]interface{})
	if !ok {
		return mcp.NewToolResultError("Invalid arguments"), nil
	}
	
	month, okMonth := args["month"].(string)
	if !okMonth || month == "" {
		return mcp.NewToolResultError("month is required"), nil
	}

	query := `
		SELECT a.name, COUNT(*) as play_count 
		FROM listening_history h 
		JOIN tracks t ON h.track_id = t.id 
		JOIN artists a ON t.artist_id = a.id 
		WHERE strftime('%Y-%m', h.played_at) = ? 
		GROUP BY a.id 
		ORDER BY play_count DESC 
		LIMIT 10
	`
	rows, err := s.db.Query(query, month)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Database error: %v", err)), nil
	}
	defer rows.Close()

	output := fmt.Sprintf("Top Artists in %s (Listening Bursts):\n\n", month)
	count := 0
	for rows.Next() {
		var name string
		var playCount int
		if err := rows.Scan(&name, &playCount); err == nil {
			output += fmt.Sprintf("- %s (%d plays)\n", name, playCount)
			count++
		}
	}

	if count == 0 {
		return mcp.NewToolResultText(fmt.Sprintf("No listening history found for %s.", month)), nil
	}

	return mcp.NewToolResultText(output), nil
}
