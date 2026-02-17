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

type wrappedMonthStoriesData struct {
	GeneratedAt       string                            `json:"generatedAt"`
	Timezone          string                            `json:"timezone"`
	Source            string                            `json:"source"`
	DiscoveryBaseline string                            `json:"discoveryBaseline"`
	TopN              int                               `json:"topN"`
	Months            []string                          `json:"months"`
	Stories           map[string]map[string]interface{} `json:"stories"`
	Summary           map[string]interface{}            `json:"summary"`
}

func runBuildWrappedMonthStories() {
	timelinePath := WebDataPath("timeline.json")
	outputPath := WebDataPath("wrapped-month-stories.json")
	topN := defaultWrappedTopN
	timezone := strings.TrimSpace(os.Getenv("MP3_WRAPPED_TIMEZONE"))
	if timezone == "" {
		timezone = defaultWrappedTimezone
	}
	source := normalizeWrappedSource(os.Getenv("MP3_WRAPPED_SOURCE"))
	discoveryBaseline := normalizeWrappedDiscoveryBaseline(os.Getenv("MP3_WRAPPED_DISCOVERY_BASELINE"))
	includeDormantReturns := false
	if raw := strings.TrimSpace(os.Getenv("MP3_WRAPPED_MONTH_INCLUDE_DORMANT")); raw != "" {
		switch strings.ToLower(raw) {
		case "1", "true", "yes":
			includeDormantReturns = true
		case "0", "false", "no":
			includeDormantReturns = false
		default:
			fmt.Printf("Warning: unsupported MP3_WRAPPED_MONTH_INCLUDE_DORMANT=%q; defaulting to false\n", raw)
		}
	}

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

	monthSet := map[string]struct{}{}
	for _, year := range timeline.Years {
		for _, month := range year.Months {
			if strings.TrimSpace(month.Month) == "" {
				continue
			}
			monthSet[month.Month] = struct{}{}
		}
	}
	months := make([]string, 0, len(monthSet))
	for month := range monthSet {
		months = append(months, month)
	}
	sort.Strings(months)
	if len(months) == 0 {
		fmt.Fprintln(os.Stderr, "No months found in timeline data.")
		os.Exit(1)
	}

	fmt.Printf("Generating wrapped month stories for %d months (%s to %s)\n", len(months), months[0], months[len(months)-1])
	fmt.Printf("Timezone=%s Source=%s DiscoveryBaseline=%s TopN=%d IncludeDormantReturns=%t\n\n",
		timezone, source, discoveryBaseline, topN, includeDormantReturns)

	client, err := startMCPProcessClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error starting MCP client: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	stories := make(map[string]map[string]interface{}, len(months))
	failures := make([]map[string]interface{}, 0)
	for idx, monthKey := range months {
		year, month, parseErr := parseMonthKey(monthKey)
		if parseErr != nil {
			failures = append(failures, map[string]interface{}{
				"month":  monthKey,
				"reason": parseErr.Error(),
			})
			continue
		}
		fmt.Printf("  [%d/%d] %s\n", idx+1, len(months), monthKey)
		story, storyErr := client.callMonthStory(year, month, topN, timezone, source, discoveryBaseline, includeDormantReturns)
		if storyErr != nil {
			failures = append(failures, map[string]interface{}{
				"month":  monthKey,
				"reason": storyErr.Error(),
			})
			continue
		}
		stories[monthKey] = story
	}

	if len(stories) == 0 {
		fmt.Fprintln(os.Stderr, "No monthly stories were generated.")
		if len(failures) > 0 {
			fmt.Fprintf(os.Stderr, "First failure: %v\n", failures[0]["reason"])
		}
		os.Exit(1)
	}

	generatedMonths := make([]string, 0, len(stories))
	for month := range stories {
		generatedMonths = append(generatedMonths, month)
	}
	sort.Strings(generatedMonths)

	payload := wrappedMonthStoriesData{
		GeneratedAt:       time.Now().UTC().Format(time.RFC3339),
		Timezone:          timezone,
		Source:            source,
		DiscoveryBaseline: discoveryBaseline,
		TopN:              topN,
		Months:            generatedMonths,
		Stories:           stories,
		Summary: map[string]interface{}{
			"requestedCount":        len(months),
			"generatedCount":        len(generatedMonths),
			"failedCount":           len(failures),
			"failedMonths":          failures,
			"includeDormantReturns": includeDormantReturns,
		},
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output directory: %v\n", err)
		os.Exit(1)
	}
	writeJSON(outputPath, payload)

	fmt.Printf("\nWrapped month story artifact written: %s\n", outputPath)
	if len(failures) > 0 {
		fmt.Printf("Generated %d/%d months with %d failures.\n", len(generatedMonths), len(months), len(failures))
	}
}

func parseMonthKey(monthKey string) (int, int, error) {
	parts := strings.Split(monthKey, "-")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid month key %q", monthKey)
	}
	year, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid year in month key %q", monthKey)
	}
	month, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid month in month key %q", monthKey)
	}
	if year < 1900 || year > 2100 || month < 1 || month > 12 {
		return 0, 0, fmt.Errorf("month key out of range %q", monthKey)
	}
	return year, month, nil
}
