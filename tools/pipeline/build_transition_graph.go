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

type transitionBuildConfig struct {
	SessionGapMinutes  int  `json:"sessionGapMinutes"`
	MinTransitionCount int  `json:"minTransitionCount"`
	MaxEdgesPerScope   int  `json:"maxEdgesPerScope"`
	IncludeSelfLoops   bool `json:"includeSelfLoops"`
}

type transitionGraphSummary struct {
	TotalEvents                 int `json:"totalEvents"`
	ValidEvents                 int `json:"validEvents"`
	SkippedEvents               int `json:"skippedEvents"`
	TotalSessions               int `json:"totalSessions"`
	TransitionsWithinSessions   int `json:"transitionsWithinSessions"`
	TransitionsAcrossSessionGap int `json:"transitionsAcrossSessionGap"`
}

type transitionNode struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Artist    string `json:"artist,omitempty"`
	Track     string `json:"track,omitempty"`
	PlayCount int    `json:"playCount"`
	InDegree  int    `json:"inDegree"`
	OutDegree int    `json:"outDegree"`
	InWeight  int    `json:"inWeight"`
	OutWeight int    `json:"outWeight"`
}

type transitionEdge struct {
	Source        string  `json:"source"`
	Target        string  `json:"target"`
	SourceLabel   string  `json:"sourceLabel"`
	TargetLabel   string  `json:"targetLabel"`
	Count         int     `json:"count"`
	Probability   float64 `json:"probability"`
	AvgGapMinutes float64 `json:"avgGapMinutes"`
	MinGapMinutes float64 `json:"minGapMinutes"`
	MaxGapMinutes float64 `json:"maxGapMinutes"`
	FirstSeen     int64   `json:"firstSeen"`
	LastSeen      int64   `json:"lastSeen"`
}

type transitionScopeSummary struct {
	TotalDistinctNodes int `json:"totalDistinctNodes"`
	NodesInGraph       int `json:"nodesInGraph"`
	IsolatedNodes      int `json:"isolatedNodes"`
	TotalEdges         int `json:"totalEdges"`
	EdgesRetained      int `json:"edgesRetained"`
	TotalTransitions   int `json:"totalTransitions"`
}

type transitionScopeOutput struct {
	Scope   string                 `json:"scope"`
	Summary transitionScopeSummary `json:"summary"`
	Nodes   []transitionNode       `json:"nodes"`
	Edges   []transitionEdge       `json:"edges"`
}

type transitionGraphArtifact struct {
	GeneratedAt string                           `json:"generatedAt"`
	Config      transitionBuildConfig            `json:"config"`
	Summary     transitionGraphSummary           `json:"summary"`
	Scopes      map[string]transitionScopeOutput `json:"scopes"`
}

type transitionNodeAccumulator struct {
	ID        string
	Label     string
	Artist    string
	Track     string
	PlayCount int
	InDegree  int
	OutDegree int
	InWeight  int
	OutWeight int
}

type transitionEdgeAccumulator struct {
	SourceID    string
	TargetID    string
	SourceLabel string
	TargetLabel string
	Count       int
	TotalGapMs  int64
	MinGapMs    int64
	MaxGapMs    int64
	FirstSeenMs int64
	LastSeenMs  int64
}

type transitionScopeAccumulator struct {
	Nodes            map[string]*transitionNodeAccumulator
	Edges            map[string]*transitionEdgeAccumulator
	TotalTransitions int
}

type normalizedTransitionEvent struct {
	Date int64

	TrackID     string
	TrackLabel  string
	TrackArtist string
	TrackName   string

	ArtistID    string
	ArtistLabel string
}

func runBuildTransitionGraph() {
	fmt.Println("Building transition graph from merged listening history...")

	history, err := loadListeningHistoryOrBuild()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading listening history: %v\n", err)
		os.Exit(1)
	}

	config := defaultTransitionBuildConfig()
	artifact := buildTransitionGraphArtifact(history.Events, config)

	outputPaths := []string{
		DataPath("transition-graph.json"),
		WebDataPath("transition-graph.json"),
	}
	for _, outputPath := range outputPaths {
		if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating output directory for %s: %v\n", outputPath, err)
			os.Exit(1)
		}

		f, err := os.Create(outputPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating output file %s: %v\n", outputPath, err)
			os.Exit(1)
		}
		encoder := json.NewEncoder(f)
		encoder.SetIndent("", "  ")
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(artifact); err != nil {
			f.Close()
			fmt.Fprintf(os.Stderr, "Error writing transition graph JSON to %s: %v\n", outputPath, err)
			os.Exit(1)
		}
		f.Close()
		fmt.Printf("Output written to: %s\n", outputPath)
	}

	fmt.Println()
	fmt.Printf("Valid events: %d / %d\n", artifact.Summary.ValidEvents, artifact.Summary.TotalEvents)
	fmt.Printf("Sessions: %d (gap=%d min)\n", artifact.Summary.TotalSessions, config.SessionGapMinutes)
	fmt.Printf("In-session transitions: %d\n", artifact.Summary.TransitionsWithinSessions)
	fmt.Printf("Across-session boundaries skipped: %d\n", artifact.Summary.TransitionsAcrossSessionGap)
	if scope, ok := artifact.Scopes["track"]; ok {
		fmt.Printf("Track graph: %d nodes, %d edges retained (of %d)\n", scope.Summary.NodesInGraph, scope.Summary.EdgesRetained, scope.Summary.TotalEdges)
	}
	if scope, ok := artifact.Scopes["artist"]; ok {
		fmt.Printf("Artist graph: %d nodes, %d edges retained (of %d)\n", scope.Summary.NodesInGraph, scope.Summary.EdgesRetained, scope.Summary.TotalEdges)
	}
}

func defaultTransitionBuildConfig() transitionBuildConfig {
	sessionGap := int(readEnvInt64("MP3_TRANSITION_SESSION_GAP_MINUTES", 30))
	if sessionGap < 5 {
		sessionGap = 5
	}
	if sessionGap > 180 {
		sessionGap = 180
	}

	minTransitionCount := int(readEnvInt64("MP3_TRANSITION_MIN_EDGE_WEIGHT", 2))
	if minTransitionCount < 1 {
		minTransitionCount = 1
	}
	if minTransitionCount > 1000 {
		minTransitionCount = 1000
	}

	maxEdges := int(readEnvInt64("MP3_TRANSITION_MAX_EDGES", 2500))
	if maxEdges < 1 {
		maxEdges = 1
	}
	if maxEdges > 50000 {
		maxEdges = 50000
	}

	return transitionBuildConfig{
		SessionGapMinutes:  sessionGap,
		MinTransitionCount: minTransitionCount,
		MaxEdgesPerScope:   maxEdges,
		IncludeSelfLoops:   transitionEnvBool("MP3_TRANSITION_INCLUDE_SELF_LOOPS", false),
	}
}

func transitionEnvBool(name string, fallback bool) bool {
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

func buildTransitionGraphArtifact(events []ListeningEvent, config transitionBuildConfig) transitionGraphArtifact {
	normalized := make([]normalizedTransitionEvent, 0, len(events))
	summary := transitionGraphSummary{
		TotalEvents: len(events),
	}

	for _, event := range events {
		trackArtist := strings.TrimSpace(event.Artist)
		trackName := strings.TrimSpace(event.Track)
		if trackArtist == "" || trackName == "" {
			summary.SkippedEvents++
			continue
		}

		artistID := NormalizeForMatching(trackArtist)
		trackID := artistID + "|" + NormalizeForMatching(trackName)
		if artistID == "" || strings.HasSuffix(trackID, "|") {
			summary.SkippedEvents++
			continue
		}

		normalized = append(normalized, normalizedTransitionEvent{
			Date: event.Date,

			TrackID:     trackID,
			TrackLabel:  trackArtist + " - " + trackName,
			TrackArtist: trackArtist,
			TrackName:   trackName,

			ArtistID:    artistID,
			ArtistLabel: trackArtist,
		})
	}

	summary.ValidEvents = len(normalized)
	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i].Date < normalized[j].Date
	})

	trackScope := transitionScopeAccumulator{
		Nodes: map[string]*transitionNodeAccumulator{},
		Edges: map[string]*transitionEdgeAccumulator{},
	}
	artistScope := transitionScopeAccumulator{
		Nodes: map[string]*transitionNodeAccumulator{},
		Edges: map[string]*transitionEdgeAccumulator{},
	}

	sessionGapMs := int64(config.SessionGapMinutes) * 60 * 1000
	var prev *normalizedTransitionEvent
	for i := range normalized {
		cur := &normalized[i]
		addScopeNode(&trackScope, cur.TrackID, cur.TrackLabel, cur.TrackArtist, cur.TrackName)
		addScopeNode(&artistScope, cur.ArtistID, cur.ArtistLabel, cur.ArtistLabel, "")

		if prev == nil {
			summary.TotalSessions++
			prev = cur
			continue
		}

		gapMs := cur.Date - prev.Date
		if gapMs <= sessionGapMs {
			summary.TransitionsWithinSessions++
			addScopeEdge(&trackScope, prev.TrackID, cur.TrackID, prev.TrackLabel, cur.TrackLabel, gapMs, cur.Date, config.IncludeSelfLoops)
			addScopeEdge(&artistScope, prev.ArtistID, cur.ArtistID, prev.ArtistLabel, cur.ArtistLabel, gapMs, cur.Date, config.IncludeSelfLoops)
		} else {
			summary.TransitionsAcrossSessionGap++
			summary.TotalSessions++
		}

		prev = cur
	}

	return transitionGraphArtifact{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Config:      config,
		Summary:     summary,
		Scopes: map[string]transitionScopeOutput{
			"track":  finalizeTransitionScope("track", trackScope, config.MinTransitionCount, config.MaxEdgesPerScope),
			"artist": finalizeTransitionScope("artist", artistScope, config.MinTransitionCount, config.MaxEdgesPerScope),
		},
	}
}

func addScopeNode(scope *transitionScopeAccumulator, id, label, artist, track string) {
	if strings.TrimSpace(id) == "" {
		return
	}
	node := scope.Nodes[id]
	if node == nil {
		node = &transitionNodeAccumulator{
			ID:     id,
			Label:  label,
			Artist: artist,
			Track:  track,
		}
		scope.Nodes[id] = node
	}
	node.PlayCount++
}

func addScopeEdge(scope *transitionScopeAccumulator, sourceID, targetID, sourceLabel, targetLabel string, gapMs, timestampMs int64, includeSelfLoops bool) {
	if sourceID == "" || targetID == "" {
		return
	}
	if !includeSelfLoops && sourceID == targetID {
		return
	}

	scope.TotalTransitions++
	key := sourceID + ">>" + targetID
	edge := scope.Edges[key]
	if edge == nil {
		scope.Edges[key] = &transitionEdgeAccumulator{
			SourceID:    sourceID,
			TargetID:    targetID,
			SourceLabel: sourceLabel,
			TargetLabel: targetLabel,
			Count:       1,
			TotalGapMs:  gapMs,
			MinGapMs:    gapMs,
			MaxGapMs:    gapMs,
			FirstSeenMs: timestampMs,
			LastSeenMs:  timestampMs,
		}
		return
	}

	edge.Count++
	edge.TotalGapMs += gapMs
	if gapMs < edge.MinGapMs {
		edge.MinGapMs = gapMs
	}
	if gapMs > edge.MaxGapMs {
		edge.MaxGapMs = gapMs
	}
	if timestampMs < edge.FirstSeenMs {
		edge.FirstSeenMs = timestampMs
	}
	if timestampMs > edge.LastSeenMs {
		edge.LastSeenMs = timestampMs
	}
}

func finalizeTransitionScope(scopeName string, scope transitionScopeAccumulator, minTransitionCount, maxEdges int) transitionScopeOutput {
	outgoingTotals := make(map[string]int, len(scope.Nodes))
	allEdges := make([]*transitionEdgeAccumulator, 0, len(scope.Edges))
	for _, edge := range scope.Edges {
		outgoingTotals[edge.SourceID] += edge.Count
		allEdges = append(allEdges, edge)
	}

	sort.Slice(allEdges, func(i, j int) bool {
		if allEdges[i].Count == allEdges[j].Count {
			if allEdges[i].SourceLabel == allEdges[j].SourceLabel {
				return allEdges[i].TargetLabel < allEdges[j].TargetLabel
			}
			return allEdges[i].SourceLabel < allEdges[j].SourceLabel
		}
		return allEdges[i].Count > allEdges[j].Count
	})

	retained := make([]*transitionEdgeAccumulator, 0, len(allEdges))
	for _, edge := range allEdges {
		if edge.Count < minTransitionCount {
			continue
		}
		retained = append(retained, edge)
		if len(retained) >= maxEdges {
			break
		}
	}

	nodesInGraph := map[string]struct{}{}
	edgesOut := make([]transitionEdge, 0, len(retained))
	for _, edge := range retained {
		outgoingTotal := outgoingTotals[edge.SourceID]
		probability := 0.0
		if outgoingTotal > 0 {
			probability = float64(edge.Count) / float64(outgoingTotal)
		}

		edgesOut = append(edgesOut, transitionEdge{
			Source:        edge.SourceID,
			Target:        edge.TargetID,
			SourceLabel:   edge.SourceLabel,
			TargetLabel:   edge.TargetLabel,
			Count:         edge.Count,
			Probability:   roundTo(probability, 4),
			AvgGapMinutes: roundTo(float64(edge.TotalGapMs)/float64(edge.Count)/(60*1000), 2),
			MinGapMinutes: roundTo(float64(edge.MinGapMs)/(60*1000), 2),
			MaxGapMinutes: roundTo(float64(edge.MaxGapMs)/(60*1000), 2),
			FirstSeen:     edge.FirstSeenMs,
			LastSeen:      edge.LastSeenMs,
		})

		nodesInGraph[edge.SourceID] = struct{}{}
		nodesInGraph[edge.TargetID] = struct{}{}

		sourceNode := scope.Nodes[edge.SourceID]
		targetNode := scope.Nodes[edge.TargetID]
		if sourceNode != nil {
			sourceNode.OutDegree++
			sourceNode.OutWeight += edge.Count
		}
		if targetNode != nil {
			targetNode.InDegree++
			targetNode.InWeight += edge.Count
		}
	}

	nodesOut := make([]transitionNode, 0, len(nodesInGraph))
	for nodeID := range nodesInGraph {
		node := scope.Nodes[nodeID]
		if node == nil {
			continue
		}
		nodesOut = append(nodesOut, transitionNode{
			ID:        node.ID,
			Label:     node.Label,
			Artist:    node.Artist,
			Track:     node.Track,
			PlayCount: node.PlayCount,
			InDegree:  node.InDegree,
			OutDegree: node.OutDegree,
			InWeight:  node.InWeight,
			OutWeight: node.OutWeight,
		})
	}
	sort.Slice(nodesOut, func(i, j int) bool {
		if nodesOut[i].PlayCount == nodesOut[j].PlayCount {
			return nodesOut[i].Label < nodesOut[j].Label
		}
		return nodesOut[i].PlayCount > nodesOut[j].PlayCount
	})

	return transitionScopeOutput{
		Scope: scopeName,
		Summary: transitionScopeSummary{
			TotalDistinctNodes: len(scope.Nodes),
			NodesInGraph:       len(nodesOut),
			IsolatedNodes:      len(scope.Nodes) - len(nodesOut),
			TotalEdges:         len(scope.Edges),
			EdgesRetained:      len(edgesOut),
			TotalTransitions:   scope.TotalTransitions,
		},
		Nodes: nodesOut,
		Edges: edgesOut,
	}
}

func roundTo(value float64, places int) float64 {
	if places < 0 {
		return value
	}
	multiplier := math.Pow(10, float64(places))
	return math.Round(value*multiplier) / multiplier
}
