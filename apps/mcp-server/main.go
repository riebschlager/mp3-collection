package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	serverName      = "music-intel-mcp"
	serverVersion   = "0.1.0"
	protocolVersion = "2024-11-05"
)

const (
	methodInitialize = "initialize"
	methodToolsList  = "tools/list"
	methodToolsCall  = "tools/call"
)

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type initializeParams struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	Capabilities    map[string]interface{} `json:"capabilities,omitempty"`
	ClientInfo      map[string]interface{} `json:"clientInfo,omitempty"`
}

type toolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

type toolCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

type toolCallResult struct {
	Content           []contentItem          `json:"content"`
	StructuredContent map[string]interface{} `json:"structuredContent,omitempty"`
	IsError           bool                   `json:"isError,omitempty"`
}

type contentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type resolverTrack struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Artist   string `json:"artist"`
	Album    string `json:"album"`
	Genre    string `json:"genre"`
	Duration int    `json:"duration"`
	Year     *int   `json:"year"`

	NormArtist string
	NormTrack  string
	NormAlbum  string
}

type resolverChunk struct {
	Tracks []resolverTrack `json:"tracks"`
}

type trackResolver struct {
	tracks      []resolverTrack
	exactIndex  map[string][]int
	artistIndex map[string][]int
	trackIndex  map[string][]int
	albumIndex  map[string][]int
	aliases     *aliasCatalog
	aliasPath   string
}

type scoredCandidate struct {
	index      int
	confidence float64
	method     string
	evidence   []map[string]interface{}
}

type aliasEntry struct {
	EntityType     string  `json:"entityType"`
	CanonicalValue string  `json:"canonicalValue"`
	AliasValue     string  `json:"aliasValue"`
	Confidence     float64 `json:"confidence"`
}

type aliasFile struct {
	Aliases []aliasEntry `json:"aliases"`
}

type aliasCatalog struct {
	artist map[string]string
	track  map[string]string
	album  map[string]string
}

type aliasStep struct {
	EntityType string
	Alias      string
	Canonical  string
}

type lastFMScrobble struct {
	Track           string `json:"track"`
	Artist          string `json:"artist"`
	Album           string `json:"album"`
	Date            int64  `json:"date"`
	Source          string `json:"source,omitempty"`
	SpotifyTrackURI string `json:"spotifyTrackUri,omitempty"`
	MsPlayed        int64  `json:"msPlayed,omitempty"`
}

type lastFMData struct {
	Username  string           `json:"username"`
	Scrobbles []lastFMScrobble `json:"scrobbles"`
}

type listeningHistoryData struct {
	Events []lastFMScrobble `json:"events"`
}

type matchResult struct {
	Status         string
	Method         string
	Confidence     float64
	CandidateIndex int
	FailurePattern string
	AliasSteps     []aliasStep
}

type periodCounters struct {
	Total     int
	Matched   int
	Unmatched int
}

type clusterInfo struct {
	Count    int
	Examples []string
}

type topTrackStat struct {
	Key    string
	Artist string
	Track  string
	Count  int
}

type eraAnalysis struct {
	Label          string
	Start          time.Time
	End            time.Time
	TotalScrobbles int
	Days           float64
	TrackCounts    map[string]int
	ArtistCounts   map[string]int
	GenreCounts    map[string]int
	TrackDisplay   map[string]*topTrackStat
}

var (
	resolverOnce     sync.Once
	resolverInstance *trackResolver
	resolverErr      error
	resolverStateMu  sync.Mutex
	listeningOnce    sync.Once
	listeningCache   []lastFMScrobble
	listeningErr     error
	debugLogOnce     sync.Once
	debugLogPath     string
)

func resolveDebugLogPath() string {
	debugLogOnce.Do(func() {
		if envPath := strings.TrimSpace(os.Getenv("MP3_MCP_LOG_PATH")); envPath != "" {
			debugLogPath = filepath.Clean(envPath)
			return
		}
		if root, err := detectProjectRoot(); err == nil {
			debugLogPath = filepath.Join(root, "apps", "mcp-server", "mcp-server.log")
			return
		}
		debugLogPath = filepath.Join(os.TempDir(), "mp3-mcp-server.log")
	})
	return debugLogPath
}

func debugMCP(format string, args ...interface{}) {
	path := resolveDebugLogPath()
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()

	line := fmt.Sprintf(format, args...)
	ts := time.Now().UTC().Format(time.RFC3339)
	_, _ = fmt.Fprintf(f, "%s %s\n", ts, line)
}

func main() {
	debugMCP("START pid=%d argv=%q", os.Getpid(), os.Args)

	reader := bufio.NewReader(os.Stdin)
	writer := bufio.NewWriter(os.Stdout)
	defer writer.Flush()

	for {
		payload, err := readMessage(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				debugMCP("EOF pid=%d", os.Getpid())
				return
			}
			debugMCP("readMessage error: %v", err)
			fmt.Fprintf(os.Stderr, "read error: %v\n", err)
			continue
		}

		var req rpcRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			if err := writeResponse(writer, rpcResponse{
				JSONRPC: "2.0",
				Error: &rpcError{
					Code:    -32700,
					Message: "Parse error",
				},
			}); err != nil {
				fmt.Fprintf(os.Stderr, "write parse error response failed: %v\n", err)
			}
			continue
		}

		resp, shouldRespond := handleRequest(req)
		if !shouldRespond {
			continue
		}
		if err := writeResponse(writer, resp); err != nil {
			fmt.Fprintf(os.Stderr, "write response error: %v\n", err)
			return
		}
	}
}

func handleRequest(req rpcRequest) (rpcResponse, bool) {
	debugMCP("request method=%q id=%s", req.Method, rawMessageForLog(req.ID))
	switch req.Method {
	case methodInitialize:
		respID := req.ID
		if len(respID) == 0 {
			respID = json.RawMessage("null")
		}
		clientProtocol := resolveInitializeProtocolVersion(req.Params)
		debugMCP("initialize params raw=%s", rawMessageForLog(req.Params))
		debugMCP("initialize request id=%s protocol=%q", rawMessageForLog(req.ID), clientProtocol)
		resp := rpcResponse{
			JSONRPC: "2.0",
			ID:      respID,
			Result: map[string]interface{}{
				"protocolVersion": clientProtocol,
				"serverInfo": map[string]interface{}{
					"name":    serverName,
					"version": serverVersion,
				},
				"capabilities": map[string]interface{}{
					"tools": map[string]interface{}{
						"listChanged": false,
					},
				},
			},
		}
		debugMCP("initialize response id=%s protocol=%q", rawMessageForLog(respID), clientProtocol)
		return resp, true
	case "notifications/initialized":
		return rpcResponse{}, false
	case methodToolsList:
		respID := req.ID
		if len(respID) == 0 {
			respID = json.RawMessage("null")
		}
		return rpcResponse{
			JSONRPC: "2.0",
			ID:      respID,
			Result: map[string]interface{}{
				"tools": toolCatalog(),
			},
		}, true
	case methodToolsCall:
		respID := req.ID
		if len(respID) == 0 {
			respID = json.RawMessage("null")
		}
		result, err := handleToolsCall(req.Params)
		if err != nil {
			return rpcResponse{
				JSONRPC: "2.0",
				ID:      respID,
				Error: &rpcError{
					Code:    -32602,
					Message: err.Error(),
				},
			}, true
		}
		return rpcResponse{
			JSONRPC: "2.0",
			ID:      respID,
			Result:  result,
		}, true
	default:
		debugMCP("request unknown method=%q id=%s", req.Method, rawMessageForLog(req.ID))
		if len(req.ID) == 0 {
			return rpcResponse{}, false
		}
		return rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &rpcError{
				Code:    -32601,
				Message: "Method not found",
			},
		}, true
	}
}

func handleToolsCall(raw json.RawMessage) (toolCallResult, error) {
	var params toolCallParams
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &params); err != nil {
			return toolCallResult{}, fmt.Errorf("invalid tools/call params: %w", err)
		}
	}

	if params.Name == "" {
		return toolCallResult{}, errors.New("missing tool name")
	}

	if params.Arguments == nil {
		params.Arguments = map[string]interface{}{}
	}

	var (
		payload map[string]interface{}
		err     error
	)

	switch params.Name {
	case "music_resolve_track_identity":
		payload, err = resolveTrackIdentity(params.Arguments)
	case "music_audit_match_coverage":
		payload, err = auditMatchCoverage(params.Arguments)
	case "music_find_dormant_returns":
		payload, err = findDormantReturns(params.Arguments)
	case "music_compare_eras":
		payload, err = compareEras(params.Arguments)
	case "music_listening_summary":
		payload, err = musicListeningSummary(params.Arguments)
	case "music_new_discoveries":
		payload, err = musicNewDiscoveries(params.Arguments)
	case "music_genre_profile":
		payload, err = musicGenreProfile(params.Arguments)
	case "music_listening_patterns":
		payload, err = musicListeningPatterns(params.Arguments)
	case "music_streaks_and_bursts":
		payload, err = musicStreaksAndBursts(params.Arguments)
	case "music_year_story":
		payload, err = musicYearStory(params.Arguments)
	case "music_reload_alias_map":
		payload, err = reloadAliasMap(params.Arguments)
	default:
		return toolCallResult{
			IsError: true,
			Content: []contentItem{{
				Type: "text",
				Text: fmt.Sprintf("unknown tool: %s", params.Name),
			}},
		}, nil
	}
	if err != nil {
		return toolCallResult{
			IsError: true,
			Content: []contentItem{{
				Type: "text",
				Text: err.Error(),
			}},
		}, nil
	}

	formatted, _ := json.MarshalIndent(payload, "", "  ")
	return toolCallResult{
		Content: []contentItem{{
			Type: "text",
			Text: string(formatted),
		}},
		StructuredContent: payload,
	}, nil
}

func resolveTrackIdentity(args map[string]interface{}) (map[string]interface{}, error) {
	query, ok := asMap(args["query"])
	if !ok {
		return nil, errors.New("query object is required")
	}

	artist := asString(query["artist"])
	track := asString(query["track"])
	if artist == "" || track == "" {
		return nil, errors.New("query.artist and query.track are required")
	}
	album := asString(query["album"])

	strictness := "medium"
	maxCandidates := 10
	includeEvidence := true
	if options, ok := asMap(args["options"]); ok {
		if v := asString(options["strictness"]); v != "" {
			strictness = v
		}
		if v, ok := asInt(options["maxCandidates"]); ok {
			if v < 1 {
				v = 1
			}
			if v > 50 {
				v = 50
			}
			maxCandidates = v
		}
		if v, ok := asBool(options["includeEvidence"]); ok {
			includeEvidence = v
		}
	}
	if strictness != "high" && strictness != "medium" && strictness != "low" {
		return nil, errors.New("options.strictness must be 'high', 'medium', or 'low'")
	}

	var (
		queryDuration int
		hasDuration   bool
	)
	if v, ok := asInt(query["durationSec"]); ok && v >= 0 {
		queryDuration = v
		hasDuration = true
	}

	resolver, err := getResolver()
	if err != nil {
		return nil, fmt.Errorf("resolver unavailable: %w", err)
	}

	normArtistRaw := normalizeForMatching(artist)
	normTrackRaw := normalizeForMatching(track)
	normAlbumRaw := normalizeForMatching(album)
	normArtist, artistAliasSteps := resolver.aliases.Canonicalize("artist", normArtistRaw)
	normTrack, trackAliasSteps := resolver.aliases.Canonicalize("track", normTrackRaw)
	normAlbum, albumAliasSteps := resolver.aliases.Canonicalize("album", normAlbumRaw)
	if normArtist == "" || normTrack == "" {
		return nil, errors.New("query.artist and query.track produced empty normalized values")
	}
	aliasEvidence := makeAliasEvidence(append(append(artistAliasSteps, trackAliasSteps...), albumAliasSteps...))

	candidateIndexes := resolver.collectCandidateIndexes(normArtist, normTrack, normAlbum)
	if len(candidateIndexes) == 0 {
		evidence := []map[string]interface{}{}
		if includeEvidence {
			evidence = append(evidence, aliasEvidence...)
			evidence = append(evidence, map[string]interface{}{
				"signal": "candidate_search",
				"score":  0.0,
				"detail": "no candidates found in library indexes",
			})
		}
		return map[string]interface{}{
			"status":           "unmatched",
			"canonicalTrackId": nil,
			"confidence":       0.0,
			"method":           "fuzzy",
			"evidence":         evidence,
			"candidates":       []map[string]interface{}{},
		}, nil
	}

	scored := make([]scoredCandidate, 0, len(candidateIndexes))
	for _, idx := range candidateIndexes {
		candidate := resolver.tracks[idx]
		scored = append(scored, scoreCandidate(
			idx,
			normArtist,
			normTrack,
			normAlbum,
			hasDuration,
			queryDuration,
			candidate,
		))
	}

	sort.Slice(scored, func(i, j int) bool {
		if scored[i].confidence == scored[j].confidence {
			a := resolver.tracks[scored[i].index]
			b := resolver.tracks[scored[j].index]
			if a.Artist == b.Artist {
				return strings.ToLower(a.Name) < strings.ToLower(b.Name)
			}
			return strings.ToLower(a.Artist) < strings.ToLower(b.Artist)
		}
		return scored[i].confidence > scored[j].confidence
	})

	top := scored[0]
	nextConfidence := 0.0
	if len(scored) > 1 {
		nextConfidence = scored[1].confidence
	}
	matchedThreshold, ambiguousThreshold := thresholdsForStrictness(strictness)
	confidenceGap := top.confidence - nextConfidence

	status := "unmatched"
	var canonicalTrackID interface{} = nil
	if top.confidence >= matchedThreshold && confidenceGap >= 0.05 {
		status = "matched"
		canonicalTrackID = resolver.tracks[top.index].ID
	} else if top.confidence >= ambiguousThreshold {
		status = "ambiguous"
	}

	limit := maxCandidates
	if limit > len(scored) {
		limit = len(scored)
	}
	candidates := make([]map[string]interface{}, 0, limit)
	for i := 0; i < limit; i++ {
		c := scored[i]
		t := resolver.tracks[c.index]
		candidates = append(candidates, map[string]interface{}{
			"trackId":    t.ID,
			"artist":     t.Artist,
			"track":      t.Name,
			"album":      t.Album,
			"confidence": roundFloat(c.confidence, 4),
		})
	}

	method := top.method
	if len(aliasEvidence) > 0 && top.method == "exact_norm" {
		method = "manual_alias"
	}

	evidence := []map[string]interface{}{}
	if includeEvidence {
		evidence = append(evidence, aliasEvidence...)
		evidence = append(evidence, top.evidence...)
	}

	return map[string]interface{}{
		"status":           status,
		"canonicalTrackId": canonicalTrackID,
		"confidence":       roundFloat(top.confidence, 4),
		"method":           method,
		"evidence":         evidence,
		"candidates":       candidates,
	}, nil
}

func getResolver() (*trackResolver, error) {
	resolverStateMu.Lock()
	defer resolverStateMu.Unlock()

	resolverOnce.Do(func() {
		resolverInstance, resolverErr = loadResolver()
	})
	return resolverInstance, resolverErr
}

func forceReloadResolver() (*trackResolver, error) {
	resolverStateMu.Lock()
	defer resolverStateMu.Unlock()

	resolverOnce = sync.Once{}
	resolverInstance = nil
	resolverErr = nil

	resolverOnce.Do(func() {
		resolverInstance, resolverErr = loadResolver()
	})
	return resolverInstance, resolverErr
}

func loadResolver() (*trackResolver, error) {
	root, err := detectProjectRoot()
	if err != nil {
		return nil, err
	}
	aliases, aliasPath, err := loadAliasCatalog(root)
	if err != nil {
		return nil, err
	}

	pattern := trackChunkPattern(root)
	chunkFiles, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to discover chunk files: %w", err)
	}
	if len(chunkFiles) == 0 {
		return nil, fmt.Errorf("no chunk files found at %s", pattern)
	}
	sort.Strings(chunkFiles)

	resolver := &trackResolver{
		exactIndex:  map[string][]int{},
		artistIndex: map[string][]int{},
		trackIndex:  map[string][]int{},
		albumIndex:  map[string][]int{},
		aliases:     aliases,
		aliasPath:   aliasPath,
	}

	for _, path := range chunkFiles {
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read chunk %s: %w", path, err)
		}

		var chunk resolverChunk
		if err := json.Unmarshal(raw, &chunk); err != nil {
			return nil, fmt.Errorf("parse chunk %s: %w", path, err)
		}

		for _, tr := range chunk.Tracks {
			if tr.ID == "" || strings.TrimSpace(tr.Name) == "" {
				continue
			}
			tr.NormArtist = aliases.CanonicalValue("artist", normalizeForMatching(tr.Artist))
			tr.NormTrack = aliases.CanonicalValue("track", normalizeForMatching(tr.Name))
			tr.NormAlbum = aliases.CanonicalValue("album", normalizeForMatching(tr.Album))
			if tr.NormArtist == "" || tr.NormTrack == "" {
				continue
			}

			idx := len(resolver.tracks)
			resolver.tracks = append(resolver.tracks, tr)

			exactKey := buildExactKey(tr.NormArtist, tr.NormTrack)
			resolver.exactIndex[exactKey] = append(resolver.exactIndex[exactKey], idx)
			resolver.artistIndex[tr.NormArtist] = append(resolver.artistIndex[tr.NormArtist], idx)
			resolver.trackIndex[tr.NormTrack] = append(resolver.trackIndex[tr.NormTrack], idx)
			if tr.NormAlbum != "" {
				resolver.albumIndex[tr.NormAlbum] = append(resolver.albumIndex[tr.NormAlbum], idx)
			}
		}
	}

	if len(resolver.tracks) == 0 {
		return nil, errors.New("resolver loaded zero tracks")
	}
	return resolver, nil
}

func detectProjectRoot() (string, error) {
	if envRoot := strings.TrimSpace(os.Getenv("MP3_COLLECTION_ROOT")); envRoot != "" {
		if hasTrackChunks(envRoot) {
			return envRoot, nil
		}
		return "", fmt.Errorf("MP3_COLLECTION_ROOT=%s does not contain track chunks at %s", envRoot, trackChunkPattern(envRoot))
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	if hasTrackChunks(cwd) {
		return inferRootFromWebDataDir(cwd), nil
	}

	checked := map[string]bool{}
	candidates := []string{cwd}
	for i := 0; i < 4; i++ {
		parent := filepath.Dir(candidates[len(candidates)-1])
		if parent == candidates[len(candidates)-1] {
			break
		}
		candidates = append(candidates, parent)
	}

	for _, candidate := range candidates {
		if checked[candidate] {
			continue
		}
		checked[candidate] = true
		if hasTrackChunks(candidate) {
			return candidate, nil
		}
	}

	return "", errors.New("could not locate project root containing track chunk data")
}

func hasTrackChunks(root string) bool {
	matches, err := filepath.Glob(trackChunkPattern(root))
	if err != nil {
		return false
	}
	return len(matches) > 0
}

func trackChunkPattern(root string) string {
	return filepath.Join(resolveWebDataDir(root), "chunks", "tracks-*.json")
}

func resolveWebDataDir(root string) string {
	if envDir := strings.TrimSpace(os.Getenv("MP3_WEB_DATA_DIR")); envDir != "" {
		return resolvePath(root, envDir)
	}
	return filepath.Join(root, "data", "derived", "web")
}

func inferRootFromWebDataDir(defaultRoot string) string {
	webDataDir := resolveWebDataDir(defaultRoot)
	if filepath.Base(webDataDir) == "web-data" {
		return filepath.Dir(webDataDir)
	}
	return defaultRoot
}

func loadAliasCatalog(root string) (*aliasCatalog, string, error) {
	catalog := newAliasCatalog()
	path, required, err := resolveAliasMapPath(root)
	if err != nil {
		return nil, "", err
	}
	if path == "" {
		return catalog, "", nil
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("read alias map %s: %w", path, err)
	}
	entries, err := parseAliasEntries(raw)
	if err != nil {
		return nil, "", fmt.Errorf("parse alias map %s: %w", path, err)
	}
	if len(entries) == 0 {
		if required {
			return nil, "", fmt.Errorf("alias map %s is empty", path)
		}
		return catalog, path, nil
	}

	for _, entry := range entries {
		entityType := normalizeEntityType(entry.EntityType)
		if entityType == "" {
			continue
		}
		aliasNorm := normalizeForMatching(entry.AliasValue)
		canonNorm := normalizeForMatching(entry.CanonicalValue)
		if aliasNorm == "" || canonNorm == "" || aliasNorm == canonNorm {
			continue
		}
		catalog.put(entityType, aliasNorm, canonNorm)
	}
	return catalog, path, nil
}

func resolveAliasMapPath(root string) (path string, required bool, err error) {
	if envPath := strings.TrimSpace(os.Getenv("MP3_ALIAS_MAP_PATH")); envPath != "" {
		required = true
		resolved := resolvePath(root, envPath)
		if _, statErr := os.Stat(resolved); statErr != nil {
			return "", required, fmt.Errorf("MP3_ALIAS_MAP_PATH points to unreadable file %s: %w", resolved, statErr)
		}
		return resolved, required, nil
	}

	candidates := []string{
		filepath.Join(root, "data", "alias_map.json"),
		filepath.Join(root, "apps", "mcp-server", "data", "alias_map.json"),
	}
	for _, candidate := range candidates {
		if _, statErr := os.Stat(candidate); statErr == nil {
			return candidate, false, nil
		}
	}
	return "", false, nil
}

func resolvePath(root, value string) string {
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	return filepath.Clean(filepath.Join(root, value))
}

func parseAliasEntries(raw []byte) ([]aliasEntry, error) {
	var direct []aliasEntry
	if err := json.Unmarshal(raw, &direct); err == nil {
		return direct, nil
	}

	var wrapped aliasFile
	if err := json.Unmarshal(raw, &wrapped); err == nil {
		if wrapped.Aliases != nil {
			return wrapped.Aliases, nil
		}
	}

	var grouped struct {
		Artists map[string]string `json:"artists"`
		Tracks  map[string]string `json:"tracks"`
		Albums  map[string]string `json:"albums"`
	}
	if err := json.Unmarshal(raw, &grouped); err == nil {
		out := make([]aliasEntry, 0, len(grouped.Artists)+len(grouped.Tracks)+len(grouped.Albums))
		for alias, canonical := range grouped.Artists {
			out = append(out, aliasEntry{
				EntityType:     "artist",
				AliasValue:     alias,
				CanonicalValue: canonical,
			})
		}
		for alias, canonical := range grouped.Tracks {
			out = append(out, aliasEntry{
				EntityType:     "track",
				AliasValue:     alias,
				CanonicalValue: canonical,
			})
		}
		for alias, canonical := range grouped.Albums {
			out = append(out, aliasEntry{
				EntityType:     "album",
				AliasValue:     alias,
				CanonicalValue: canonical,
			})
		}
		return out, nil
	}

	return nil, errors.New("expected alias map format: []aliasEntry, {\"aliases\": [...]}, or {\"artists\"|\"tracks\"|\"albums\": {...}}")
}

func normalizeEntityType(entity string) string {
	switch strings.ToLower(strings.TrimSpace(entity)) {
	case "artist", "artists":
		return "artist"
	case "track", "tracks":
		return "track"
	case "album", "albums":
		return "album"
	default:
		return ""
	}
}

func newAliasCatalog() *aliasCatalog {
	return &aliasCatalog{
		artist: map[string]string{},
		track:  map[string]string{},
		album:  map[string]string{},
	}
}

func (a *aliasCatalog) put(entityType, alias, canonical string) {
	if a == nil {
		return
	}
	target := a.bucket(entityType)
	if target == nil {
		return
	}
	target[alias] = canonical
}

func (a *aliasCatalog) bucket(entityType string) map[string]string {
	if a == nil {
		return nil
	}
	switch entityType {
	case "artist":
		return a.artist
	case "track":
		return a.track
	case "album":
		return a.album
	default:
		return nil
	}
}

func (a *aliasCatalog) CanonicalValue(entityType, normalized string) string {
	value, _ := a.Canonicalize(entityType, normalized)
	return value
}

func (a *aliasCatalog) Canonicalize(entityType, normalized string) (string, []aliasStep) {
	if a == nil || normalized == "" {
		return normalized, nil
	}
	table := a.bucket(entityType)
	if table == nil {
		return normalized, nil
	}

	steps := []aliasStep{}
	seen := map[string]struct{}{normalized: struct{}{}}
	current := normalized
	for depth := 0; depth < 8; depth++ {
		next, ok := table[current]
		if !ok || next == "" || next == current {
			break
		}
		steps = append(steps, aliasStep{
			EntityType: entityType,
			Alias:      current,
			Canonical:  next,
		})
		if _, loop := seen[next]; loop {
			current = next
			break
		}
		seen[next] = struct{}{}
		current = next
	}
	return current, steps
}

func makeAliasEvidence(steps []aliasStep) []map[string]interface{} {
	if len(steps) == 0 {
		return nil
	}
	evidence := make([]map[string]interface{}, 0, len(steps))
	for _, step := range steps {
		evidence = append(evidence, map[string]interface{}{
			"signal": "alias_override",
			"score":  1.0,
			"detail": fmt.Sprintf("%s alias %q -> %q", step.EntityType, step.Alias, step.Canonical),
		})
	}
	return evidence
}

func (a *aliasCatalog) Count() int {
	if a == nil {
		return 0
	}
	return len(a.artist) + len(a.track) + len(a.album)
}

func (a *aliasCatalog) CountByEntity() map[string]int {
	if a == nil {
		return map[string]int{
			"artist": 0,
			"track":  0,
			"album":  0,
		}
	}
	return map[string]int{
		"artist": len(a.artist),
		"track":  len(a.track),
		"album":  len(a.album),
	}
}

func buildExactKey(normArtist, normTrack string) string {
	return normArtist + "|" + normTrack
}

func (r *trackResolver) collectCandidateIndexes(normArtist, normTrack, normAlbum string) []int {
	seen := map[int]struct{}{}
	add := func(indexes []int) {
		for _, idx := range indexes {
			seen[idx] = struct{}{}
		}
	}

	if normArtist != "" && normTrack != "" {
		add(r.exactIndex[buildExactKey(normArtist, normTrack)])
	}
	if normArtist != "" {
		add(r.artistIndex[normArtist])
	}
	if normTrack != "" {
		add(r.trackIndex[normTrack])
	}
	if normAlbum != "" {
		add(r.albumIndex[normAlbum])
	}

	if len(seen) == 0 {
		for i := range r.tracks {
			seen[i] = struct{}{}
		}
	}

	out := make([]int, 0, len(seen))
	for idx := range seen {
		out = append(out, idx)
	}
	return out
}

func scoreCandidate(
	candidateIndex int,
	queryArtist string,
	queryTrack string,
	queryAlbum string,
	hasDuration bool,
	queryDuration int,
	candidate resolverTrack,
) scoredCandidate {
	trackSimilarity := stringSimilarity(queryTrack, candidate.NormTrack)
	artistSimilarity := stringSimilarity(queryArtist, candidate.NormArtist)
	albumSimilarity := 0.0
	if queryAlbum != "" && candidate.NormAlbum != "" {
		albumSimilarity = stringSimilarity(queryAlbum, candidate.NormAlbum)
	}

	exactPairScore := 0.0
	if queryArtist == candidate.NormArtist && queryTrack == candidate.NormTrack {
		exactPairScore = 0.55
	}

	titleScore := 0.20 * trackSimilarity

	artistScore := 0.0
	if queryArtist == candidate.NormArtist {
		artistScore = 0.15
	} else if artistSimilarity >= 0.8 {
		artistScore = 0.15 * artistSimilarity
	}

	durationSimilarity := 0.0
	if hasDuration && candidate.Duration > 0 {
		diff := math.Abs(float64(queryDuration - candidate.Duration))
		if diff <= 30 {
			durationSimilarity = 1 - (diff / 30.0)
		}
	}
	durationScore := 0.05 * durationSimilarity

	albumScore := 0.05 * albumSimilarity

	confidence := clamp01(exactPairScore + titleScore + artistScore + durationScore + albumScore)

	method := "fuzzy"
	if exactPairScore > 0 {
		method = "exact_norm"
	} else if queryAlbum != "" && albumSimilarity >= 0.8 && trackSimilarity >= 0.7 {
		method = "album_backoff"
	}

	evidence := []map[string]interface{}{
		{
			"signal": "exact_norm_pair",
			"score":  roundFloat(exactPairScore, 4),
			"detail": "0.55 weight when normalized artist+track both exactly match",
		},
		{
			"signal": "fuzzy_title_similarity",
			"score":  roundFloat(titleScore, 4),
			"detail": fmt.Sprintf("track similarity=%.4f", trackSimilarity),
		},
		{
			"signal": "artist_similarity",
			"score":  roundFloat(artistScore, 4),
			"detail": fmt.Sprintf("artist similarity=%.4f", artistSimilarity),
		},
		{
			"signal": "duration_proximity",
			"score":  roundFloat(durationScore, 4),
			"detail": fmt.Sprintf("duration similarity=%.4f", durationSimilarity),
		},
		{
			"signal": "album_similarity",
			"score":  roundFloat(albumScore, 4),
			"detail": fmt.Sprintf("album similarity=%.4f", albumSimilarity),
		},
	}

	return scoredCandidate{
		index:      candidateIndex,
		confidence: confidence,
		method:     method,
		evidence:   evidence,
	}
}

func thresholdsForStrictness(strictness string) (matched float64, ambiguous float64) {
	switch strictness {
	case "high":
		return 0.90, 0.75
	case "low":
		return 0.75, 0.60
	default:
		return 0.85, 0.70
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func roundFloat(v float64, places int) float64 {
	p := math.Pow(10, float64(places))
	return math.Round(v*p) / p
}

func stringSimilarity(a, b string) float64 {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return 0
	}
	if a == b {
		return 1
	}

	// Strong partial containment, useful for small naming variants.
	if strings.Contains(a, b) || strings.Contains(b, a) {
		shorter, longer := len(a), len(b)
		if shorter > longer {
			shorter, longer = longer, shorter
		}
		ratio := float64(shorter) / float64(longer)
		return clamp01(0.75 + (0.25 * ratio))
	}

	lev := levenshteinDistance(a, b)
	maxLen := len(a)
	if len(b) > maxLen {
		maxLen = len(b)
	}
	if maxLen == 0 {
		return 0
	}
	levSimilarity := 1.0 - (float64(lev) / float64(maxLen))
	if levSimilarity < 0 {
		levSimilarity = 0
	}

	tokenSimilarity := tokenJaccard(a, b)
	return clamp01((0.65 * levSimilarity) + (0.35 * tokenSimilarity))
}

func tokenJaccard(a, b string) float64 {
	aTokens := strings.Fields(a)
	bTokens := strings.Fields(b)
	if len(aTokens) == 0 || len(bTokens) == 0 {
		return 0
	}

	aSet := map[string]struct{}{}
	bSet := map[string]struct{}{}
	for _, tok := range aTokens {
		aSet[tok] = struct{}{}
	}
	for _, tok := range bTokens {
		bSet[tok] = struct{}{}
	}

	intersection := 0
	for tok := range aSet {
		if _, ok := bSet[tok]; ok {
			intersection++
		}
	}
	union := len(aSet) + len(bSet) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

func levenshteinDistance(a, b string) int {
	if a == b {
		return 0
	}
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)

	for j := 0; j <= len(b); j++ {
		prev[j] = j
	}

	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}

			insertCost := curr[j-1] + 1
			deleteCost := prev[j] + 1
			replaceCost := prev[j-1] + cost

			curr[j] = minInt(insertCost, minInt(deleteCost, replaceCost))
		}
		prev, curr = curr, prev
	}

	return prev[len(b)]
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func normalizeForMatching(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}

	s = strings.ReplaceAll(s, " & ", " and ")
	s = strings.ReplaceAll(s, "&", "and")

	featuringPatterns := []string{
		" feat. ", " feat ", " ft. ", " ft ", " featuring ",
	}
	for _, pattern := range featuringPatterns {
		if idx := strings.Index(s, pattern); idx != -1 {
			s = s[:idx]
		}
	}
	s = stripLeadingTrackNumber(s)

	s = stripBracketContent(s, '(', ')')
	s = stripBracketContent(s, '[', ']')

	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == ' ' {
			b.WriteRune(r)
		}
	}
	s = b.String()

	return strings.Join(strings.Fields(s), " ")
}

func stripBracketContent(s string, open, close rune) string {
	var b strings.Builder
	depth := 0
	for _, r := range s {
		if r == open {
			depth++
			continue
		}
		if r == close && depth > 0 {
			depth--
			continue
		}
		if depth == 0 {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func stripLeadingTrackNumber(s string) string {
	fields := strings.Fields(s)
	if len(fields) < 2 {
		return s
	}
	if !isAllDigits(fields[0]) {
		return s
	}
	if len(fields) >= 3 && (fields[1] == "-" || fields[1] == "." || fields[1] == ":") {
		return strings.Join(fields[2:], " ")
	}
	return strings.Join(fields[1:], " ")
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func auditMatchCoverage(args map[string]interface{}) (map[string]interface{}, error) {
	groupBy := asString(args["groupBy"])
	if groupBy == "" {
		groupBy = "month"
	}
	if groupBy != "month" && groupBy != "year" {
		return nil, errors.New("groupBy must be 'month' or 'year'")
	}
	sourceFilter, err := parseSourceFilter(args)
	if err != nil {
		return nil, err
	}
	minClusterSize := 10
	if v, ok := asInt(args["minClusterSize"]); ok && v >= 3 {
		minClusterSize = v
	}

	var periodStart *time.Time
	var periodEnd *time.Time
	scope := map[string]string{}
	if p, ok := asMap(args["period"]); ok {
		start := asString(p["startDate"])
		end := asString(p["endDate"])
		if start != "" {
			parsed, err := parseDate(start)
			if err != nil {
				return nil, fmt.Errorf("invalid period.startDate: %w", err)
			}
			periodStart = &parsed
			scope["startDate"] = start
		}
		if end != "" {
			parsed, err := parseDate(end)
			if err != nil {
				return nil, fmt.Errorf("invalid period.endDate: %w", err)
			}
			periodEnd = &parsed
			scope["endDate"] = end
		}
	}
	if periodStart != nil && periodEnd != nil && periodEnd.Before(*periodStart) {
		return nil, errors.New("period.endDate must be on or after period.startDate")
	}

	resolver, err := getResolver()
	if err != nil {
		return nil, fmt.Errorf("resolver unavailable: %w", err)
	}
	scrobbles, err := getListeningScrobbles()
	if err != nil {
		return nil, fmt.Errorf("listening history unavailable: %w", err)
	}
	scrobbles = filterScrobblesBySource(scrobbles, sourceFilter)

	periodSummary := map[string]*periodCounters{}
	failureClusters := map[string]*clusterInfo{}
	matchedArtistCounts := map[string]int{}
	unmatchedArtistCounts := map[string]int{}
	unmatchedTrackCounts := map[string]*topTrackStat{}
	uniqueMatchedTrackIDs := map[string]struct{}{}

	totalScrobbles := 0
	matchedScrobbles := 0
	unmatchedScrobbles := 0

	for _, sc := range scrobbles {
		if !timestampInOptionalRange(sc.Date, periodStart, periodEnd) {
			continue
		}
		if strings.TrimSpace(sc.Artist) == "" || strings.TrimSpace(sc.Track) == "" {
			continue
		}
		totalScrobbles++

		periodKey := formatPeriodKey(sc.Date, groupBy)
		counter := periodSummary[periodKey]
		if counter == nil {
			counter = &periodCounters{}
			periodSummary[periodKey] = counter
		}
		counter.Total++

		match := resolver.matchScrobble(sc, "medium", true)
		if match.Status == "matched" {
			counter.Matched++
			matchedScrobbles++
			matchedArtistCounts[sc.Artist]++
			if match.CandidateIndex >= 0 && match.CandidateIndex < len(resolver.tracks) {
				uniqueMatchedTrackIDs[resolver.tracks[match.CandidateIndex].ID] = struct{}{}
			}
			continue
		}

		counter.Unmatched++
		unmatchedScrobbles++
		unmatchedArtistCounts[sc.Artist]++

		trackKey := normalizeForMatching(sc.Artist) + "|" + normalizeForMatching(sc.Track)
		trackStat := unmatchedTrackCounts[trackKey]
		if trackStat == nil {
			trackStat = &topTrackStat{
				Key:    trackKey,
				Artist: sc.Artist,
				Track:  sc.Track,
			}
			unmatchedTrackCounts[trackKey] = trackStat
		}
		trackStat.Count++

		pattern := match.FailurePattern
		if pattern == "" {
			pattern = "unclassified"
		}
		cluster := failureClusters[pattern]
		if cluster == nil {
			cluster = &clusterInfo{Examples: []string{}}
			failureClusters[pattern] = cluster
		}
		cluster.Count++
		if len(cluster.Examples) < 5 {
			example := fmt.Sprintf("%s - %s", sc.Artist, sc.Track)
			exists := false
			for _, existing := range cluster.Examples {
				if existing == example {
					exists = true
					break
				}
			}
			if !exists {
				cluster.Examples = append(cluster.Examples, example)
			}
		}
	}

	coverageByPeriod := []map[string]interface{}{}
	periodKeys := make([]string, 0, len(periodSummary))
	for key := range periodSummary {
		periodKeys = append(periodKeys, key)
	}
	sort.Strings(periodKeys)
	for _, key := range periodKeys {
		counter := periodSummary[key]
		matchRate := 0.0
		if counter.Total > 0 {
			matchRate = float64(counter.Matched) / float64(counter.Total)
		}
		coverageByPeriod = append(coverageByPeriod, map[string]interface{}{
			"period":         key,
			"matchRate":      roundFloat(matchRate, 4),
			"unmatchedCount": counter.Unmatched,
		})
	}

	failureClusterList := []map[string]interface{}{}
	for pattern, cluster := range failureClusters {
		if cluster.Count < minClusterSize {
			continue
		}
		failureClusterList = append(failureClusterList, map[string]interface{}{
			"pattern":         pattern,
			"count":           cluster.Count,
			"examples":        cluster.Examples,
			"suggestedRule":   suggestedRuleForPattern(pattern),
			"estimatedImpact": cluster.Count,
		})
	}
	sort.Slice(failureClusterList, func(i, j int) bool {
		ic, _ := failureClusterList[i]["count"].(int)
		jc, _ := failureClusterList[j]["count"].(int)
		if ic == jc {
			return fmt.Sprint(failureClusterList[i]["pattern"]) < fmt.Sprint(failureClusterList[j]["pattern"])
		}
		return ic > jc
	})

	matchRate := 0.0
	if totalScrobbles > 0 {
		matchRate = float64(matchedScrobbles) / float64(totalScrobbles)
	}
	libraryTracksWithPlays := len(uniqueMatchedTrackIDs)
	libraryTrackCoverage := 0.0
	if len(resolver.tracks) > 0 {
		libraryTrackCoverage = float64(libraryTracksWithPlays) / float64(len(resolver.tracks))
	}

	notes := []string{
		fmt.Sprintf("groupBy=%s minClusterSize=%d", groupBy, minClusterSize),
	}
	if resolver.aliasPath != "" {
		notes = append(notes, fmt.Sprintf("alias map loaded: %s", resolver.aliasPath))
	}
	scope["source"] = sourceFilter

	return map[string]interface{}{
		"summary": map[string]interface{}{
			"totalScrobbles":         totalScrobbles,
			"matchedScrobbles":       matchedScrobbles,
			"unmatchedScrobbles":     unmatchedScrobbles,
			"matchRate":              roundFloat(matchRate, 4),
			"libraryTracksWithPlays": libraryTracksWithPlays,
			"libraryTrackCoverage":   roundFloat(libraryTrackCoverage, 4),
		},
		"coverageByPeriod":    coverageByPeriod,
		"failureClusters":     failureClusterList,
		"topMatchedArtists":   rankedCounts(matchedArtistCounts, 20, "artist"),
		"topUnmatchedArtists": rankedCounts(unmatchedArtistCounts, 20, "artist"),
		"topUnmatchedTracks":  rankTrackStats(unmatchedTrackCounts, 20),
		"notes":               notes,
		"scope":               scope,
	}, nil
}

func musicNewDiscoveries(args map[string]interface{}) (map[string]interface{}, error) {
	start, end, label, err := parseEra(args, "Discovery")
	if err != nil {
		return nil, err
	}
	sourceFilter, err := parseSourceFilter(args)
	if err != nil {
		return nil, err
	}
	topN := 25
	if v, ok := asInt(args["topN"]); ok {
		if v < 5 {
			v = 5
		}
		if v > 100 {
			v = 100
		}
		topN = v
	}

	scrobbles, err := getListeningScrobbles()
	if err != nil {
		return nil, fmt.Errorf("listening history unavailable: %w", err)
	}
	scrobbles = filterScrobblesBySource(scrobbles, sourceFilter)

	firstArtistPlay := map[string]int64{}
	firstTrackPlay := map[string]int64{}

	for _, sc := range scrobbles {
		normArtist := normalizeForMatching(sc.Artist)
		normTrack := normalizeForMatching(sc.Track)
		if normArtist == "" || normTrack == "" {
			continue
		}
		trackKey := buildExactKey(normArtist, normTrack)

		if old, ok := firstArtistPlay[normArtist]; !ok || sc.Date < old {
			firstArtistPlay[normArtist] = sc.Date
		}
		if old, ok := firstTrackPlay[trackKey]; !ok || sc.Date < old {
			firstTrackPlay[trackKey] = sc.Date
		}
	}

	startMs := start.UTC().UnixMilli()
	endMs := end.UTC().Add(24 * time.Hour).UnixMilli()

	artistCounts := map[string]int{}
	trackCounts := map[string]int{}
	trackDisplay := map[string]*topTrackStat{}

	for _, sc := range scrobbles {
		if sc.Date < startMs || sc.Date >= endMs {
			continue
		}

		normArtist := normalizeForMatching(sc.Artist)
		normTrack := normalizeForMatching(sc.Track)
		if normArtist == "" || normTrack == "" {
			continue
		}
		trackKey := buildExactKey(normArtist, normTrack)

		if firstArtistPlay[normArtist] >= startMs {
			artistCounts[sc.Artist]++
		}
		if firstTrackPlay[trackKey] >= startMs {
			trackCounts[trackKey]++
			if _, exists := trackDisplay[trackKey]; !exists {
				trackDisplay[trackKey] = &topTrackStat{
					Key:    trackKey,
					Artist: sc.Artist,
					Track:  sc.Track,
				}
			}
			trackDisplay[trackKey].Count++
		}
	}

	return map[string]interface{}{
		"period":     label,
		"source":     sourceFilter,
		"newArtists": rankedCounts(artistCounts, topN, "artist"),
		"newTracks":  rankTrackStats(trackDisplay, topN),
		"summary": map[string]interface{}{
			"newArtistCount": len(artistCounts),
			"newTrackCount":  len(trackCounts),
		},
	}, nil
}

func musicListeningPatterns(args map[string]interface{}) (map[string]interface{}, error) {
	start, end, label, err := parseEra(args, "Patterns")
	if err != nil {
		return nil, err
	}
	sourceFilter, err := parseSourceFilter(args)
	if err != nil {
		return nil, err
	}

	scrobbles, err := getListeningScrobbles()
	if err != nil {
		return nil, fmt.Errorf("listening history unavailable: %w", err)
	}
	scrobbles = filterScrobblesBySource(scrobbles, sourceFilter)

	startMs := start.UTC().UnixMilli()
	endMs := end.UTC().Add(24 * time.Hour).UnixMilli()

	hourlyCounts := make([]int, 24)
	dayOfWeekCounts := make([]int, 7)
	artistCounts := map[string]int{}

	type session struct {
		start int64
		end   int64
		count int
	}
	sessions := []session{}
	var currentSession *session

	inPeriod := []lastFMScrobble{}
	for _, sc := range scrobbles {
		if sc.Date >= startMs && sc.Date < endMs {
			inPeriod = append(inPeriod, sc)
		}
	}
	sort.Slice(inPeriod, func(i, j int) bool {
		return inPeriod[i].Date < inPeriod[j].Date
	})

	for _, sc := range inPeriod {
		t := time.UnixMilli(sc.Date).UTC()
		hourlyCounts[t.Hour()]++
		dayOfWeekCounts[int(t.Weekday())]++
		artistCounts[normalizeForMatching(sc.Artist)]++

		if currentSession == nil {
			currentSession = &session{start: sc.Date, end: sc.Date, count: 1}
		} else {
			if sc.Date-currentSession.end > 30*60*1000 {
				sessions = append(sessions, *currentSession)
				currentSession = &session{start: sc.Date, end: sc.Date, count: 1}
			} else {
				currentSession.end = sc.Date
				currentSession.count++
			}
		}
	}
	if currentSession != nil {
		sessions = append(sessions, *currentSession)
	}

	totalSessionDuration := 0.0
	maxSessionDuration := 0.0
	totalSessionTracks := 0
	for _, s := range sessions {
		dur := float64(s.end-s.start) / (1000 * 60)
		totalSessionDuration += dur
		totalSessionTracks += s.count
		if dur > maxSessionDuration {
			maxSessionDuration = dur
		}
	}

	avgSessionDuration := 0.0
	avgTracksPerSession := 0.0
	if len(sessions) > 0 {
		avgSessionDuration = totalSessionDuration / float64(len(sessions))
		avgTracksPerSession = float64(totalSessionTracks) / float64(len(sessions))
	}

	days := []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
	dayStats := []map[string]interface{}{}
	for i, count := range dayOfWeekCounts {
		dayStats = append(dayStats, map[string]interface{}{
			"day":   days[i],
			"count": count,
		})
	}

	hourStats := []map[string]interface{}{}
	for i, count := range hourlyCounts {
		hourStats = append(hourStats, map[string]interface{}{
			"hour":  i,
			"count": count,
		})
	}

	artistsPer100Scrobbles := 0.0
	if len(inPeriod) > 0 {
		artistsPer100Scrobbles = roundFloat(float64(len(artistCounts))/float64(len(inPeriod))*100, 2)
	}

	return map[string]interface{}{
		"period": label,
		"source": sourceFilter,
		"sessions": map[string]interface{}{
			"totalSessions":       len(sessions),
			"avgDurationMinutes":  roundFloat(avgSessionDuration, 2),
			"maxDurationMinutes":  roundFloat(maxSessionDuration, 2),
			"avgTracksPerSession": roundFloat(avgTracksPerSession, 2),
		},
		"timeOfDay": hourStats,
		"dayOfWeek": dayStats,
		"diversity": map[string]interface{}{
			"uniqueArtists":          len(artistCounts),
			"artistsPer100Scrobbles": artistsPer100Scrobbles,
		},
	}, nil
}

func musicListeningSummary(args map[string]interface{}) (map[string]interface{}, error) {
	start, end, label, err := parseEra(args, "Summary")
	if err != nil {
		return nil, err
	}
	sourceFilter, err := parseSourceFilter(args)
	if err != nil {
		return nil, err
	}
	topN := 25
	if v, ok := asInt(args["topN"]); ok {
		if v < 5 {
			v = 5
		}
		if v > 100 {
			v = 100
		}
		topN = v
	}

	resolver, err := getResolver()
	if err != nil {
		return nil, fmt.Errorf("resolver unavailable: %w", err)
	}
	scrobbles, err := getListeningScrobbles()
	if err != nil {
		return nil, fmt.Errorf("listening history unavailable: %w", err)
	}
	scrobbles = filterScrobblesBySource(scrobbles, sourceFilter)

	stats := analyzeEra(label, start, end, scrobbles, resolver)
	playsPerDay := 0.0
	if stats.Days > 0 {
		playsPerDay = roundFloat(float64(stats.TotalScrobbles)/stats.Days, 2)
	}

	return map[string]interface{}{
		"summary": map[string]interface{}{
			"totalScrobbles": stats.TotalScrobbles,
			"uniqueTracks":   len(stats.TrackCounts),
			"uniqueArtists":  len(stats.ArtistCounts),
			"days":           roundFloat(stats.Days, 2),
			"playsPerDay":    playsPerDay,
		},
		"source":     sourceFilter,
		"topTracks":  rankTrackStats(stats.TrackDisplay, topN),
		"topArtists": rankedCounts(stats.ArtistCounts, topN, "artist"),
		"topGenres":  rankedCounts(stats.GenreCounts, topN, "genre"),
	}, nil
}

func musicGenreProfile(args map[string]interface{}) (map[string]interface{}, error) {
	start, end, label, err := parseEra(args, "Genre Profile")
	if err != nil {
		return nil, err
	}
	sourceFilter, err := parseSourceFilter(args)
	if err != nil {
		return nil, err
	}
	topN := 25
	if v, ok := asInt(args["topN"]); ok {
		if v < 5 {
			v = 5
		}
		if v > 100 {
			v = 100
		}
		topN = v
	}

	resolver, err := getResolver()
	if err != nil {
		return nil, fmt.Errorf("resolver unavailable: %w", err)
	}
	scrobbles, err := getListeningScrobbles()
	if err != nil {
		return nil, fmt.Errorf("listening history unavailable: %w", err)
	}
	scrobbles = filterScrobblesBySource(scrobbles, sourceFilter)

	stats := analyzeEra(label, start, end, scrobbles, resolver)

	return map[string]interface{}{
		"period":           label,
		"source":           sourceFilter,
		"totalScrobbles":   stats.TotalScrobbles,
		"topGenres":        rankedCounts(stats.GenreCounts, topN, "genre"),
		"diversityEntropy": roundFloat(shannonEntropyNormalized(stats.GenreCounts), 4),
	}, nil
}

func compareEras(args map[string]interface{}) (map[string]interface{}, error) {
	eraA, ok := asMap(args["eraA"])
	if !ok {
		return nil, errors.New("eraA is required")
	}
	eraB, ok := asMap(args["eraB"])
	if !ok {
		return nil, errors.New("eraB is required")
	}

	aStart, aEnd, aLabel, err := parseEra(eraA, "Era A")
	if err != nil {
		return nil, fmt.Errorf("invalid eraA: %w", err)
	}
	bStart, bEnd, bLabel, err := parseEra(eraB, "Era B")
	if err != nil {
		return nil, fmt.Errorf("invalid eraB: %w", err)
	}
	topN := 25
	if v, ok := asInt(args["topN"]); ok {
		if v < 5 {
			v = 5
		}
		if v > 100 {
			v = 100
		}
		topN = v
	}
	sourceFilter, err := parseSourceFilter(args)
	if err != nil {
		return nil, err
	}

	resolver, err := getResolver()
	if err != nil {
		return nil, fmt.Errorf("resolver unavailable: %w", err)
	}
	scrobbles, err := getListeningScrobbles()
	if err != nil {
		return nil, fmt.Errorf("listening history unavailable: %w", err)
	}
	scrobbles = filterScrobblesBySource(scrobbles, sourceFilter)

	aStats := analyzeEra(aLabel, aStart, aEnd, scrobbles, resolver)
	bStats := analyzeEra(bLabel, bStart, bEnd, scrobbles, resolver)

	noveltyRateA := noveltyRate(aStats.TrackCounts)
	noveltyRateB := noveltyRate(bStats.TrackCounts)
	entropyA := shannonEntropyNormalized(aStats.TrackCounts)
	entropyB := shannonEntropyNormalized(bStats.TrackCounts)

	artistJaccard := jaccardFromCountMaps(aStats.ArtistCounts, bStats.ArtistCounts)
	trackJaccard := jaccardFromCountMaps(aStats.TrackCounts, bStats.TrackCounts)
	persistentFavorites := computePersistentFavorites(aStats, bStats, 10)
	rising, falling := computeRisingFallingTracks(aStats, bStats, topN)
	genreShift := computeGenreShift(aStats, bStats, minInt(topN, 20))

	insightBullets := []string{
		fmt.Sprintf("%s had %d scrobbles across %d unique tracks; %s had %d scrobbles across %d unique tracks.",
			aStats.Label, aStats.TotalScrobbles, len(aStats.TrackCounts), bStats.Label, bStats.TotalScrobbles, len(bStats.TrackCounts)),
		fmt.Sprintf("Artist overlap (Jaccard) was %.3f and track overlap was %.3f.", artistJaccard, trackJaccard),
		fmt.Sprintf("Novelty shifted from %.3f to %.3f; diversity entropy shifted from %.3f to %.3f.",
			noveltyRateA, noveltyRateB, entropyA, entropyB),
	}
	if len(rising) > 0 {
		artist, _ := rising[0]["artist"].(string)
		track, _ := rising[0]["track"].(string)
		delta, _ := rising[0]["delta"].(float64)
		insightBullets = append(insightBullets,
			fmt.Sprintf("Strongest rising track: %s - %s (delta %.3f plays/day).", artist, track, delta))
	}

	return map[string]interface{}{
		"source": sourceFilter,
		"summary": map[string]interface{}{
			"scrobblesA":        aStats.TotalScrobbles,
			"scrobblesB":        bStats.TotalScrobbles,
			"uniqueTracksA":     len(aStats.TrackCounts),
			"uniqueTracksB":     len(bStats.TrackCounts),
			"noveltyRateA":      roundFloat(noveltyRateA, 4),
			"noveltyRateB":      roundFloat(noveltyRateB, 4),
			"diversityEntropyA": roundFloat(entropyA, 4),
			"diversityEntropyB": roundFloat(entropyB, 4),
		},
		"overlap": map[string]interface{}{
			"artistJaccard":       roundFloat(artistJaccard, 4),
			"trackJaccard":        roundFloat(trackJaccard, 4),
			"persistentFavorites": persistentFavorites,
		},
		"rising":         rising,
		"falling":        falling,
		"genreShift":     genreShift,
		"insightBullets": insightBullets,
	}, nil
}

func findDormantReturns(args map[string]interface{}) (map[string]interface{}, error) {
	sourceFilter, err := parseSourceFilter(args)
	if err != nil {
		return nil, err
	}

	returnPeriod, ok := asMap(args["returnPeriod"])
	if !ok {
		return nil, errors.New("returnPeriod is required")
	}
	returnStart, err := parseDate(asString(returnPeriod["startDate"]))
	if err != nil {
		return nil, fmt.Errorf("invalid returnPeriod.startDate: %w", err)
	}
	returnEnd, err := parseDate(asString(returnPeriod["endDate"]))
	if err != nil {
		return nil, fmt.Errorf("invalid returnPeriod.endDate: %w", err)
	}
	if returnEnd.Before(returnStart) {
		return nil, errors.New("returnPeriod.endDate must be on or after returnPeriod.startDate")
	}

	var historyStart *time.Time
	if raw := asString(args["historyStartDate"]); raw != "" {
		parsed, err := parseDate(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid historyStartDate: %w", err)
		}
		historyStart = &parsed
		if historyStart.After(returnEnd) {
			return nil, errors.New("historyStartDate must be on or before returnPeriod.endDate")
		}
	}

	minDormancyDays := 365 * 5
	if v, ok := asInt(args["minDormancyDays"]); ok && v >= 30 {
		minDormancyDays = v
	}
	minPreReturnPlays := 2
	if v, ok := asInt(args["minPreReturnPlays"]); ok && v >= 1 {
		minPreReturnPlays = v
	}
	minReturnPlays := 2
	if v, ok := asInt(args["minReturnPlays"]); ok && v >= 1 {
		minReturnPlays = v
	}
	topN := 25
	if v, ok := asInt(args["topN"]); ok {
		if v < 1 {
			v = 1
		}
		if v > 200 {
			v = 200
		}
		topN = v
	}
	strictness := "medium"
	if v := asString(args["strictness"]); v != "" {
		strictness = v
	}
	if strictness != "high" && strictness != "medium" && strictness != "low" {
		return nil, errors.New("strictness must be 'high', 'medium', or 'low'")
	}

	resolver, err := getResolver()
	if err != nil {
		return nil, fmt.Errorf("resolver unavailable: %w", err)
	}
	scrobbles, err := getListeningScrobbles()
	if err != nil {
		return nil, fmt.Errorf("listening history unavailable: %w", err)
	}
	scrobbles = filterScrobblesBySource(scrobbles, sourceFilter)

	type dormantTrackAccumulator struct {
		TrackID       string
		Artist        string
		Track         string
		Album         string
		PreCount      int
		ReturnCount   int
		ScopedCount   int
		LastBeforeMs  int64
		FirstReturnMs int64
	}
	byTrack := map[string]*dormantTrackAccumulator{}
	totalScrobblesInScope := 0
	matchedScrobblesInScope := 0
	returnDays := int(returnEnd.Sub(returnStart).Hours()/24) + 1
	if returnDays < 1 {
		returnDays = 1
	}

	for _, sc := range scrobbles {
		if historyStart != nil && sc.Date < historyStart.UTC().UnixMilli() {
			continue
		}
		if !timestampInOptionalRange(sc.Date, nil, &returnEnd) {
			continue
		}
		if strings.TrimSpace(sc.Artist) == "" || strings.TrimSpace(sc.Track) == "" {
			continue
		}
		totalScrobblesInScope++

		match := resolver.matchScrobble(sc, strictness, true)
		if match.Status != "matched" || match.CandidateIndex < 0 || match.CandidateIndex >= len(resolver.tracks) {
			continue
		}
		matchedScrobblesInScope++
		canonical := resolver.tracks[match.CandidateIndex]
		acc := byTrack[canonical.ID]
		if acc == nil {
			acc = &dormantTrackAccumulator{
				TrackID: canonical.ID,
				Artist:  canonical.Artist,
				Track:   canonical.Name,
				Album:   canonical.Album,
			}
			byTrack[canonical.ID] = acc
		}
		acc.ScopedCount++
		if sc.Date < returnStart.UTC().UnixMilli() {
			acc.PreCount++
			if sc.Date > acc.LastBeforeMs {
				acc.LastBeforeMs = sc.Date
			}
			continue
		}
		if timestampInOptionalRange(sc.Date, &returnStart, &returnEnd) {
			acc.ReturnCount++
			if acc.FirstReturnMs == 0 || sc.Date < acc.FirstReturnMs {
				acc.FirstReturnMs = sc.Date
			}
		}
	}

	results := []map[string]interface{}{}
	artistAggregate := map[string]map[string]int{}
	for _, acc := range byTrack {
		if acc.PreCount < minPreReturnPlays || acc.ReturnCount < minReturnPlays {
			continue
		}
		if acc.LastBeforeMs == 0 || acc.FirstReturnMs == 0 {
			continue
		}
		dormancyDays := int(time.UnixMilli(acc.FirstReturnMs).UTC().Sub(time.UnixMilli(acc.LastBeforeMs).UTC()).Hours() / 24)
		if dormancyDays < minDormancyDays {
			continue
		}
		dormancyYears := float64(dormancyDays) / 365.25
		returnRate := float64(acc.ReturnCount) / float64(returnDays)
		results = append(results, map[string]interface{}{
			"trackId":              acc.TrackID,
			"artist":               acc.Artist,
			"track":                acc.Track,
			"album":                acc.Album,
			"lastPreReturnPlay":    time.UnixMilli(acc.LastBeforeMs).UTC().Format("2006-01-02"),
			"firstReturnPlay":      time.UnixMilli(acc.FirstReturnMs).UTC().Format("2006-01-02"),
			"dormancyDays":         dormancyDays,
			"dormancyYears":        roundFloat(dormancyYears, 2),
			"preReturnPlays":       acc.PreCount,
			"returnPeriodPlays":    acc.ReturnCount,
			"returnPeriodPlayRate": roundFloat(returnRate, 4),
			"scopedPlays":          acc.ScopedCount,
		})

		agg := artistAggregate[acc.Artist]
		if agg == nil {
			agg = map[string]int{
				"trackCount": 0,
				"playCount":  0,
			}
			artistAggregate[acc.Artist] = agg
		}
		agg["trackCount"]++
		agg["playCount"] += acc.ReturnCount
	}

	sort.Slice(results, func(i, j int) bool {
		ir, _ := results[i]["returnPeriodPlays"].(int)
		jr, _ := results[j]["returnPeriodPlays"].(int)
		if ir == jr {
			id, _ := results[i]["dormancyDays"].(int)
			jd, _ := results[j]["dormancyDays"].(int)
			if id == jd {
				ia := strings.ToLower(fmt.Sprint(results[i]["artist"]))
				ja := strings.ToLower(fmt.Sprint(results[j]["artist"]))
				if ia == ja {
					return strings.ToLower(fmt.Sprint(results[i]["track"])) < strings.ToLower(fmt.Sprint(results[j]["track"]))
				}
				return ia < ja
			}
			return id > jd
		}
		return ir > jr
	})
	totalDormantReturnedTracks := len(results)
	if topN < len(results) {
		results = results[:topN]
	}

	topArtists := []map[string]interface{}{}
	for artist, agg := range artistAggregate {
		topArtists = append(topArtists, map[string]interface{}{
			"artist":            artist,
			"dormantTrackCount": agg["trackCount"],
			"returnPeriodPlays": agg["playCount"],
		})
	}
	sort.Slice(topArtists, func(i, j int) bool {
		ip, _ := topArtists[i]["returnPeriodPlays"].(int)
		jp, _ := topArtists[j]["returnPeriodPlays"].(int)
		if ip == jp {
			it, _ := topArtists[i]["dormantTrackCount"].(int)
			jt, _ := topArtists[j]["dormantTrackCount"].(int)
			if it == jt {
				return strings.ToLower(fmt.Sprint(topArtists[i]["artist"])) < strings.ToLower(fmt.Sprint(topArtists[j]["artist"]))
			}
			return it > jt
		}
		return ip > jp
	})
	totalDormantReturnedArtists := len(topArtists)
	if topN < len(topArtists) {
		topArtists = topArtists[:topN]
	}

	matchRate := 0.0
	if totalScrobblesInScope > 0 {
		matchRate = float64(matchedScrobblesInScope) / float64(totalScrobblesInScope)
	}
	scope := map[string]interface{}{
		"returnPeriod": map[string]string{
			"startDate": returnStart.Format("2006-01-02"),
			"endDate":   returnEnd.Format("2006-01-02"),
		},
	}
	if historyStart != nil {
		scope["historyStartDate"] = historyStart.Format("2006-01-02")
	}
	scope["source"] = sourceFilter

	notes := []string{
		fmt.Sprintf("strictness=%s minDormancyDays=%d minPreReturnPlays=%d minReturnPlays=%d",
			strictness, minDormancyDays, minPreReturnPlays, minReturnPlays),
		fmt.Sprintf("source=%s", sourceFilter),
		"Only canonically matched scrobbles are considered for dormancy and return calculations.",
	}
	if resolver.aliasPath != "" {
		notes = append(notes, fmt.Sprintf("alias map loaded: %s", resolver.aliasPath))
	}

	return map[string]interface{}{
		"summary": map[string]interface{}{
			"totalScrobblesInScope":      totalScrobblesInScope,
			"matchedScrobblesInScope":    matchedScrobblesInScope,
			"matchRateInScope":           roundFloat(matchRate, 4),
			"dormantReturnedTrackCount":  totalDormantReturnedTracks,
			"dormantReturnedArtistCount": totalDormantReturnedArtists,
			"resultsReturned":            len(results),
			"returnPeriodDays":           returnDays,
		},
		"scope":             scope,
		"topDormantReturns": results,
		"topReturnArtists":  topArtists,
		"notes":             notes,
	}, nil
}

func musicStreaksAndBursts(args map[string]interface{}) (map[string]interface{}, error) {
	start, end, label, err := parseEra(args, "Streaks & Bursts")
	if err != nil {
		return nil, err
	}
	sourceFilter, err := parseSourceFilter(args)
	if err != nil {
		return nil, err
	}

	topN := 10
	if v, ok := asInt(args["topN"]); ok {
		if v < 1 {
			v = 1
		}
		if v > 100 {
			v = 100
		}
		topN = v
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

	loc, tzName, err := parseTimezoneArg(args)
	if err != nil {
		return nil, err
	}

	scrobbles, err := getListeningScrobbles()
	if err != nil {
		return nil, fmt.Errorf("listening history unavailable: %w", err)
	}
	scrobbles = filterScrobblesBySource(scrobbles, sourceFilter)

	report := buildStreakBurstReport(scrobbles, start, end, loc, sessionGapMinutes, topN)
	report["period"] = label
	report["timezone"] = tzName
	report["source"] = sourceFilter
	report["scope"] = map[string]string{
		"startDate": start.Format("2006-01-02"),
		"endDate":   end.Format("2006-01-02"),
		"source":    sourceFilter,
	}

	return report, nil
}

func musicYearStory(args map[string]interface{}) (map[string]interface{}, error) {
	year, ok := asInt(args["year"])
	if !ok {
		return nil, errors.New("year is required")
	}
	start, end, err := yearBounds(year)
	if err != nil {
		return nil, err
	}
	sourceFilter, err := parseSourceFilter(args)
	if err != nil {
		return nil, err
	}

	topN := 10
	if v, ok := asInt(args["topN"]); ok {
		if v < 3 {
			v = 3
		}
		if v > 50 {
			v = 50
		}
		topN = v
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

	includeDormantReturns := true
	if v, ok := asBool(args["includeDormantReturns"]); ok {
		includeDormantReturns = v
	}

	loc, tzName, err := parseTimezoneArg(args)
	if err != nil {
		return nil, err
	}

	scrobbles, err := getListeningScrobbles()
	if err != nil {
		return nil, fmt.Errorf("listening history unavailable: %w", err)
	}
	scrobbles = filterScrobblesBySource(scrobbles, sourceFilter)

	resolver, err := getResolver()
	if err != nil {
		return nil, fmt.Errorf("resolver unavailable: %w", err)
	}

	periodLabel := fmt.Sprintf("%d", year)
	stats := analyzeEra(periodLabel, start, end, scrobbles, resolver)
	topArtists, topTracks, uniqueArtists, uniqueTracks := rankDisplayArtistsAndTracks(scrobbles, start, end, topN)

	streakBurst := buildStreakBurstReport(scrobbles, start, end, loc, sessionGapMinutes, topN)
	streakSummary, _ := streakBurst["summary"].(map[string]interface{})
	activeDays, _ := asInt(streakSummary["activeDays"])
	if activeDays < 0 {
		activeDays = 0
	}
	nightShare := 0.0
	if v, ok := streakSummary["nightPlayShare"].(float64); ok {
		nightShare = v
	}

	playsPerActiveDay := 0.0
	if activeDays > 0 {
		playsPerActiveDay = float64(stats.TotalScrobbles) / float64(activeDays)
	}

	discoveryArgs := map[string]interface{}{
		"startDate": start.Format("2006-01-02"),
		"endDate":   end.Format("2006-01-02"),
		"topN":      topN,
		"source":    sourceFilter,
	}
	discovery, discoveryErr := musicNewDiscoveries(discoveryArgs)
	if discoveryErr != nil {
		return nil, fmt.Errorf("discovery analysis failed: %w", discoveryErr)
	}
	discoverySummary, _ := discovery["summary"].(map[string]interface{})
	newArtistCount, _ := asInt(discoverySummary["newArtistCount"])
	newTrackCount, _ := asInt(discoverySummary["newTrackCount"])

	sourceBreakdown, _ := streakBurst["sourceBreakdown"].([]map[string]interface{})

	spotifyMs := int64(0)
	estimatedCount := 0
	for _, sc := range scrobbles {
		if !timestampInOptionalRange(sc.Date, &start, &end) {
			continue
		}
		if strings.TrimSpace(sc.Artist) == "" || strings.TrimSpace(sc.Track) == "" {
			continue
		}
		if sc.MsPlayed > 0 {
			spotifyMs += sc.MsPlayed
		} else {
			estimatedCount++
		}
	}
	measuredSpotifyMinutes := float64(spotifyMs) / (1000 * 60)
	estimatedMinutes := measuredSpotifyMinutes + (float64(estimatedCount) * 3.5)

	var yearOverYear map[string]interface{}
	if year > 1900 {
		prevStart, prevEnd, boundsErr := yearBounds(year - 1)
		if boundsErr == nil {
			compareArgs := map[string]interface{}{
				"eraA": map[string]interface{}{
					"startDate": prevStart.Format("2006-01-02"),
					"endDate":   prevEnd.Format("2006-01-02"),
					"label":     fmt.Sprintf("%d", year-1),
				},
				"eraB": map[string]interface{}{
					"startDate": start.Format("2006-01-02"),
					"endDate":   end.Format("2006-01-02"),
					"label":     fmt.Sprintf("%d", year),
				},
				"topN":   topN,
				"source": sourceFilter,
			}
			if cmp, cmpErr := compareEras(compareArgs); cmpErr == nil {
				summary, _ := cmp["summary"].(map[string]interface{})
				overlap, _ := cmp["overlap"].(map[string]interface{})
				yearOverYear = map[string]interface{}{
					"previousYear":           year - 1,
					"artistOverlapJaccard":   overlap["artistJaccard"],
					"trackOverlapJaccard":    overlap["trackJaccard"],
					"noveltyRatePrevious":    summary["noveltyRateA"],
					"noveltyRateCurrent":     summary["noveltyRateB"],
					"diversityPrev":          summary["diversityEntropyA"],
					"diversityCurrent":       summary["diversityEntropyB"],
					"persistentFavorites":    overlap["persistentFavorites"],
					"topRisers":              cmp["rising"],
					"topDecliners":           cmp["falling"],
					"genreShift":             cmp["genreShift"],
					"comparisonBulletPoints": cmp["insightBullets"],
				}
			}
		}
	}

	var dormantReturns map[string]interface{}
	if includeDormantReturns {
		dormantArgs := map[string]interface{}{
			"returnPeriod": map[string]interface{}{
				"startDate": start.Format("2006-01-02"),
				"endDate":   end.Format("2006-01-02"),
			},
			"minDormancyDays":   3650,
			"minPreReturnPlays": 2,
			"minReturnPlays":    2,
			"topN":              minInt(topN, 5),
			"strictness":        "medium",
			"source":            sourceFilter,
		}
		if dr, drErr := findDormantReturns(dormantArgs); drErr == nil {
			dormantReturns = dr
		}
	}

	topBurstDays, _ := streakBurst["topBurstDays"].([]map[string]interface{})
	streaks, _ := streakBurst["streaks"].(map[string]interface{})
	longestArtistRun := streaks["longestArtistRun"]

	storyCards := []map[string]interface{}{}
	insightBullets := []string{}

	storyCards = append(storyCards, map[string]interface{}{
		"id":       "total_plays",
		"title":    "Total Plays",
		"headline": fmt.Sprintf("%d plays", stats.TotalScrobbles),
		"detail":   fmt.Sprintf("%d active days (%.2f plays per active day)", activeDays, playsPerActiveDay),
	})
	insightBullets = append(insightBullets,
		fmt.Sprintf("You logged %d plays in %d across %d active days.", stats.TotalScrobbles, year, activeDays))

	if len(sourceBreakdown) > 0 {
		parts := make([]string, 0, len(sourceBreakdown))
		for i := 0; i < len(sourceBreakdown) && i < 3; i++ {
			source := fmt.Sprint(sourceBreakdown[i]["source"])
			count, _ := asInt(sourceBreakdown[i]["count"])
			share := 0.0
			if v, ok := sourceBreakdown[i]["share"].(float64); ok {
				share = v
			}
			parts = append(parts, fmt.Sprintf("%s %d%% (%d)", source, int(math.Round(share*100)), count))
		}
		storyCards = append(storyCards, map[string]interface{}{
			"id":       "source_mix",
			"title":    "Source Mix",
			"headline": strings.Join(parts, " · "),
			"detail":   "Combined listening sources in this year",
		})
	}

	if len(topArtists) > 0 {
		artist := fmt.Sprint(topArtists[0]["artist"])
		count, _ := asInt(topArtists[0]["count"])
		storyCards = append(storyCards, map[string]interface{}{
			"id":       "top_artist",
			"title":    "Top Artist",
			"headline": artist,
			"detail":   fmt.Sprintf("%d plays", count),
		})
		insightBullets = append(insightBullets, fmt.Sprintf("Top artist: %s (%d plays).", artist, count))
	}

	if len(topTracks) > 0 {
		artist := fmt.Sprint(topTracks[0]["artist"])
		track := fmt.Sprint(topTracks[0]["track"])
		count, _ := asInt(topTracks[0]["count"])
		storyCards = append(storyCards, map[string]interface{}{
			"id":       "top_track",
			"title":    "Top Track",
			"headline": fmt.Sprintf("%s - %s", artist, track),
			"detail":   fmt.Sprintf("%d plays", count),
		})
	}

	storyCards = append(storyCards, map[string]interface{}{
		"id":       "discovery",
		"title":    "Discovery",
		"headline": fmt.Sprintf("%d new artists", newArtistCount),
		"detail":   fmt.Sprintf("%d new tracks first heard in %d", newTrackCount, year),
	})
	insightBullets = append(insightBullets,
		fmt.Sprintf("You discovered %d new artists and %d new tracks.", newArtistCount, newTrackCount))

	if len(topBurstDays) > 0 {
		date := fmt.Sprint(topBurstDays[0]["date"])
		plays, _ := asInt(topBurstDays[0]["plays"])
		storyCards = append(storyCards, map[string]interface{}{
			"id":       "peak_day",
			"title":    "Peak Day",
			"headline": fmt.Sprintf("%s", date),
			"detail":   fmt.Sprintf("%d plays", plays),
		})
	}

	if run, ok := longestArtistRun.(map[string]interface{}); ok && len(run) > 0 {
		artist := fmt.Sprint(run["artist"])
		count, _ := asInt(run["count"])
		storyCards = append(storyCards, map[string]interface{}{
			"id":       "artist_streak",
			"title":    "Longest Artist Run",
			"headline": fmt.Sprintf("%s", artist),
			"detail":   fmt.Sprintf("%d consecutive plays", count),
		})
	}

	if dormantReturns != nil {
		summary, _ := dormantReturns["summary"].(map[string]interface{})
		count, _ := asInt(summary["dormantReturnedTrackCount"])
		if count > 0 {
			storyCards = append(storyCards, map[string]interface{}{
				"id":       "time_capsule",
				"title":    "Time Capsule Returns",
				"headline": fmt.Sprintf("%d dormant tracks returned", count),
				"detail":   "Tracks that came back after long gaps",
			})
		}
	}

	nightPct := int(math.Round(nightShare * 100))
	if stats.TotalScrobbles > 0 {
		insightBullets = append(insightBullets,
			fmt.Sprintf("%d%% of plays happened at night (10pm-6am, %s).", nightPct, tzName))
	}

	summary := map[string]interface{}{
		"totalScrobbles":          stats.TotalScrobbles,
		"uniqueTracks":            uniqueTracks,
		"uniqueArtists":           uniqueArtists,
		"activeDays":              activeDays,
		"playsPerActiveDay":       roundFloat(playsPerActiveDay, 2),
		"nightPlayShare":          roundFloat(nightShare, 4),
		"sourceBreakdown":         sourceBreakdown,
		"measuredSpotifyMinutes":  roundFloat(measuredSpotifyMinutes, 2),
		"estimatedTotalMinutes":   roundFloat(estimatedMinutes, 2),
		"estimatedFromNonSpotify": estimatedCount,
		"source":                  sourceFilter,
	}

	return map[string]interface{}{
		"year":   year,
		"source": sourceFilter,
		"scope": map[string]string{
			"startDate": start.Format("2006-01-02"),
			"endDate":   end.Format("2006-01-02"),
			"timezone":  tzName,
			"source":    sourceFilter,
		},
		"summary":          summary,
		"topArtists":       topArtists,
		"topTracks":        topTracks,
		"discoveries":      discovery,
		"yearOverYear":     yearOverYear,
		"dormantReturns":   dormantReturns,
		"streaksAndBursts": streakBurst,
		"storyCards":       storyCards,
		"insightBullets":   insightBullets,
	}, nil
}

func buildStreakBurstReport(scrobbles []lastFMScrobble, start, end time.Time, loc *time.Location, sessionGapMinutes, topN int) map[string]interface{} {
	type dayAccumulator struct {
		plays      int
		artistKeys map[string]struct{}
		trackKeys  map[string]struct{}
		sources    map[string]int
	}
	type sessionAccumulator struct {
		start          int64
		end            int64
		plays          int
		artistCounts   map[string]int
		trackCounts    map[string]int
		uniqueTrackSet map[string]struct{}
		sources        map[string]int
	}
	type trackAccumulator struct {
		artist string
		track  string
		count  int
		first  int64
		last   int64
	}

	const sessionTrackSep = "\x1f"

	inPeriod := make([]lastFMScrobble, 0)
	for _, sc := range scrobbles {
		if !timestampInOptionalRange(sc.Date, &start, &end) {
			continue
		}
		if strings.TrimSpace(sc.Artist) == "" || strings.TrimSpace(sc.Track) == "" {
			continue
		}
		inPeriod = append(inPeriod, sc)
	}

	sort.Slice(inPeriod, func(i, j int) bool {
		return inPeriod[i].Date < inPeriod[j].Date
	})

	dayMap := map[string]*dayAccumulator{}
	sourceCounts := map[string]int{}
	trackWindows := map[string]*trackAccumulator{}
	nightCount := 0

	currArtistKey := ""
	currArtistDisplay := ""
	currArtistCount := 0
	currArtistStart := int64(0)
	bestArtistKey := ""
	bestArtistDisplay := ""
	bestArtistCount := 0
	bestArtistStart := int64(0)
	bestArtistEnd := int64(0)

	currTrackKey := ""
	currTrackArtist := ""
	currTrackName := ""
	currTrackCount := 0
	currTrackStart := int64(0)
	bestTrackKey := ""
	bestTrackArtist := ""
	bestTrackName := ""
	bestTrackCount := 0
	bestTrackStart := int64(0)
	bestTrackEnd := int64(0)

	sessionGapMs := int64(sessionGapMinutes) * 60 * 1000
	sessions := []*sessionAccumulator{}
	var currentSession *sessionAccumulator

	for _, sc := range inPeriod {
		source := scrobbleSource(sc)
		sourceCounts[source]++

		normArtist := normalizeForMatching(sc.Artist)
		normTrack := normalizeForMatching(sc.Track)
		if normArtist == "" || normTrack == "" {
			continue
		}
		normTrackKey := buildExactKey(normArtist, normTrack)

		localTime := time.UnixMilli(sc.Date).In(loc)
		if hour := localTime.Hour(); hour >= 22 || hour < 6 {
			nightCount++
		}
		dayKey := localTime.Format("2006-01-02")

		day := dayMap[dayKey]
		if day == nil {
			day = &dayAccumulator{
				artistKeys: map[string]struct{}{},
				trackKeys:  map[string]struct{}{},
				sources:    map[string]int{},
			}
			dayMap[dayKey] = day
		}
		day.plays++
		day.artistKeys[normArtist] = struct{}{}
		day.trackKeys[normTrackKey] = struct{}{}
		day.sources[source]++

		if trackWindows[normTrackKey] == nil {
			trackWindows[normTrackKey] = &trackAccumulator{
				artist: sc.Artist,
				track:  sc.Track,
				count:  1,
				first:  sc.Date,
				last:   sc.Date,
			}
		} else {
			trackWindows[normTrackKey].count++
			if sc.Date < trackWindows[normTrackKey].first {
				trackWindows[normTrackKey].first = sc.Date
			}
			if sc.Date > trackWindows[normTrackKey].last {
				trackWindows[normTrackKey].last = sc.Date
			}
		}

		if currArtistKey == normArtist {
			currArtistCount++
		} else {
			currArtistKey = normArtist
			currArtistDisplay = sc.Artist
			currArtistCount = 1
			currArtistStart = sc.Date
		}
		if currArtistCount > bestArtistCount {
			bestArtistCount = currArtistCount
			bestArtistKey = currArtistKey
			bestArtistDisplay = currArtistDisplay
			bestArtistStart = currArtistStart
			bestArtistEnd = sc.Date
		}

		if currTrackKey == normTrackKey {
			currTrackCount++
		} else {
			currTrackKey = normTrackKey
			currTrackArtist = sc.Artist
			currTrackName = sc.Track
			currTrackCount = 1
			currTrackStart = sc.Date
		}
		if currTrackCount > bestTrackCount {
			bestTrackCount = currTrackCount
			bestTrackKey = currTrackKey
			bestTrackArtist = currTrackArtist
			bestTrackName = currTrackName
			bestTrackStart = currTrackStart
			bestTrackEnd = sc.Date
		}

		if currentSession == nil || sc.Date-currentSession.end > sessionGapMs {
			currentSession = &sessionAccumulator{
				start:          sc.Date,
				end:            sc.Date,
				plays:          0,
				artistCounts:   map[string]int{},
				trackCounts:    map[string]int{},
				uniqueTrackSet: map[string]struct{}{},
				sources:        map[string]int{},
			}
			sessions = append(sessions, currentSession)
		}
		currentSession.end = sc.Date
		currentSession.plays++
		currentSession.artistCounts[sc.Artist]++
		currentSession.trackCounts[sc.Artist+sessionTrackSep+sc.Track]++
		currentSession.uniqueTrackSet[normTrackKey] = struct{}{}
		currentSession.sources[source]++
	}

	dayRows := make([]map[string]interface{}, 0, len(dayMap))
	dayCounts := make([]int, 0, len(dayMap))
	dayKeys := make([]string, 0, len(dayMap))
	maxDailyPlays := 0
	for dayKey, day := range dayMap {
		if day.plays > maxDailyPlays {
			maxDailyPlays = day.plays
		}
		dayCounts = append(dayCounts, day.plays)
		dayKeys = append(dayKeys, dayKey)
		dayRows = append(dayRows, map[string]interface{}{
			"date":          dayKey,
			"plays":         day.plays,
			"uniqueArtists": len(day.artistKeys),
			"uniqueTracks":  len(day.trackKeys),
			"sources":       sortSourceCounts(day.sources, day.plays),
		})
	}
	sort.Slice(dayRows, func(i, j int) bool {
		ip, _ := asInt(dayRows[i]["plays"])
		jp, _ := asInt(dayRows[j]["plays"])
		if ip == jp {
			return fmt.Sprint(dayRows[i]["date"]) < fmt.Sprint(dayRows[j]["date"])
		}
		return ip > jp
	})
	if topN < len(dayRows) {
		dayRows = dayRows[:topN]
	}

	sort.Ints(dayCounts)
	medianDailyPlays := 0
	if len(dayCounts) > 0 {
		medianDailyPlays = dayCounts[len(dayCounts)/2]
	}

	sort.Strings(dayKeys)
	longestDayStreak := 0
	longestDayStreakStart := ""
	longestDayStreakEnd := ""
	if len(dayKeys) > 0 {
		currLen := 1
		currStart := dayKeys[0]
		currEnd := dayKeys[0]
		longestDayStreak = 1
		longestDayStreakStart = dayKeys[0]
		longestDayStreakEnd = dayKeys[0]

		for i := 1; i < len(dayKeys); i++ {
			prevDay, _ := time.Parse("2006-01-02", dayKeys[i-1])
			currDay, _ := time.Parse("2006-01-02", dayKeys[i])
			if prevDay.AddDate(0, 0, 1).Equal(currDay) {
				currLen++
				currEnd = dayKeys[i]
			} else {
				currLen = 1
				currStart = dayKeys[i]
				currEnd = dayKeys[i]
			}
			if currLen > longestDayStreak {
				longestDayStreak = currLen
				longestDayStreakStart = currStart
				longestDayStreakEnd = currEnd
			}
		}
	}

	sessionRows := make([]map[string]interface{}, 0, len(sessions))
	totalSessionDurationMinutes := 0.0
	totalSessionTracks := 0
	for _, s := range sessions {
		durationMinutes := float64(s.end-s.start) / (1000 * 60)
		totalSessionDurationMinutes += durationMinutes
		totalSessionTracks += s.plays

		topArtist, topArtistCount := topStringCount(s.artistCounts)
		topTrackKey, topTrackCount := topStringCount(s.trackCounts)
		topTrackArtist := ""
		topTrackName := ""
		if parts := strings.SplitN(topTrackKey, sessionTrackSep, 2); len(parts) == 2 {
			topTrackArtist = parts[0]
			topTrackName = parts[1]
		}

		sessionRows = append(sessionRows, map[string]interface{}{
			"start":         time.UnixMilli(s.start).In(loc).Format(time.RFC3339),
			"end":           time.UnixMilli(s.end).In(loc).Format(time.RFC3339),
			"plays":         s.plays,
			"durationMin":   roundFloat(durationMinutes, 2),
			"uniqueArtists": len(s.artistCounts),
			"uniqueTracks":  len(s.uniqueTrackSet),
			"topArtist": map[string]interface{}{
				"artist": topArtist,
				"count":  topArtistCount,
			},
			"topTrack": map[string]interface{}{
				"artist": topTrackArtist,
				"track":  topTrackName,
				"count":  topTrackCount,
			},
			"sources": sortSourceCounts(s.sources, s.plays),
		})
	}
	sort.Slice(sessionRows, func(i, j int) bool {
		ip, _ := asInt(sessionRows[i]["plays"])
		jp, _ := asInt(sessionRows[j]["plays"])
		if ip == jp {
			id, _ := sessionRows[i]["durationMin"].(float64)
			jd, _ := sessionRows[j]["durationMin"].(float64)
			if id == jd {
				return fmt.Sprint(sessionRows[i]["start"]) < fmt.Sprint(sessionRows[j]["start"])
			}
			return id > jd
		}
		return ip > jp
	})
	if topN < len(sessionRows) {
		sessionRows = sessionRows[:topN]
	}

	avgSessionMinutes := 0.0
	avgTracksPerSession := 0.0
	if len(sessions) > 0 {
		avgSessionMinutes = totalSessionDurationMinutes / float64(len(sessions))
		avgTracksPerSession = float64(totalSessionTracks) / float64(len(sessions))
	}

	concentratedTracks := make([]map[string]interface{}, 0)
	for _, window := range trackWindows {
		if window.count < 5 {
			continue
		}
		spanDays := int((window.last - window.first) / (24 * 60 * 60 * 1000))
		if spanDays < 0 {
			spanDays = 0
		}
		burstScore := float64(window.count) / float64(maxInt(1, spanDays+1))
		concentratedTracks = append(concentratedTracks, map[string]interface{}{
			"artist":     window.artist,
			"track":      window.track,
			"plays":      window.count,
			"spanDays":   spanDays,
			"burstScore": roundFloat(burstScore, 4),
			"firstPlay":  time.UnixMilli(window.first).In(loc).Format("2006-01-02"),
			"lastPlay":   time.UnixMilli(window.last).In(loc).Format("2006-01-02"),
		})
	}
	sort.Slice(concentratedTracks, func(i, j int) bool {
		is, _ := concentratedTracks[i]["burstScore"].(float64)
		js, _ := concentratedTracks[j]["burstScore"].(float64)
		if is == js {
			ip, _ := asInt(concentratedTracks[i]["plays"])
			jp, _ := asInt(concentratedTracks[j]["plays"])
			if ip == jp {
				return strings.ToLower(fmt.Sprint(concentratedTracks[i]["artist"])+fmt.Sprint(concentratedTracks[i]["track"])) <
					strings.ToLower(fmt.Sprint(concentratedTracks[j]["artist"])+fmt.Sprint(concentratedTracks[j]["track"]))
			}
			return ip > jp
		}
		return is > js
	})
	if topN < len(concentratedTracks) {
		concentratedTracks = concentratedTracks[:topN]
	}

	totalScrobbles := len(inPeriod)
	nightPlayShare := 0.0
	if totalScrobbles > 0 {
		nightPlayShare = float64(nightCount) / float64(totalScrobbles)
	}

	var longestArtistRun interface{}
	if bestArtistCount > 0 {
		longestArtistRun = map[string]interface{}{
			"artist":          bestArtistDisplay,
			"artistNormKey":   bestArtistKey,
			"count":           bestArtistCount,
			"start":           time.UnixMilli(bestArtistStart).In(loc).Format(time.RFC3339),
			"end":             time.UnixMilli(bestArtistEnd).In(loc).Format(time.RFC3339),
			"durationMinutes": roundFloat(float64(bestArtistEnd-bestArtistStart)/(1000*60), 2),
		}
	}

	var longestTrackRun interface{}
	if bestTrackCount > 0 {
		longestTrackRun = map[string]interface{}{
			"artist":          bestTrackArtist,
			"track":           bestTrackName,
			"trackNormKey":    bestTrackKey,
			"count":           bestTrackCount,
			"start":           time.UnixMilli(bestTrackStart).In(loc).Format(time.RFC3339),
			"end":             time.UnixMilli(bestTrackEnd).In(loc).Format(time.RFC3339),
			"durationMinutes": roundFloat(float64(bestTrackEnd-bestTrackStart)/(1000*60), 2),
		}
	}

	var longestActiveDayStreak interface{}
	if longestDayStreak > 0 {
		longestActiveDayStreak = map[string]interface{}{
			"days":      longestDayStreak,
			"startDate": longestDayStreakStart,
			"endDate":   longestDayStreakEnd,
		}
	}

	return map[string]interface{}{
		"summary": map[string]interface{}{
			"totalScrobbles":      totalScrobbles,
			"activeDays":          len(dayMap),
			"maxDailyPlays":       maxDailyPlays,
			"medianDailyPlays":    medianDailyPlays,
			"totalSessions":       len(sessions),
			"avgSessionMinutes":   roundFloat(avgSessionMinutes, 2),
			"avgTracksPerSession": roundFloat(avgTracksPerSession, 2),
			"nightPlayShare":      roundFloat(nightPlayShare, 4),
		},
		"sourceBreakdown":    sortSourceCounts(sourceCounts, totalScrobbles),
		"streaks":            map[string]interface{}{"longestArtistRun": longestArtistRun, "longestTrackRun": longestTrackRun, "longestActiveDayStreak": longestActiveDayStreak},
		"topBurstDays":       dayRows,
		"topBurstSessions":   sessionRows,
		"concentratedTracks": concentratedTracks,
	}
}

func rankDisplayArtistsAndTracks(scrobbles []lastFMScrobble, start, end time.Time, topN int) ([]map[string]interface{}, []map[string]interface{}, int, int) {
	type artistStat struct {
		display string
		count   int
	}
	type trackStat struct {
		artist string
		track  string
		count  int
	}

	artistCounts := map[string]*artistStat{}
	trackCounts := map[string]*trackStat{}

	for _, sc := range scrobbles {
		if !timestampInOptionalRange(sc.Date, &start, &end) {
			continue
		}
		normArtist := normalizeForMatching(sc.Artist)
		normTrack := normalizeForMatching(sc.Track)
		if normArtist == "" || normTrack == "" {
			continue
		}

		if artistCounts[normArtist] == nil {
			artistCounts[normArtist] = &artistStat{display: sc.Artist}
		}
		artistCounts[normArtist].count++

		trackKey := buildExactKey(normArtist, normTrack)
		if trackCounts[trackKey] == nil {
			trackCounts[trackKey] = &trackStat{artist: sc.Artist, track: sc.Track}
		}
		trackCounts[trackKey].count++
	}

	type artistRow struct {
		key     string
		display string
		count   int
	}
	artists := make([]artistRow, 0, len(artistCounts))
	for key, stat := range artistCounts {
		artists = append(artists, artistRow{key: key, display: stat.display, count: stat.count})
	}
	sort.Slice(artists, func(i, j int) bool {
		if artists[i].count == artists[j].count {
			return strings.ToLower(artists[i].display) < strings.ToLower(artists[j].display)
		}
		return artists[i].count > artists[j].count
	})
	if topN > len(artists) {
		topN = len(artists)
	}
	topArtists := make([]map[string]interface{}, 0, topN)
	for i := 0; i < topN; i++ {
		topArtists = append(topArtists, map[string]interface{}{
			"artist": artists[i].display,
			"count":  artists[i].count,
		})
	}

	type trackRow struct {
		artist string
		track  string
		count  int
	}
	tracks := make([]trackRow, 0, len(trackCounts))
	for _, stat := range trackCounts {
		tracks = append(tracks, trackRow{
			artist: stat.artist,
			track:  stat.track,
			count:  stat.count,
		})
	}
	sort.Slice(tracks, func(i, j int) bool {
		if tracks[i].count == tracks[j].count {
			if strings.EqualFold(tracks[i].artist, tracks[j].artist) {
				return strings.ToLower(tracks[i].track) < strings.ToLower(tracks[j].track)
			}
			return strings.ToLower(tracks[i].artist) < strings.ToLower(tracks[j].artist)
		}
		return tracks[i].count > tracks[j].count
	})
	trackLimit := topN
	if trackLimit > len(tracks) {
		trackLimit = len(tracks)
	}
	topTracks := make([]map[string]interface{}, 0, trackLimit)
	for i := 0; i < trackLimit; i++ {
		topTracks = append(topTracks, map[string]interface{}{
			"artist": tracks[i].artist,
			"track":  tracks[i].track,
			"count":  tracks[i].count,
		})
	}

	return topArtists, topTracks, len(artistCounts), len(trackCounts)
}

func topStringCount(counts map[string]int) (string, int) {
	bestKey := ""
	bestCount := 0
	for key, count := range counts {
		if count > bestCount {
			bestCount = count
			bestKey = key
			continue
		}
		if count == bestCount && bestCount > 0 && strings.ToLower(key) < strings.ToLower(bestKey) {
			bestKey = key
		}
	}
	return bestKey, bestCount
}

func sortSourceCounts(counts map[string]int, total int) []map[string]interface{} {
	type row struct {
		source string
		count  int
	}
	rows := make([]row, 0, len(counts))
	for source, count := range counts {
		rows = append(rows, row{
			source: source,
			count:  count,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].count == rows[j].count {
			return strings.ToLower(rows[i].source) < strings.ToLower(rows[j].source)
		}
		return rows[i].count > rows[j].count
	})

	out := make([]map[string]interface{}, 0, len(rows))
	for _, r := range rows {
		share := 0.0
		if total > 0 {
			share = float64(r.count) / float64(total)
		}
		out = append(out, map[string]interface{}{
			"source": r.source,
			"count":  r.count,
			"share":  roundFloat(share, 4),
		})
	}
	return out
}

func parseSourceFilter(args map[string]interface{}) (string, error) {
	source := strings.ToLower(strings.TrimSpace(asString(args["source"])))
	if source == "" || source == "all" || source == "both" {
		return "all", nil
	}
	if source != "lastfm" && source != "spotify" {
		return "", errors.New("source must be 'all', 'lastfm', or 'spotify'")
	}
	return source, nil
}

func scrobbleSource(sc lastFMScrobble) string {
	source := strings.ToLower(strings.TrimSpace(sc.Source))
	switch source {
	case "", "last.fm":
		return "lastfm"
	case "lastfm", "spotify":
		return source
	default:
		return source
	}
}

func filterScrobblesBySource(scrobbles []lastFMScrobble, source string) []lastFMScrobble {
	if source == "" || source == "all" {
		return scrobbles
	}
	filtered := make([]lastFMScrobble, 0, len(scrobbles))
	for _, sc := range scrobbles {
		if scrobbleSource(sc) == source {
			filtered = append(filtered, sc)
		}
	}
	return filtered
}

func parseTimezoneArg(args map[string]interface{}) (*time.Location, string, error) {
	tz := asString(args["timezone"])
	if tz == "" {
		return time.UTC, "UTC", nil
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, "", fmt.Errorf("invalid timezone: %q", tz)
	}
	return loc, tz, nil
}

func yearBounds(year int) (time.Time, time.Time, error) {
	if year < 1900 || year > 2100 {
		return time.Time{}, time.Time{}, errors.New("year must be between 1900 and 2100")
	}
	start := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(year, 12, 31, 0, 0, 0, 0, time.UTC)
	return start, end, nil
}

func getListeningScrobbles() ([]lastFMScrobble, error) {
	listeningOnce.Do(func() {
		root, err := detectProjectRoot()
		if err != nil {
			listeningErr = err
			return
		}

		historyPath := resolveListeningHistoryPath(root)
		raw, err := os.ReadFile(historyPath)
		if err == nil {
			var history listeningHistoryData
			if err := json.Unmarshal(raw, &history); err == nil && len(history.Events) > 0 {
				listeningCache = history.Events
				sort.Slice(listeningCache, func(i, j int) bool {
					return listeningCache[i].Date < listeningCache[j].Date
				})
				return
			}

			// Backward compatibility: allow directly pointed legacy Last.fm stats JSON.
			var legacy lastFMData
			if err := json.Unmarshal(raw, &legacy); err == nil && len(legacy.Scrobbles) > 0 {
				listeningCache = legacy.Scrobbles
				sort.Slice(listeningCache, func(i, j int) bool {
					return listeningCache[i].Date < listeningCache[j].Date
				})
				return
			}

			listeningErr = fmt.Errorf("parse %s: expected listening-history JSON with events[]", historyPath)
			return
		}

		if !os.IsNotExist(err) {
			listeningErr = fmt.Errorf("read %s: %w", historyPath, err)
			return
		}

		// Backward compatibility: fall back to legacy Last.fm-only input if merged history is absent.
		legacyPath := resolveLastFMStatsPath(root)
		legacyRaw, legacyReadErr := os.ReadFile(legacyPath)
		if legacyReadErr != nil {
			listeningErr = fmt.Errorf("read %s: %w (fallback read %s: %v)", historyPath, err, legacyPath, legacyReadErr)
			return
		}
		var legacyData lastFMData
		if err := json.Unmarshal(legacyRaw, &legacyData); err != nil {
			listeningErr = fmt.Errorf("parse fallback %s: %w", legacyPath, err)
			return
		}
		listeningCache = legacyData.Scrobbles
		sort.Slice(listeningCache, func(i, j int) bool {
			return listeningCache[i].Date < listeningCache[j].Date
		})
	})
	return listeningCache, listeningErr
}

func resolveListeningHistoryPath(root string) string {
	if path := strings.TrimSpace(os.Getenv("MP3_LISTENING_HISTORY_FILE")); path != "" {
		return resolvePath(root, path)
	}
	if dataDir := strings.TrimSpace(os.Getenv("MP3_DATA_DIR")); dataDir != "" {
		return filepath.Join(resolvePath(root, dataDir), "listening-history.json")
	}
	return filepath.Join(root, "data", "derived", "core", "listening-history.json")
}

func resolveLastFMStatsPath(root string) string {
	if path := strings.TrimSpace(os.Getenv("MP3_LASTFM_FILE")); path != "" {
		return resolvePath(root, path)
	}
	filename := fmt.Sprintf("lastfmstats-%s.json", lastFMUsername())
	if dir := strings.TrimSpace(os.Getenv("MP3_LASTFM_DIR")); dir != "" {
		return filepath.Join(resolvePath(root, dir), filename)
	}
	return filepath.Join(root, "data", "inputs", "lastfm", filename)
}

func lastFMUsername() string {
	if username := strings.TrimSpace(os.Getenv("LASTFM_USERNAME")); username != "" {
		return username
	}
	return "riebschlager"
}

func timestampInOptionalRange(tsMs int64, start, end *time.Time) bool {
	if start == nil && end == nil {
		return true
	}
	ts := time.UnixMilli(tsMs).UTC()
	if start != nil {
		if ts.Before(start.UTC()) {
			return false
		}
	}
	if end != nil {
		if !ts.Before(end.UTC().Add(24 * time.Hour)) {
			return false
		}
	}
	return true
}

func formatPeriodKey(tsMs int64, groupBy string) string {
	t := time.UnixMilli(tsMs).UTC()
	if groupBy == "year" {
		return fmt.Sprintf("%04d", t.Year())
	}
	return fmt.Sprintf("%04d-%02d", t.Year(), int(t.Month()))
}

func (r *trackResolver) collectScopedCandidateIndexes(normArtist, normTrack, normAlbum string) []int {
	seen := map[int]struct{}{}
	add := func(indexes []int) {
		for _, idx := range indexes {
			seen[idx] = struct{}{}
		}
	}
	if normArtist != "" {
		add(r.artistIndex[normArtist])
	}
	if normTrack != "" {
		add(r.trackIndex[normTrack])
	}
	if normAlbum != "" {
		add(r.albumIndex[normAlbum])
	}
	out := make([]int, 0, len(seen))
	for idx := range seen {
		out = append(out, idx)
	}
	return out
}

func (r *trackResolver) matchScrobble(sc lastFMScrobble, strictness string, allowFuzzy bool) matchResult {
	normArtistRaw := normalizeForMatching(sc.Artist)
	normTrackRaw := normalizeForMatching(sc.Track)
	normAlbumRaw := normalizeForMatching(sc.Album)
	normArtist, artistAliasSteps := r.aliases.Canonicalize("artist", normArtistRaw)
	normTrack, trackAliasSteps := r.aliases.Canonicalize("track", normTrackRaw)
	normAlbum, albumAliasSteps := r.aliases.Canonicalize("album", normAlbumRaw)
	aliasSteps := append(append(artistAliasSteps, trackAliasSteps...), albumAliasSteps...)

	if normArtist == "" || normTrack == "" {
		return matchResult{
			Status:         "unmatched",
			Method:         "fuzzy",
			Confidence:     0.0,
			CandidateIndex: -1,
			FailurePattern: "invalid_metadata",
			AliasSteps:     aliasSteps,
		}
	}

	exact := r.exactIndex[buildExactKey(normArtist, normTrack)]
	if len(exact) == 1 {
		method := "exact_norm"
		if len(aliasSteps) > 0 {
			method = "manual_alias"
		}
		return matchResult{
			Status:         "matched",
			Method:         method,
			Confidence:     0.9,
			CandidateIndex: exact[0],
			AliasSteps:     aliasSteps,
		}
	}
	if len(exact) > 1 {
		bestIndex := exact[0]
		bestScore := -1.0
		for _, idx := range exact {
			score := stringSimilarity(normAlbum, r.tracks[idx].NormAlbum)
			if score > bestScore {
				bestScore = score
				bestIndex = idx
			}
		}
		if bestScore >= 0.8 {
			method := "exact_norm"
			if len(aliasSteps) > 0 {
				method = "manual_alias"
			}
			return matchResult{
				Status:         "matched",
				Method:         method,
				Confidence:     0.9,
				CandidateIndex: bestIndex,
				AliasSteps:     aliasSteps,
			}
		}
		return matchResult{
			Status:         "ambiguous",
			Method:         "exact_norm",
			Confidence:     0.8,
			CandidateIndex: bestIndex,
			FailurePattern: "duplicate_library_key",
			AliasSteps:     aliasSteps,
		}
	}

	if !allowFuzzy {
		return matchResult{
			Status:         "unmatched",
			Method:         "fuzzy",
			Confidence:     0.0,
			CandidateIndex: -1,
			FailurePattern: determineFailurePattern(sc, normArtist, normTrack, r, 0),
			AliasSteps:     aliasSteps,
		}
	}

	candidates := r.collectScopedCandidateIndexes(normArtist, normTrack, normAlbum)
	if len(candidates) == 0 {
		return matchResult{
			Status:         "unmatched",
			Method:         "fuzzy",
			Confidence:     0.0,
			CandidateIndex: -1,
			FailurePattern: determineFailurePattern(sc, normArtist, normTrack, r, 0),
			AliasSteps:     aliasSteps,
		}
	}

	scored := make([]scoredCandidate, 0, len(candidates))
	for _, idx := range candidates {
		scored = append(scored, scoreCandidate(idx, normArtist, normTrack, normAlbum, false, 0, r.tracks[idx]))
	}
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].confidence > scored[j].confidence
	})
	top := scored[0]
	next := 0.0
	if len(scored) > 1 {
		next = scored[1].confidence
	}
	matchedThreshold, ambiguousThreshold := thresholdsForStrictness(strictness)
	gap := top.confidence - next
	method := top.method
	if len(aliasSteps) > 0 && method == "exact_norm" {
		method = "manual_alias"
	}

	if top.confidence >= matchedThreshold && gap >= 0.05 {
		return matchResult{
			Status:         "matched",
			Method:         method,
			Confidence:     top.confidence,
			CandidateIndex: top.index,
			AliasSteps:     aliasSteps,
		}
	}
	if top.confidence >= ambiguousThreshold {
		return matchResult{
			Status:         "ambiguous",
			Method:         method,
			Confidence:     top.confidence,
			CandidateIndex: top.index,
			FailurePattern: "low_confidence_fuzzy",
			AliasSteps:     aliasSteps,
		}
	}
	return matchResult{
		Status:         "unmatched",
		Method:         method,
		Confidence:     top.confidence,
		CandidateIndex: top.index,
		FailurePattern: determineFailurePattern(sc, normArtist, normTrack, r, len(candidates)),
		AliasSteps:     aliasSteps,
	}
}

func determineFailurePattern(sc lastFMScrobble, normArtist, normTrack string, resolver *trackResolver, candidateCount int) string {
	raw := strings.ToLower(strings.TrimSpace(sc.Track + " " + sc.Artist))
	if raw == "" || normArtist == "" || normTrack == "" {
		return "invalid_metadata"
	}
	if strings.Contains(raw, " feat ") || strings.Contains(raw, " ft ") || strings.Contains(raw, " featuring ") {
		return "featuring_variation"
	}
	if strings.Contains(raw, " remix") || strings.Contains(raw, " live") || strings.Contains(raw, " version") || strings.Contains(raw, " edit") {
		return "version_suffix"
	}
	if strings.TrimSpace(sc.Album) == "" {
		return "missing_album"
	}
	hasArtist := len(resolver.artistIndex[normArtist]) > 0
	hasTrack := len(resolver.trackIndex[normTrack]) > 0
	switch {
	case hasArtist && !hasTrack:
		return "title_mismatch"
	case !hasArtist && hasTrack:
		return "artist_mismatch"
	case candidateCount > 0:
		return "low_confidence_fuzzy"
	default:
		return "no_candidate"
	}
}

func suggestedRuleForPattern(pattern string) string {
	switch pattern {
	case "featuring_variation":
		return "expand featuring token normalization to include punctuated and localized variants"
	case "version_suffix":
		return "strip remix/live/version/edit suffix tokens before fuzzy scoring"
	case "missing_album":
		return "fall back to artist+track exact matching when album is empty"
	case "title_mismatch":
		return "add track-level alias entries for frequent title variants"
	case "artist_mismatch":
		return "add artist alias mappings and transliteration rules"
	case "duplicate_library_key":
		return "use album and duration tie-breakers for duplicate normalized keys"
	case "low_confidence_fuzzy":
		return "increase candidate features (duration/year) or add manual aliases"
	default:
		return "review top examples and add targeted alias or normalization rules"
	}
}

func rankedCounts(counts map[string]int, limit int, keyName string) []map[string]interface{} {
	type pair struct {
		Key   string
		Count int
	}
	pairs := make([]pair, 0, len(counts))
	for key, count := range counts {
		pairs = append(pairs, pair{Key: key, Count: count})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].Count == pairs[j].Count {
			return strings.ToLower(pairs[i].Key) < strings.ToLower(pairs[j].Key)
		}
		return pairs[i].Count > pairs[j].Count
	})
	if limit > len(pairs) {
		limit = len(pairs)
	}
	out := make([]map[string]interface{}, 0, limit)
	for i := 0; i < limit; i++ {
		out = append(out, map[string]interface{}{
			keyName: pairs[i].Key,
			"count": pairs[i].Count,
		})
	}
	return out
}

func rankTrackStats(stats map[string]*topTrackStat, limit int) []map[string]interface{} {
	items := make([]topTrackStat, 0, len(stats))
	for _, item := range stats {
		items = append(items, *item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count == items[j].Count {
			if strings.EqualFold(items[i].Artist, items[j].Artist) {
				return strings.ToLower(items[i].Track) < strings.ToLower(items[j].Track)
			}
			return strings.ToLower(items[i].Artist) < strings.ToLower(items[j].Artist)
		}
		return items[i].Count > items[j].Count
	})
	if limit > len(items) {
		limit = len(items)
	}
	out := make([]map[string]interface{}, 0, limit)
	for i := 0; i < limit; i++ {
		out = append(out, map[string]interface{}{
			"artist": items[i].Artist,
			"track":  items[i].Track,
			"count":  items[i].Count,
		})
	}
	return out
}

func analyzeEra(label string, start, end time.Time, scrobbles []lastFMScrobble, resolver *trackResolver) eraAnalysis {
	stats := eraAnalysis{
		Label:          label,
		Start:          start,
		End:            end,
		TotalScrobbles: 0,
		Days:           maxFloat(1, end.Sub(start).Hours()/24+1),
		TrackCounts:    map[string]int{},
		ArtistCounts:   map[string]int{},
		GenreCounts:    map[string]int{},
		TrackDisplay:   map[string]*topTrackStat{},
	}

	for _, sc := range scrobbles {
		if !timestampInOptionalRange(sc.Date, &start, &end) {
			continue
		}
		if strings.TrimSpace(sc.Artist) == "" || strings.TrimSpace(sc.Track) == "" {
			continue
		}
		stats.TotalScrobbles++

		normArtist := resolver.aliases.CanonicalValue("artist", normalizeForMatching(sc.Artist))
		normTrack := resolver.aliases.CanonicalValue("track", normalizeForMatching(sc.Track))
		if normArtist == "" || normTrack == "" {
			continue
		}
		trackKey := buildExactKey(normArtist, normTrack)
		stats.TrackCounts[trackKey]++
		stats.ArtistCounts[normArtist]++

		trackStat, exists := stats.TrackDisplay[trackKey]
		if !exists {
			displayArtist := sc.Artist
			displayTrack := sc.Track
			if exact := resolver.exactIndex[trackKey]; len(exact) > 0 {
				t := resolver.tracks[exact[0]]
				if t.Artist != "" {
					displayArtist = t.Artist
				}
				if t.Name != "" {
					displayTrack = t.Name
				}
			}
			trackStat = &topTrackStat{
				Key:    trackKey,
				Artist: displayArtist,
				Track:  displayTrack,
			}
			stats.TrackDisplay[trackKey] = trackStat
		}
		trackStat.Count++

		if exact := resolver.exactIndex[trackKey]; len(exact) > 0 {
			genre := strings.TrimSpace(resolver.tracks[exact[0]].Genre)
			if genre != "" {
				stats.GenreCounts[genre]++
			}
		}
	}

	return stats
}

func noveltyRate(trackCounts map[string]int) float64 {
	if len(trackCounts) == 0 {
		return 0
	}
	singletons := 0
	for _, count := range trackCounts {
		if count == 1 {
			singletons++
		}
	}
	return float64(singletons) / float64(len(trackCounts))
}

func shannonEntropyNormalized(counts map[string]int) float64 {
	if len(counts) <= 1 {
		return 0
	}
	total := 0
	for _, count := range counts {
		total += count
	}
	if total == 0 {
		return 0
	}
	entropy := 0.0
	for _, count := range counts {
		if count == 0 {
			continue
		}
		p := float64(count) / float64(total)
		entropy -= p * math.Log(p)
	}
	maxEntropy := math.Log(float64(len(counts)))
	if maxEntropy == 0 {
		return 0
	}
	return clamp01(entropy / maxEntropy)
}

func jaccardFromCountMaps(a, b map[string]int) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	union := map[string]struct{}{}
	intersection := 0
	for key := range a {
		union[key] = struct{}{}
	}
	for key := range b {
		if _, ok := a[key]; ok {
			intersection++
		}
		union[key] = struct{}{}
	}
	if len(union) == 0 {
		return 0
	}
	return float64(intersection) / float64(len(union))
}

func computePersistentFavorites(a, b eraAnalysis, limit int) []map[string]interface{} {
	type item struct {
		Artist string
		Track  string
		CountA int
		CountB int
		RateA  float64
		RateB  float64
		Score  float64
	}
	items := []item{}
	for key, countA := range a.TrackCounts {
		countB := b.TrackCounts[key]
		if countA == 0 || countB == 0 {
			continue
		}
		display := a.TrackDisplay[key]
		if display == nil || (display.Artist == "" && display.Track == "") {
			display = b.TrackDisplay[key]
		}
		if display == nil {
			continue
		}
		rateA := float64(countA) / a.Days
		rateB := float64(countB) / b.Days
		items = append(items, item{
			Artist: display.Artist,
			Track:  display.Track,
			CountA: countA,
			CountB: countB,
			RateA:  rateA,
			RateB:  rateB,
			Score:  minFloat(rateA, rateB),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Score == items[j].Score {
			return strings.ToLower(items[i].Artist+"|"+items[i].Track) < strings.ToLower(items[j].Artist+"|"+items[j].Track)
		}
		return items[i].Score > items[j].Score
	})
	if limit > len(items) {
		limit = len(items)
	}
	out := make([]map[string]interface{}, 0, limit)
	for i := 0; i < limit; i++ {
		out = append(out, map[string]interface{}{
			"artist": items[i].Artist,
			"track":  items[i].Track,
			"countA": items[i].CountA,
			"countB": items[i].CountB,
			"rateA":  roundFloat(items[i].RateA, 4),
			"rateB":  roundFloat(items[i].RateB, 4),
		})
	}
	return out
}

func computeRisingFallingTracks(a, b eraAnalysis, limit int) ([]map[string]interface{}, []map[string]interface{}) {
	type row struct {
		Artist string
		Track  string
		CountA int
		CountB int
		RateA  float64
		RateB  float64
		Delta  float64
	}
	keys := map[string]struct{}{}
	for key := range a.TrackCounts {
		keys[key] = struct{}{}
	}
	for key := range b.TrackCounts {
		keys[key] = struct{}{}
	}

	risingRows := []row{}
	fallingRows := []row{}
	for key := range keys {
		countA := a.TrackCounts[key]
		countB := b.TrackCounts[key]
		if countA+countB < 2 {
			continue
		}
		rateA := float64(countA) / a.Days
		rateB := float64(countB) / b.Days
		display := b.TrackDisplay[key]
		if display == nil || (display.Artist == "" && display.Track == "") {
			display = a.TrackDisplay[key]
		}
		if display == nil {
			continue
		}
		rowVal := row{
			Artist: display.Artist,
			Track:  display.Track,
			CountA: countA,
			CountB: countB,
			RateA:  rateA,
			RateB:  rateB,
			Delta:  rateB - rateA,
		}
		if rowVal.Delta > 0 {
			risingRows = append(risingRows, rowVal)
		} else if rowVal.Delta < 0 {
			fallingRows = append(fallingRows, rowVal)
		}
	}

	sort.Slice(risingRows, func(i, j int) bool {
		if risingRows[i].Delta == risingRows[j].Delta {
			return strings.ToLower(risingRows[i].Artist+"|"+risingRows[i].Track) < strings.ToLower(risingRows[j].Artist+"|"+risingRows[j].Track)
		}
		return risingRows[i].Delta > risingRows[j].Delta
	})
	sort.Slice(fallingRows, func(i, j int) bool {
		if fallingRows[i].Delta == fallingRows[j].Delta {
			return strings.ToLower(fallingRows[i].Artist+"|"+fallingRows[i].Track) < strings.ToLower(fallingRows[j].Artist+"|"+fallingRows[j].Track)
		}
		return fallingRows[i].Delta < fallingRows[j].Delta
	})

	rising := []map[string]interface{}{}
	for i := 0; i < len(risingRows) && i < limit; i++ {
		rowVal := risingRows[i]
		rising = append(rising, map[string]interface{}{
			"artist": rowVal.Artist,
			"track":  rowVal.Track,
			"countA": rowVal.CountA,
			"countB": rowVal.CountB,
			"rateA":  roundFloat(rowVal.RateA, 4),
			"rateB":  roundFloat(rowVal.RateB, 4),
			"delta":  roundFloat(rowVal.Delta, 4),
		})
	}
	falling := []map[string]interface{}{}
	for i := 0; i < len(fallingRows) && i < limit; i++ {
		rowVal := fallingRows[i]
		falling = append(falling, map[string]interface{}{
			"artist": rowVal.Artist,
			"track":  rowVal.Track,
			"countA": rowVal.CountA,
			"countB": rowVal.CountB,
			"rateA":  roundFloat(rowVal.RateA, 4),
			"rateB":  roundFloat(rowVal.RateB, 4),
			"delta":  roundFloat(rowVal.Delta, 4),
		})
	}
	return rising, falling
}

func computeGenreShift(a, b eraAnalysis, limit int) []map[string]interface{} {
	type row struct {
		Genre string
		RateA float64
		RateB float64
		Delta float64
	}
	keys := map[string]struct{}{}
	for key := range a.GenreCounts {
		keys[key] = struct{}{}
	}
	for key := range b.GenreCounts {
		keys[key] = struct{}{}
	}

	rows := []row{}
	for genre := range keys {
		rateA := float64(a.GenreCounts[genre]) / a.Days
		rateB := float64(b.GenreCounts[genre]) / b.Days
		if rateA == 0 && rateB == 0 {
			continue
		}
		rows = append(rows, row{
			Genre: genre,
			RateA: rateA,
			RateB: rateB,
			Delta: rateB - rateA,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		ai := math.Abs(rows[i].Delta)
		aj := math.Abs(rows[j].Delta)
		if ai == aj {
			return strings.ToLower(rows[i].Genre) < strings.ToLower(rows[j].Genre)
		}
		return ai > aj
	})
	if limit > len(rows) {
		limit = len(rows)
	}

	out := make([]map[string]interface{}, 0, limit)
	for i := 0; i < limit; i++ {
		out = append(out, map[string]interface{}{
			"genre": rows[i].Genre,
			"rateA": roundFloat(rows[i].RateA, 4),
			"rateB": roundFloat(rows[i].RateB, 4),
			"delta": roundFloat(rows[i].Delta, 4),
		})
	}
	return out
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func reloadAliasMap(args map[string]interface{}) (map[string]interface{}, error) {
	start := time.Now()
	previousResolver, _ := getResolver()
	previousAliasPath := ""
	previousAliasCount := 0
	if previousResolver != nil {
		previousAliasPath = previousResolver.aliasPath
		previousAliasCount = previousResolver.aliases.Count()
	}

	reloadedResolver, err := forceReloadResolver()
	if err != nil {
		return nil, fmt.Errorf("failed to reload resolver: %w", err)
	}

	reloadedAliasPath := reloadedResolver.aliasPath
	status := "reloaded"
	if previousAliasPath != reloadedAliasPath {
		status = "reloaded_with_alias_path_change"
	}

	durationMs := time.Since(start).Milliseconds()
	if durationMs < 0 {
		durationMs = 0
	}

	return map[string]interface{}{
		"status": status,
		"timing": map[string]interface{}{
			"durationMs": durationMs,
		},
		"aliases": map[string]interface{}{
			"path":               reloadedAliasPath,
			"loaded":             reloadedAliasPath != "",
			"countTotal":         reloadedResolver.aliases.Count(),
			"countByEntity":      reloadedResolver.aliases.CountByEntity(),
			"previousPath":       previousAliasPath,
			"previousCountTotal": previousAliasCount,
		},
		"resolver": map[string]interface{}{
			"tracksIndexed": len(reloadedResolver.tracks),
			"exactKeys":     len(reloadedResolver.exactIndex),
		},
		"note": "alias map and resolver indexes were reloaded in-process",
	}, nil
}

func toolCatalog() []toolDefinition {
	return []toolDefinition{
		{
			Name:        "music_resolve_track_identity",
			Description: "Resolve a track/scrobble to a canonical library track with confidence and evidence.",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"query"},
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":     "object",
						"required": []string{"artist", "track"},
						"properties": map[string]interface{}{
							"artist":      map[string]interface{}{"type": "string"},
							"track":       map[string]interface{}{"type": "string"},
							"album":       map[string]interface{}{"type": "string"},
							"durationSec": map[string]interface{}{"type": "integer", "minimum": 0},
							"year":        map[string]interface{}{"type": "integer", "minimum": 1000, "maximum": 2100},
							"timestampMs": map[string]interface{}{"type": "integer"},
						},
					},
					"options": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"strictness":      map[string]interface{}{"type": "string", "enum": []string{"high", "medium", "low"}, "default": "medium"},
							"maxCandidates":   map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 50, "default": 10},
							"includeEvidence": map[string]interface{}{"type": "boolean", "default": true},
						},
					},
				},
			},
		},
		{
			Name:        "music_audit_match_coverage",
			Description: "Analyze match quality between scrobbles and library tracks; return gaps, clusters, and suggested rule fixes.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"period": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"startDate": map[string]interface{}{"type": "string", "format": "date"},
							"endDate":   map[string]interface{}{"type": "string", "format": "date"},
						},
					},
					"groupBy":        map[string]interface{}{"type": "string", "enum": []string{"month", "year"}, "default": "month"},
					"minClusterSize": map[string]interface{}{"type": "integer", "minimum": 3, "default": 10},
					"source":         sourceFilterSchema(),
				},
			},
		},
		{
			Name:        "music_compare_eras",
			Description: "Compare two listening windows and quantify drift, overlap, and emerging/declining preferences.",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"eraA", "eraB"},
				"properties": map[string]interface{}{
					"eraA":   eraInputSchema("Era A"),
					"eraB":   eraInputSchema("Era B"),
					"topN":   map[string]interface{}{"type": "integer", "minimum": 5, "maximum": 100, "default": 25},
					"source": sourceFilterSchema(),
				},
			},
		},
		{
			Name:        "music_listening_summary",
			Description: "Return top artists, tracks, and genres for a given period.",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"startDate", "endDate"},
				"properties": map[string]interface{}{
					"startDate": map[string]interface{}{"type": "string", "format": "date"},
					"endDate":   map[string]interface{}{"type": "string", "format": "date"},
					"topN":      map[string]interface{}{"type": "integer", "minimum": 5, "maximum": 100, "default": 25},
					"source":    sourceFilterSchema(),
				},
			},
		},
		{
			Name:        "music_new_discoveries",
			Description: "Identify tracks and artists scrobbled for the first time in a given period.",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"startDate", "endDate"},
				"properties": map[string]interface{}{
					"startDate": map[string]interface{}{"type": "string", "format": "date"},
					"endDate":   map[string]interface{}{"type": "string", "format": "date"},
					"topN":      map[string]interface{}{"type": "integer", "minimum": 5, "maximum": 100, "default": 25},
					"source":    sourceFilterSchema(),
				},
			},
		},
		{
			Name:        "music_genre_profile",
			Description: "Analyze genre and tag-level distribution for a given period.",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"startDate", "endDate"},
				"properties": map[string]interface{}{
					"startDate": map[string]interface{}{"type": "string", "format": "date"},
					"endDate":   map[string]interface{}{"type": "string", "format": "date"},
					"topN":      map[string]interface{}{"type": "integer", "minimum": 5, "maximum": 100, "default": 25},
					"source":    sourceFilterSchema(),
				},
			},
		},
		{
			Name:        "music_listening_patterns",
			Description: "Analyze listening habits like session length, time of day, and bingeing vs. shuffling behavior.",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"startDate", "endDate"},
				"properties": map[string]interface{}{
					"startDate": map[string]interface{}{"type": "string", "format": "date"},
					"endDate":   map[string]interface{}{"type": "string", "format": "date"},
					"source":    sourceFilterSchema(),
				},
			},
		},
		{
			Name:        "music_streaks_and_bursts",
			Description: "Analyze consecutive listening streaks, burst sessions, peak days, and concentrated track binges.",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"startDate", "endDate"},
				"properties": map[string]interface{}{
					"startDate":         map[string]interface{}{"type": "string", "format": "date"},
					"endDate":           map[string]interface{}{"type": "string", "format": "date"},
					"label":             map[string]interface{}{"type": "string"},
					"timezone":          map[string]interface{}{"type": "string", "default": "UTC"},
					"topN":              map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 100, "default": 10},
					"sessionGapMinutes": map[string]interface{}{"type": "integer", "minimum": 5, "maximum": 180, "default": 30},
					"source":            sourceFilterSchema(),
				},
			},
		},
		{
			Name:        "music_year_story",
			Description: "Generate wrapped-ready annual story cards and narrative bullets from listening history.",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"year"},
				"properties": map[string]interface{}{
					"year":                  map[string]interface{}{"type": "integer", "minimum": 1900, "maximum": 2100},
					"topN":                  map[string]interface{}{"type": "integer", "minimum": 3, "maximum": 50, "default": 10},
					"timezone":              map[string]interface{}{"type": "string", "default": "UTC"},
					"sessionGapMinutes":     map[string]interface{}{"type": "integer", "minimum": 5, "maximum": 180, "default": 30},
					"includeDormantReturns": map[string]interface{}{"type": "boolean", "default": true},
					"source":                sourceFilterSchema(),
				},
			},
		},
		{
			Name:        "music_find_dormant_returns",
			Description: "Find tracks that were dormant for a long gap and then returned in a target listening window.",
			InputSchema: map[string]interface{}{
				"type":     "object",
				"required": []string{"returnPeriod"},
				"properties": map[string]interface{}{
					"returnPeriod": map[string]interface{}{
						"type":     "object",
						"required": []string{"startDate", "endDate"},
						"properties": map[string]interface{}{
							"startDate": map[string]interface{}{"type": "string", "format": "date"},
							"endDate":   map[string]interface{}{"type": "string", "format": "date"},
						},
					},
					"historyStartDate":  map[string]interface{}{"type": "string", "format": "date"},
					"minDormancyDays":   map[string]interface{}{"type": "integer", "minimum": 30, "default": 1825},
					"minPreReturnPlays": map[string]interface{}{"type": "integer", "minimum": 1, "default": 2},
					"minReturnPlays":    map[string]interface{}{"type": "integer", "minimum": 1, "default": 2},
					"topN":              map[string]interface{}{"type": "integer", "minimum": 1, "maximum": 200, "default": 25},
					"strictness":        map[string]interface{}{"type": "string", "enum": []string{"high", "medium", "low"}, "default": "medium"},
					"source":            sourceFilterSchema(),
				},
			},
		},
		{
			Name:        "music_reload_alias_map",
			Description: "Reload alias map and resolver indexes without restarting the MCP server.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"reason": map[string]interface{}{
						"type": "string",
					},
				},
			},
		},
	}
}

func eraInputSchema(defaultLabel string) map[string]interface{} {
	return map[string]interface{}{
		"type":     "object",
		"required": []string{"startDate", "endDate"},
		"properties": map[string]interface{}{
			"startDate": map[string]interface{}{"type": "string", "format": "date"},
			"endDate":   map[string]interface{}{"type": "string", "format": "date"},
			"label":     map[string]interface{}{"type": "string", "default": defaultLabel},
		},
	}
}

func sourceFilterSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":    "string",
		"enum":    []string{"all", "lastfm", "spotify"},
		"default": "all",
	}
}

func parseEra(value map[string]interface{}, fallbackLabel string) (time.Time, time.Time, string, error) {
	start, err := parseDate(asString(value["startDate"]))
	if err != nil {
		return time.Time{}, time.Time{}, "", fmt.Errorf("startDate: %w", err)
	}
	end, err := parseDate(asString(value["endDate"]))
	if err != nil {
		return time.Time{}, time.Time{}, "", fmt.Errorf("endDate: %w", err)
	}
	if end.Before(start) {
		return time.Time{}, time.Time{}, "", errors.New("endDate must be on or after startDate")
	}
	label := asString(value["label"])
	if label == "" {
		label = fallbackLabel
	}
	return start, end, label, nil
}

func parseDate(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, errors.New("date is required")
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, errors.New("expected YYYY-MM-DD")
	}
	return t, nil
}

func asMap(v interface{}) (map[string]interface{}, bool) {
	m, ok := v.(map[string]interface{})
	return m, ok
}

func asString(v interface{}) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func asInt(v interface{}) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int32:
		return int(n), true
	case int64:
		return int(n), true
	case float32:
		return int(n), true
	case float64:
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(n))
		if err != nil {
			return 0, false
		}
		return i, true
	default:
		return 0, false
	}
}

func asBool(v interface{}) (bool, bool) {
	switch b := v.(type) {
	case bool:
		return b, true
	case string:
		switch strings.ToLower(strings.TrimSpace(b)) {
		case "true", "1", "yes":
			return true, true
		case "false", "0", "no":
			return false, true
		default:
			return false, false
		}
	default:
		return false, false
	}
}

func readMessage(reader *bufio.Reader) ([]byte, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	first := strings.TrimRight(line, "\r\n")
	if first == "" {
		return nil, errors.New("empty frame prelude")
	}

	// Allow raw JSON messages without framed headers for local/manual testing.
	if strings.HasPrefix(first, "{") || strings.HasPrefix(first, "[") {
		return []byte(first), nil
	}

	headers := []string{first}
	for {
		h, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		trimmed := strings.TrimRight(h, "\r\n")
		if strings.TrimSpace(trimmed) == "" {
			break
		}
		headers = append(headers, trimmed)
	}

	length := -1
	for _, h := range headers {
		if strings.HasPrefix(strings.ToLower(h), "content-length:") {
			n, err := parseContentLength(h)
			if err != nil {
				return nil, err
			}
			length = n
			break
		}
	}
	if length <= 0 {
		return nil, errors.New("missing content-length header")
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func parseContentLength(line string) (int, error) {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return 0, errors.New("invalid content-length header")
	}
	n, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, fmt.Errorf("invalid content-length value: %w", err)
	}
	if n <= 0 {
		return 0, errors.New("content-length must be positive")
	}
	return n, nil
}

func resolveInitializeProtocolVersion(raw json.RawMessage) string {
	if len(raw) == 0 {
		return protocolVersion
	}

	var params initializeParams
	if err := json.Unmarshal(raw, &params); err != nil {
		debugMCP("initialize params parse error: %v", err)
		return protocolVersion
	}

	version := strings.TrimSpace(params.ProtocolVersion)
	if version == "" {
		return protocolVersion
	}
	return version
}

func rawMessageForLog(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "null"
	}
	return string(raw)
}

func writeResponse(writer *bufio.Writer, response rpcResponse) error {
	if response.JSONRPC == "" {
		response.JSONRPC = "2.0"
	}

	body, err := json.Marshal(response)
	if err != nil {
		return err
	}

	// Use newline-delimited JSON for stdio transport (Claude Desktop compatibility)
	if _, err := writer.Write(body); err != nil {
		return err
	}
	if _, err := writer.WriteString("\n"); err != nil {
		return err
	}
	return writer.Flush()
}
