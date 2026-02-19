package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	defaultEraSimilarityTopN = 20
)

type eraSimilarityCacheConfig struct {
	TopN int `json:"topN"`
}

type eraSimilarityPair struct {
	EraA                int                      `json:"eraA"`
	EraB                int                      `json:"eraB"`
	SimilarityIndex     float64                  `json:"similarityIndex"`
	Confidence          float64                  `json:"confidence"`
	ConfidenceBand      string                   `json:"confidenceBand"`
	Components          map[string]float64       `json:"components"`
	Overlap             map[string]float64       `json:"overlap"`
	Novelty             map[string]float64       `json:"novelty"`
	Diversity           map[string]float64       `json:"diversity"`
	GenreSimilarity     float64                  `json:"genreSimilarity"`
	PersistentFavorites []map[string]interface{} `json:"persistentFavorites"`
	Rising              []map[string]interface{} `json:"rising"`
	Falling             []map[string]interface{} `json:"falling"`
	InsightBullets      []string                 `json:"insightBullets"`
}

type eraSimilarityCacheData struct {
	GeneratedAt string                                  `json:"generatedAt"`
	Years       []int                                   `json:"years"`
	Sources     []string                                `json:"sources"`
	Config      eraSimilarityCacheConfig                `json:"config"`
	Matrices    map[string][][]float64                  `json:"matrices"`
	Details     map[string]map[string]eraSimilarityPair `json:"details"`
	Summary     map[string]interface{}                  `json:"summary"`
}

func runBuildEraSimilarityCache() {
	fmt.Println("Building era similarity cache from MCP compare_eras tool...")

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

	sources := parseEraSimilaritySources(os.Getenv("MP3_ERA_SIMILARITY_SOURCES"))
	config := eraSimilarityCacheConfig{
		TopN: clampInt(int(readEnvInt64("MP3_ERA_SIMILARITY_TOP_N", defaultEraSimilarityTopN)), 5, 100),
	}

	fmt.Printf("Years: %d (%d-%d)\n", len(years), years[0], years[len(years)-1])
	fmt.Printf("Sources: %s\n", strings.Join(sources, ", "))
	fmt.Printf("Config: topN=%d\n\n", config.TopN)

	client, err := startMCPProcessClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error starting MCP client: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	matrices := make(map[string][][]float64, len(sources))
	details := make(map[string]map[string]eraSimilarityPair, len(sources))
	failures := make([]map[string]interface{}, 0)

	requestedPairs := len(sources) * ((len(years) * (len(years) - 1)) / 2)
	generatedPairs := 0
	current := 0

	for _, source := range sources {
		matrix := make([][]float64, len(years))
		for i := range matrix {
			matrix[i] = make([]float64, len(years))
			matrix[i][i] = 100
		}
		matrices[source] = matrix
		details[source] = make(map[string]eraSimilarityPair, len(years)*(len(years)+1)/2)

		for i := range years {
			year := years[i]
			details[source][pairKey(year, year)] = identityEraSimilarityPair(year)
		}

		for i := 0; i < len(years); i++ {
			yearA := years[i]
			startA := fmt.Sprintf("%04d-01-01", yearA)
			endA := fmt.Sprintf("%04d-12-31", yearA)

			for j := i + 1; j < len(years); j++ {
				yearB := years[j]
				startB := fmt.Sprintf("%04d-01-01", yearB)
				endB := fmt.Sprintf("%04d-12-31", yearB)

				current++
				fmt.Printf("  [%d/%d] source=%s %d vs %d\n", current, requestedPairs, source, yearA, yearB)

				raw, callErr := client.callCompareEras(startA, endA, fmt.Sprintf("%d", yearA), startB, endB, fmt.Sprintf("%d", yearB), source, config.TopN)
				if callErr != nil {
					failures = append(failures, map[string]interface{}{
						"source": source,
						"eraA":   yearA,
						"eraB":   yearB,
						"reason": callErr.Error(),
					})
					continue
				}

				pair := compactEraSimilarityPair(raw, yearA, yearB)
				score := pair.SimilarityIndex
				matrix[i][j] = score
				matrix[j][i] = score
				details[source][pairKey(yearA, yearB)] = pair
				generatedPairs++
			}
		}
	}

	output := eraSimilarityCacheData{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Years:       years,
		Sources:     sources,
		Config:      config,
		Matrices:    matrices,
		Details:     details,
		Summary: map[string]interface{}{
			"requestedPairs": requestedPairs,
			"generatedPairs": generatedPairs,
			"failedPairs":    len(failures),
			"failures":       failures,
		},
	}

	outputPath := WebDataPath("era-similarity-cache.json")
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output directory: %v\n", err)
		os.Exit(1)
	}
	writeJSON(outputPath, output)
	fmt.Printf("\nEra similarity cache written: %s\n", outputPath)
	if len(failures) > 0 {
		fmt.Printf("Generated %d/%d pairs with %d failures.\n", generatedPairs, requestedPairs, len(failures))
	}
}

func parseEraSimilaritySources(raw string) []string {
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

func pairKey(a, b int) string {
	if a > b {
		a, b = b, a
	}
	return fmt.Sprintf("%d/%d", a, b)
}

func identityEraSimilarityPair(year int) eraSimilarityPair {
	components := map[string]float64{
		"artistOverlap":      1,
		"trackOverlap":       1,
		"genreSimilarity":    1,
		"noveltyAlignment":   1,
		"diversityAlignment": 1,
		"persistentStrength": 1,
	}
	return eraSimilarityPair{
		EraA:            year,
		EraB:            year,
		SimilarityIndex: 100,
		Confidence:      1,
		ConfidenceBand:  "high",
		Components:      components,
		Overlap: map[string]float64{
			"artistJaccard": 1,
			"trackJaccard":  1,
		},
		Novelty: map[string]float64{
			"eraA":      1,
			"eraB":      1,
			"alignment": 1,
		},
		Diversity: map[string]float64{
			"eraA":      1,
			"eraB":      1,
			"alignment": 1,
		},
		GenreSimilarity:     1,
		PersistentFavorites: []map[string]interface{}{},
		Rising:              []map[string]interface{}{},
		Falling:             []map[string]interface{}{},
		InsightBullets:      []string{"Same-year comparison baseline (identity score)."},
	}
}

func compactEraSimilarityPair(raw map[string]interface{}, eraA, eraB int) eraSimilarityPair {
	summary, _ := raw["summary"].(map[string]interface{})
	overlap, _ := raw["overlap"].(map[string]interface{})
	similarity, _ := raw["similarity"].(map[string]interface{})
	componentsRaw, _ := similarity["components"].(map[string]interface{})

	score := roundFloat(readFloat(similarity, "eraSimilarityIndex", readFloat(summary, "eraSimilarity", 0)), 2)
	confidence := roundFloat(readFloat(similarity, "confidence", readFloat(summary, "similarityConfidence", 0)), 4)
	confidenceBand := strings.TrimSpace(readString(similarity, "confidenceBand"))
	if confidenceBand == "" {
		confidenceBand = similarityBandFromConfidence(confidence)
	}

	noveltyA := readFloat(summary, "noveltyRateA", 0)
	noveltyB := readFloat(summary, "noveltyRateB", 0)
	diversityA := readFloat(summary, "diversityEntropyA", 0)
	diversityB := readFloat(summary, "diversityEntropyB", 0)

	components := map[string]float64{
		"artistOverlap":      roundFloat(readFloat(componentsRaw, "artistOverlap", readFloat(overlap, "artistJaccard", 0)), 4),
		"trackOverlap":       roundFloat(readFloat(componentsRaw, "trackOverlap", readFloat(overlap, "trackJaccard", 0)), 4),
		"genreSimilarity":    roundFloat(readFloat(componentsRaw, "genreSimilarity", readFloat(summary, "genreSimilarity", 0)), 4),
		"noveltyAlignment":   roundFloat(readFloat(componentsRaw, "noveltyAlignment", clamp01(1-absFloat(noveltyA-noveltyB))), 4),
		"diversityAlignment": roundFloat(readFloat(componentsRaw, "diversityAlignment", clamp01(1-absFloat(diversityA-diversityB))), 4),
		"persistentStrength": roundFloat(readFloat(componentsRaw, "persistentStrength", 0), 4),
	}

	return eraSimilarityPair{
		EraA:            eraA,
		EraB:            eraB,
		SimilarityIndex: score,
		Confidence:      confidence,
		ConfidenceBand:  confidenceBand,
		Components:      components,
		Overlap: map[string]float64{
			"artistJaccard": roundFloat(readFloat(overlap, "artistJaccard", 0), 4),
			"trackJaccard":  roundFloat(readFloat(overlap, "trackJaccard", 0), 4),
		},
		Novelty: map[string]float64{
			"eraA":      roundFloat(noveltyA, 4),
			"eraB":      roundFloat(noveltyB, 4),
			"alignment": roundFloat(components["noveltyAlignment"], 4),
		},
		Diversity: map[string]float64{
			"eraA":      roundFloat(diversityA, 4),
			"eraB":      roundFloat(diversityB, 4),
			"alignment": roundFloat(components["diversityAlignment"], 4),
		},
		GenreSimilarity:     roundFloat(components["genreSimilarity"], 4),
		PersistentFavorites: compactMapList(overlap["persistentFavorites"], 4, []string{"artist", "track", "countA", "countB", "rateA", "rateB"}),
		Rising:              compactMapList(raw["rising"], 4, []string{"artist", "track", "delta", "countA", "countB", "rateA", "rateB"}),
		Falling:             compactMapList(raw["falling"], 4, []string{"artist", "track", "delta", "countA", "countB", "rateA", "rateB"}),
		InsightBullets:      compactBullets(raw["insightBullets"], 4),
	}
}

func readFloat(m map[string]interface{}, key string, fallback float64) float64 {
	if m == nil {
		return fallback
	}
	value, exists := m[key]
	if !exists {
		return fallback
	}
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	default:
		return fallback
	}
}

func readString(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	value, exists := m[key]
	if !exists {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func compactMapList(raw interface{}, limit int, keep []string) []map[string]interface{} {
	items, ok := raw.([]interface{})
	if !ok || len(items) == 0 {
		return []map[string]interface{}{}
	}
	if limit > len(items) {
		limit = len(items)
	}

	out := make([]map[string]interface{}, 0, limit)
	for i := 0; i < limit; i++ {
		item, ok := items[i].(map[string]interface{})
		if !ok {
			continue
		}
		entry := map[string]interface{}{}
		for _, key := range keep {
			if value, exists := item[key]; exists {
				entry[key] = value
			}
		}
		if len(entry) > 0 {
			out = append(out, entry)
		}
	}
	return out
}

func compactBullets(raw interface{}, limit int) []string {
	items, ok := raw.([]interface{})
	if !ok || len(items) == 0 {
		return []string{}
	}
	if limit > len(items) {
		limit = len(items)
	}
	out := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		text, ok := items[i].(string)
		if !ok {
			continue
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		out = append(out, text)
	}
	return out
}

func similarityBandFromConfidence(confidence float64) string {
	switch {
	case confidence >= 0.8:
		return "high"
	case confidence >= 0.55:
		return "medium"
	default:
		return "low"
	}
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func roundFloat(value float64, places int) float64 {
	if places < 0 {
		places = 0
	}
	factor := math.Pow(10, float64(places))
	return math.Round(value*factor) / factor
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
