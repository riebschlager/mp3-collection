package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const defaultStreaksTopN = 10

type streaksCacheData struct {
	GeneratedAt   string                            `json:"generatedAt"`
	Source        string                            `json:"source"`
	Timezone      string                            `json:"timezone"`
	TopN          int                               `json:"topN"`
	Years         []int                             `json:"years"`
	StreaksByYear map[string]map[string]interface{} `json:"streaksByYear"`
}

func runBuildStreaksCache() {
	fmt.Println("Building streaks and bursts cache from MCP tool...")

	timelinePath := WebDataPath("timeline.json")
	if _, err := os.Stat(timelinePath); os.IsNotExist(err) {
		fmt.Println("Timeline data missing; running build-timeline first...")
		runBuildTimeline()
	}

	timelineRaw, err := os.ReadFile(timelinePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading timeline file: %v\n", err)
		os.Exit(1)
	}
	var timeline TimelineData
	if err := json.Unmarshal(timelineRaw, &timeline); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing timeline JSON: %v\n", err)
		os.Exit(1)
	}

	years := make([]int, 0, len(timeline.Years))
	for _, year := range timeline.Years {
		years = append(years, year.Year)
	}
	sort.Ints(years)
	if len(years) == 0 {
		fmt.Fprintln(os.Stderr, "No years found in timeline data.")
		os.Exit(1)
	}

	source := "all"
	if envSource := strings.TrimSpace(os.Getenv("MP3_STREAKS_SOURCE")); envSource != "" {
		source = strings.ToLower(envSource)
	}
	timezone := "UTC"
	if envTz := strings.TrimSpace(os.Getenv("MP3_STREAKS_TIMEZONE")); envTz != "" {
		timezone = envTz
	}
	topN := defaultStreaksTopN
	if envTopN := readEnvInt64("MP3_STREAKS_TOP_N", int64(defaultStreaksTopN)); envTopN > 0 {
		topN = int(envTopN)
	}

	fmt.Printf("Years: %d (%d-%d)\n", len(years), years[0], years[len(years)-1])
	fmt.Printf("Config: source=%s timezone=%s topN=%d\n\n", source, timezone, topN)

	client, err := startMCPProcessClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error starting MCP client: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	streaksByYear := make(map[string]map[string]interface{}, len(years))

	for idx, year := range years {
		fmt.Printf("  [%d/%d] Fetching streaks for %d...\n", idx+1, len(years), year)

		eraLabel := fmt.Sprintf("%d", year)
		startDate := fmt.Sprintf("%04d-01-01", year)
		endDate := fmt.Sprintf("%04d-12-31", year)

		result, callErr := client.callStreaksAndBursts(eraLabel, startDate, endDate, source, timezone, topN)
		if callErr != nil {
			fmt.Fprintf(os.Stderr, "Error generating streaks for %d: %v\n", year, callErr)
			os.Exit(1)
		}

		streaksByYear[eraLabel] = result
	}

	output := streaksCacheData{
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		Source:        source,
		Timezone:      timezone,
		TopN:          topN,
		Years:         years,
		StreaksByYear: streaksByYear,
	}

	outputPath := WebDataPath("streaks-cache.json")
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output directory: %v\n", err)
		os.Exit(1)
	}
	writeJSON(outputPath, output)
	fmt.Printf("\nStreaks cache artifact written: %s\n", outputPath)
}
