package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type transitionGraphNodeAccumulator struct {
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

type transitionGraphEdgeAccumulator struct {
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

type transitionGraphScopeAccumulator struct {
	Nodes            map[string]*transitionGraphNodeAccumulator
	Edges            map[string]*transitionGraphEdgeAccumulator
	TotalTransitions int
}

type transitionGraphEvent struct {
	Date int64

	TrackID     string
	TrackLabel  string
	TrackArtist string
	TrackName   string

	ArtistID    string
	ArtistLabel string
}

func musicTransitionGraph(args map[string]interface{}) (map[string]interface{}, error) {
	sourceFilter, err := parseSourceFilter(args)
	if err != nil {
		return nil, err
	}

	scopeInput := strings.ToLower(strings.TrimSpace(asString(args["scope"])))
	if scopeInput == "" {
		scopeInput = "both"
	}
	includeTrack := false
	includeArtist := false
	switch scopeInput {
	case "track":
		includeTrack = true
	case "artist":
		includeArtist = true
	case "both":
		includeTrack = true
		includeArtist = true
	default:
		return nil, errors.New("scope must be 'track', 'artist', or 'both'")
	}

	sessionGapMinutes := 30
	if v, ok := asInt(args["sessionGapMinutes"]); ok {
		if v < 5 {
			v = 5
		}
		if v > 180 {
			v = 180
		}
		sessionGapMinutes = v
	}

	minTransitionCount := 2
	if v, ok := asInt(args["minTransitionCount"]); ok {
		if v < 1 {
			v = 1
		}
		if v > 1000 {
			v = 1000
		}
		minTransitionCount = v
	}

	maxEdges := 1000
	if v, ok := asInt(args["maxEdges"]); ok {
		if v < 1 {
			v = 1
		}
		if v > 50000 {
			v = 50000
		}
		maxEdges = v
	}

	includeSelfLoops := false
	if v, ok := asBool(args["includeSelfLoops"]); ok {
		includeSelfLoops = v
	}

	var periodStart *time.Time
	var periodEnd *time.Time
	if raw := asString(args["startDate"]); raw != "" {
		parsed, err := parseDate(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid startDate: %w", err)
		}
		periodStart = &parsed
	}
	if raw := asString(args["endDate"]); raw != "" {
		parsed, err := parseDate(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid endDate: %w", err)
		}
		periodEnd = &parsed
	}
	if periodStart != nil && periodEnd != nil && periodEnd.Before(*periodStart) {
		return nil, errors.New("endDate must be on or after startDate")
	}

	scrobbles, err := getListeningScrobbles()
	if err != nil {
		return nil, fmt.Errorf("listening history unavailable: %w", err)
	}
	scrobbles = filterScrobblesBySource(scrobbles, sourceFilter)

	totalScrobblesInScope := 0
	events := make([]transitionGraphEvent, 0, len(scrobbles))
	skippedEvents := 0

	for _, sc := range scrobbles {
		if !timestampInOptionalRange(sc.Date, periodStart, periodEnd) {
			continue
		}
		totalScrobblesInScope++

		artist := strings.TrimSpace(sc.Artist)
		track := strings.TrimSpace(sc.Track)
		if artist == "" || track == "" {
			skippedEvents++
			continue
		}

		artistNorm := normalizeForMatching(artist)
		trackNorm := normalizeForMatching(track)
		if artistNorm == "" || trackNorm == "" {
			skippedEvents++
			continue
		}

		events = append(events, transitionGraphEvent{
			Date: sc.Date,

			TrackID:     artistNorm + "|" + trackNorm,
			TrackLabel:  artist + " - " + track,
			TrackArtist: artist,
			TrackName:   track,

			ArtistID:    artistNorm,
			ArtistLabel: artist,
		})
	}

	sort.Slice(events, func(i, j int) bool {
		return events[i].Date < events[j].Date
	})

	trackScope := transitionGraphScopeAccumulator{
		Nodes: map[string]*transitionGraphNodeAccumulator{},
		Edges: map[string]*transitionGraphEdgeAccumulator{},
	}
	artistScope := transitionGraphScopeAccumulator{
		Nodes: map[string]*transitionGraphNodeAccumulator{},
		Edges: map[string]*transitionGraphEdgeAccumulator{},
	}

	sessionGapMs := int64(sessionGapMinutes) * 60 * 1000
	totalSessions := 0
	inSessionTransitions := 0
	acrossSessionGap := 0
	var prev *transitionGraphEvent

	for i := range events {
		cur := &events[i]
		if includeTrack {
			addTransitionGraphNode(&trackScope, cur.TrackID, cur.TrackLabel, cur.TrackArtist, cur.TrackName)
		}
		if includeArtist {
			addTransitionGraphNode(&artistScope, cur.ArtistID, cur.ArtistLabel, cur.ArtistLabel, "")
		}

		if prev == nil {
			totalSessions++
			prev = cur
			continue
		}

		gapMs := cur.Date - prev.Date
		if gapMs <= sessionGapMs {
			inSessionTransitions++
			if includeTrack {
				addTransitionGraphEdge(&trackScope, prev.TrackID, cur.TrackID, prev.TrackLabel, cur.TrackLabel, gapMs, cur.Date, includeSelfLoops)
			}
			if includeArtist {
				addTransitionGraphEdge(&artistScope, prev.ArtistID, cur.ArtistID, prev.ArtistLabel, cur.ArtistLabel, gapMs, cur.Date, includeSelfLoops)
			}
		} else {
			acrossSessionGap++
			totalSessions++
		}

		prev = cur
	}

	graphs := map[string]interface{}{}
	if includeTrack {
		graphs["track"] = finalizeTransitionGraphScope(trackScope, minTransitionCount, maxEdges)
	}
	if includeArtist {
		graphs["artist"] = finalizeTransitionGraphScope(artistScope, minTransitionCount, maxEdges)
	}

	periodScope := map[string]interface{}{
		"source": sourceFilter,
	}
	if periodStart != nil {
		periodScope["startDate"] = periodStart.Format("2006-01-02")
	}
	if periodEnd != nil {
		periodScope["endDate"] = periodEnd.Format("2006-01-02")
	}
	if len(events) > 0 {
		periodScope["effectiveStartDate"] = time.UnixMilli(events[0].Date).UTC().Format("2006-01-02")
		periodScope["effectiveEndDate"] = time.UnixMilli(events[len(events)-1].Date).UTC().Format("2006-01-02")
	}

	return map[string]interface{}{
		"scope":  periodScope,
		"graphs": graphs,
		"config": map[string]interface{}{
			"scope":              scopeInput,
			"sessionGapMinutes":  sessionGapMinutes,
			"minTransitionCount": minTransitionCount,
			"maxEdges":           maxEdges,
			"includeSelfLoops":   includeSelfLoops,
		},
		"summary": map[string]interface{}{
			"totalScrobblesInScope":       totalScrobblesInScope,
			"validEvents":                 len(events),
			"skippedEvents":               skippedEvents,
			"totalSessions":               totalSessions,
			"transitionsWithinSessions":   inSessionTransitions,
			"transitionsAcrossSessionGap": acrossSessionGap,
		},
	}, nil
}

func addTransitionGraphNode(scope *transitionGraphScopeAccumulator, id, label, artist, track string) {
	if id == "" {
		return
	}
	node := scope.Nodes[id]
	if node == nil {
		node = &transitionGraphNodeAccumulator{
			ID:     id,
			Label:  label,
			Artist: artist,
			Track:  track,
		}
		scope.Nodes[id] = node
	}
	node.PlayCount++
}

func addTransitionGraphEdge(scope *transitionGraphScopeAccumulator, sourceID, targetID, sourceLabel, targetLabel string, gapMs, timestampMs int64, includeSelfLoops bool) {
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
		scope.Edges[key] = &transitionGraphEdgeAccumulator{
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

func finalizeTransitionGraphScope(scope transitionGraphScopeAccumulator, minTransitionCount, maxEdges int) map[string]interface{} {
	allEdges := make([]*transitionGraphEdgeAccumulator, 0, len(scope.Edges))
	outgoingTotals := map[string]int{}
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

	retained := make([]*transitionGraphEdgeAccumulator, 0, len(allEdges))
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
	edgesOut := make([]map[string]interface{}, 0, len(retained))
	for _, edge := range retained {
		probability := 0.0
		if outgoingTotals[edge.SourceID] > 0 {
			probability = float64(edge.Count) / float64(outgoingTotals[edge.SourceID])
		}
		edgesOut = append(edgesOut, map[string]interface{}{
			"source":        edge.SourceID,
			"target":        edge.TargetID,
			"sourceLabel":   edge.SourceLabel,
			"targetLabel":   edge.TargetLabel,
			"count":         edge.Count,
			"probability":   roundFloat(probability, 4),
			"avgGapMinutes": roundFloat(float64(edge.TotalGapMs)/float64(edge.Count)/(60*1000), 2),
			"minGapMinutes": roundFloat(float64(edge.MinGapMs)/(60*1000), 2),
			"maxGapMinutes": roundFloat(float64(edge.MaxGapMs)/(60*1000), 2),
			"firstSeen":     edge.FirstSeenMs,
			"lastSeen":      edge.LastSeenMs,
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

	nodesOut := make([]map[string]interface{}, 0, len(nodesInGraph))
	for nodeID := range nodesInGraph {
		node := scope.Nodes[nodeID]
		if node == nil {
			continue
		}
		nodesOut = append(nodesOut, map[string]interface{}{
			"id":        node.ID,
			"label":     node.Label,
			"artist":    node.Artist,
			"track":     node.Track,
			"playCount": node.PlayCount,
			"inDegree":  node.InDegree,
			"outDegree": node.OutDegree,
			"inWeight":  node.InWeight,
			"outWeight": node.OutWeight,
		})
	}

	sort.Slice(nodesOut, func(i, j int) bool {
		ip, _ := asInt(nodesOut[i]["playCount"])
		jp, _ := asInt(nodesOut[j]["playCount"])
		if ip == jp {
			return fmt.Sprint(nodesOut[i]["label"]) < fmt.Sprint(nodesOut[j]["label"])
		}
		return ip > jp
	})

	return map[string]interface{}{
		"summary": map[string]interface{}{
			"totalDistinctNodes": len(scope.Nodes),
			"nodesInGraph":       len(nodesOut),
			"isolatedNodes":      len(scope.Nodes) - len(nodesOut),
			"totalEdges":         len(scope.Edges),
			"edgesRetained":      len(edgesOut),
			"totalTransitions":   scope.TotalTransitions,
		},
		"nodes": nodesOut,
		"edges": edgesOut,
	}
}
