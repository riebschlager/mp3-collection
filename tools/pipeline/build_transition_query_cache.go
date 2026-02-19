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
	defaultTransitionQueryCacheSessionGapMinutes  = 30
	defaultTransitionQueryCacheMinTransitionCount = 2
	defaultTransitionQueryCacheMaxEdges           = 180
)

type transitionQueryCacheConfig struct {
	SessionGapMinutes  int    `json:"sessionGapMinutes"`
	MinTransitionCount int    `json:"minTransitionCount"`
	MaxEdges           int    `json:"maxEdges"`
	IncludeSelfLoops   bool   `json:"includeSelfLoops"`
	Scope              string `json:"scope"`
}

type transitionQueryCacheScope struct {
	Summary map[string]interface{}   `json:"summary"`
	Nodes   []map[string]interface{} `json:"nodes"`
	Edges   []map[string]interface{} `json:"edges"`
}

type transitionQueryCacheSlice struct {
	Source    string                               `json:"source"`
	Year      int                                  `json:"year"`
	StartDate string                               `json:"startDate"`
	EndDate   string                               `json:"endDate"`
	Summary   map[string]interface{}               `json:"summary"`
	Graphs    map[string]transitionQueryCacheScope `json:"graphs"`
}

type transitionQueryCacheData struct {
	GeneratedAt string                               `json:"generatedAt"`
	Config      transitionQueryCacheConfig           `json:"config"`
	Years       []int                                `json:"years"`
	Sources     []string                             `json:"sources"`
	Slices      map[string]transitionQueryCacheSlice `json:"slices"`
	Summary     map[string]interface{}               `json:"summary"`
}

func runBuildTransitionQueryCache() {
	fmt.Println("Building transition query cache from MCP transition_graph tool...")

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
	for _, y := range timeline.Years {
		years = append(years, y.Year)
	}
	sort.Ints(years)
	if len(years) == 0 {
		fmt.Fprintln(os.Stderr, "No years found in timeline data.")
		os.Exit(1)
	}

	sources := parseTransitionCacheSources(os.Getenv("MP3_TRANSITION_QUERY_SOURCES"))
	config := transitionQueryCacheConfig{
		SessionGapMinutes:  clampInt(int(readEnvInt64("MP3_TRANSITION_QUERY_SESSION_GAP_MINUTES", defaultTransitionQueryCacheSessionGapMinutes)), 5, 180),
		MinTransitionCount: clampInt(int(readEnvInt64("MP3_TRANSITION_QUERY_MIN_EDGE_WEIGHT", defaultTransitionQueryCacheMinTransitionCount)), 1, 1000),
		MaxEdges:           clampInt(int(readEnvInt64("MP3_TRANSITION_QUERY_MAX_EDGES", defaultTransitionQueryCacheMaxEdges)), 20, 50000),
		IncludeSelfLoops:   transitionCacheEnvBool("MP3_TRANSITION_QUERY_INCLUDE_SELF_LOOPS", false),
		Scope:              "both",
	}

	fmt.Printf("Years: %d (%d-%d)\n", len(years), years[0], years[len(years)-1])
	fmt.Printf("Sources: %s\n", strings.Join(sources, ", "))
	fmt.Printf("Config: sessionGap=%d minTransition=%d maxEdges=%d includeSelfLoops=%t\n\n",
		config.SessionGapMinutes, config.MinTransitionCount, config.MaxEdges, config.IncludeSelfLoops)

	client, err := startMCPProcessClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error starting MCP client: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	totalRequests := len(years) * len(sources)
	completed := 0
	failures := make([]map[string]interface{}, 0)
	slices := make(map[string]transitionQueryCacheSlice, totalRequests)

	for _, source := range sources {
		for _, year := range years {
			completed++
			startDate := fmt.Sprintf("%04d-01-01", year)
			endDate := fmt.Sprintf("%04d-12-31", year)
			fmt.Printf("  [%d/%d] source=%s year=%d\n", completed, totalRequests, source, year)

			raw, err := client.callTransitionGraph(startDate, endDate, source, config.SessionGapMinutes, config.MinTransitionCount, config.MaxEdges, config.IncludeSelfLoops)
			if err != nil {
				failures = append(failures, map[string]interface{}{
					"source": source,
					"year":   year,
					"reason": err.Error(),
				})
				continue
			}

			graphs, summary, err := compactTransitionGraphs(raw)
			if err != nil {
				failures = append(failures, map[string]interface{}{
					"source": source,
					"year":   year,
					"reason": err.Error(),
				})
				continue
			}

			key := fmt.Sprintf("%s/%d", source, year)
			slices[key] = transitionQueryCacheSlice{
				Source:    source,
				Year:      year,
				StartDate: startDate,
				EndDate:   endDate,
				Summary:   summary,
				Graphs:    graphs,
			}
		}
	}

	if len(slices) == 0 {
		fmt.Fprintln(os.Stderr, "No transition query slices were generated.")
		if len(failures) > 0 {
			fmt.Fprintf(os.Stderr, "First failure: %v\n", failures[0]["reason"])
		}
		os.Exit(1)
	}

	output := transitionQueryCacheData{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Config:      config,
		Years:       years,
		Sources:     sources,
		Slices:      slices,
		Summary: map[string]interface{}{
			"requested": totalRequests,
			"generated": len(slices),
			"failed":    len(failures),
			"failures":  failures,
		},
	}

	outputPath := WebDataPath("transition-query-cache.json")
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output directory: %v\n", err)
		os.Exit(1)
	}
	writeJSON(outputPath, output)
	fmt.Printf("\nTransition query cache written: %s\n", outputPath)
	if len(failures) > 0 {
		fmt.Printf("Generated %d/%d slices with %d failures.\n", len(slices), totalRequests, len(failures))
	}
}

func parseTransitionCacheSources(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{"all", "lastfm", "spotify"}
	}

	valid := map[string]struct{}{
		"all":     {},
		"lastfm":  {},
		"spotify": {},
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, 3)
	for _, part := range strings.Split(raw, ",") {
		source := strings.ToLower(strings.TrimSpace(part))
		if _, ok := valid[source]; !ok {
			continue
		}
		if _, ok := seen[source]; ok {
			continue
		}
		seen[source] = struct{}{}
		out = append(out, source)
	}
	if len(out) == 0 {
		return []string{"all", "lastfm", "spotify"}
	}
	return out
}

func transitionCacheEnvBool(name string, fallback bool) bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	if raw == "" {
		return fallback
	}
	switch raw {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func clampInt(v, minVal, maxVal int) int {
	if v < minVal {
		return minVal
	}
	if v > maxVal {
		return maxVal
	}
	return v
}

func compactTransitionGraphs(raw map[string]interface{}) (map[string]transitionQueryCacheScope, map[string]interface{}, error) {
	graphsRaw, ok := raw["graphs"].(map[string]interface{})
	if !ok {
		return nil, nil, fmt.Errorf("transition response missing graphs")
	}

	graphs := map[string]transitionQueryCacheScope{}
	for _, scopeName := range []string{"track", "artist"} {
		scopeRaw, ok := graphsRaw[scopeName].(map[string]interface{})
		if !ok {
			continue
		}
		summary, _ := scopeRaw["summary"].(map[string]interface{})
		nodes := pruneTransitionNodes(scopeRaw["nodes"])
		edges := pruneTransitionEdges(scopeRaw["edges"])
		graphs[scopeName] = transitionQueryCacheScope{
			Summary: summary,
			Nodes:   nodes,
			Edges:   edges,
		}
	}

	if len(graphs) == 0 {
		return nil, nil, fmt.Errorf("transition response missing track/artist graph payloads")
	}

	summary, _ := raw["summary"].(map[string]interface{})
	if summary == nil {
		summary = map[string]interface{}{}
	}
	return graphs, summary, nil
}

func pruneTransitionNodes(raw interface{}) []map[string]interface{} {
	items, ok := raw.([]interface{})
	if !ok {
		return []map[string]interface{}{}
	}

	keepKeys := []string{"id", "label", "artist", "track", "playCount", "inDegree", "outDegree", "inWeight", "outWeight"}
	out := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		node, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		kept := map[string]interface{}{}
		for _, key := range keepKeys {
			if value, exists := node[key]; exists {
				kept[key] = value
			}
		}
		if _, hasID := kept["id"]; !hasID {
			continue
		}
		if _, hasLabel := kept["label"]; !hasLabel {
			if id, ok := kept["id"].(string); ok {
				kept["label"] = id
			}
		}
		out = append(out, kept)
	}
	return out
}

func pruneTransitionEdges(raw interface{}) []map[string]interface{} {
	items, ok := raw.([]interface{})
	if !ok {
		return []map[string]interface{}{}
	}

	keepKeys := []string{
		"source", "target", "sourceLabel", "targetLabel",
		"count", "probability", "avgGapMinutes", "minGapMinutes", "maxGapMinutes",
		"firstSeen", "lastSeen",
	}

	out := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		edge, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		kept := map[string]interface{}{}
		for _, key := range keepKeys {
			if value, exists := edge[key]; exists {
				kept[key] = value
			}
		}
		if _, hasSource := kept["source"]; !hasSource {
			continue
		}
		if _, hasTarget := kept["target"]; !hasTarget {
			continue
		}
		out = append(out, kept)
	}
	return out
}

func atoiOrZero(v interface{}) int {
	switch value := v.(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return 0
		}
		return i
	default:
		return 0
	}
}
