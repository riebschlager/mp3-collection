package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultWrappedTopN              = 10
	defaultWrappedTimezone          = "UTC"
	defaultWrappedSource            = "all"
	defaultWrappedDiscoveryBaseline = "global"
)

type wrappedStoriesData struct {
	GeneratedAt       string                            `json:"generatedAt"`
	Timezone          string                            `json:"timezone"`
	Source            string                            `json:"source"`
	DiscoveryBaseline string                            `json:"discoveryBaseline"`
	TopN              int                               `json:"topN"`
	Years             []int                             `json:"years"`
	Stories           map[string]map[string]interface{} `json:"stories"`
	BatchSummary      map[string]interface{}            `json:"batchSummary,omitempty"`
}

func runBuildWrappedStories() {
	timelinePath := WebDataPath("timeline.json")
	outputPath := WebDataPath("wrapped-stories.json")
	topN := defaultWrappedTopN
	timezone := strings.TrimSpace(os.Getenv("MP3_WRAPPED_TIMEZONE"))
	if timezone == "" {
		timezone = defaultWrappedTimezone
	}
	source := normalizeWrappedSource(os.Getenv("MP3_WRAPPED_SOURCE"))
	discoveryBaseline := normalizeWrappedDiscoveryBaseline(os.Getenv("MP3_WRAPPED_DISCOVERY_BASELINE"))

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
	for _, y := range timeline.Years {
		years = append(years, y.Year)
	}
	sort.Ints(years)
	if len(years) == 0 {
		fmt.Fprintln(os.Stderr, "No years found in timeline data.")
		os.Exit(1)
	}

	fmt.Printf("Generating wrapped stories for %d years (%d-%d)\n", len(years), years[0], years[len(years)-1])
	fmt.Printf("Timezone=%s Source=%s DiscoveryBaseline=%s TopN=%d\n\n", timezone, source, discoveryBaseline, topN)

	client, err := startMCPProcessClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error starting MCP client: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	fmt.Println("Running batch MCP request...")
	stories, batchSummary, err := client.callBatchYearStory(years, topN, timezone, source, discoveryBaseline)
	if err != nil {
		fmt.Printf("Batch MCP request failed: %v\n", err)
		fmt.Println("Falling back to per-year MCP requests...")
		stories = make(map[string]map[string]interface{}, len(years))
		for idx, year := range years {
			fmt.Printf("  [%d/%d] %d\n", idx+1, len(years), year)
			story, fallbackErr := client.callYearStory(year, topN, timezone, source, discoveryBaseline)
			if fallbackErr != nil {
				fmt.Fprintf(os.Stderr, "Error generating story for %d: %v\n", year, fallbackErr)
				os.Exit(1)
			}
			stories[strconv.Itoa(year)] = story
		}
	} else if len(stories) < len(years) {
		fmt.Printf("Batch MCP request returned %d/%d years; filling gaps via per-year fallback...\n", len(stories), len(years))
		for _, year := range years {
			key := strconv.Itoa(year)
			if _, exists := stories[key]; exists {
				continue
			}
			fmt.Printf("  [fallback] %d\n", year)
			story, fallbackErr := client.callYearStory(year, topN, timezone, source, discoveryBaseline)
			if fallbackErr != nil {
				fmt.Fprintf(os.Stderr, "Error generating story for %d: %v\n", year, fallbackErr)
				os.Exit(1)
			}
			stories[key] = story
		}
	}

	payload := wrappedStoriesData{
		GeneratedAt:       time.Now().UTC().Format(time.RFC3339),
		Timezone:          timezone,
		Source:            source,
		DiscoveryBaseline: discoveryBaseline,
		TopN:              topN,
		Years:             years,
		Stories:           stories,
		BatchSummary:      batchSummary,
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output directory: %v\n", err)
		os.Exit(1)
	}
	writeJSON(outputPath, payload)
	fmt.Printf("\nWrapped story artifact written: %s\n", outputPath)
}

func normalizeWrappedSource(raw string) string {
	source := strings.ToLower(strings.TrimSpace(raw))
	switch source {
	case "", "all", "both":
		return "all"
	case "lastfm", "spotify":
		return source
	default:
		fmt.Printf("Warning: unsupported MP3_WRAPPED_SOURCE=%q; defaulting to %q\n", raw, defaultWrappedSource)
		return defaultWrappedSource
	}
}

func normalizeWrappedDiscoveryBaseline(raw string) string {
	baseline := strings.ToLower(strings.TrimSpace(raw))
	switch baseline {
	case "", "global":
		return "global"
	case "source", "window":
		return baseline
	default:
		fmt.Printf("Warning: unsupported MP3_WRAPPED_DISCOVERY_BASELINE=%q; defaulting to %q\n", raw, defaultWrappedDiscoveryBaseline)
		return defaultWrappedDiscoveryBaseline
	}
}
