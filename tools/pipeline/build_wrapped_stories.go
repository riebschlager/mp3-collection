package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultWrappedTopN     = 10
	defaultWrappedTimezone = "UTC"
	defaultWrappedSource   = "all"
)

type wrappedStoriesData struct {
	GeneratedAt string                            `json:"generatedAt"`
	Timezone    string                            `json:"timezone"`
	Source      string                            `json:"source"`
	TopN        int                               `json:"topN"`
	Years       []int                             `json:"years"`
	Stories     map[string]map[string]interface{} `json:"stories"`
}

type mcpRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *mcpRPCError    `json:"error,omitempty"`
	Method  string          `json:"method,omitempty"`
}

type mcpRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpToolCallResult struct {
	IsError           bool                   `json:"isError,omitempty"`
	Content           []mcpTextContent       `json:"content,omitempty"`
	StructuredContent map[string]interface{} `json:"structuredContent,omitempty"`
}

type mcpTextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type mcpProcessClient struct {
	cmd       *exec.Cmd
	stdinPipe io.WriteCloser
	stdin     *bufio.Writer
	stdout    *bufio.Reader
	nextID    int
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
	fmt.Printf("Timezone=%s Source=%s TopN=%d\n\n", timezone, source, topN)

	client, err := startMCPProcessClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error starting MCP client: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	stories := make(map[string]map[string]interface{}, len(years))
	for idx, year := range years {
		fmt.Printf("  [%d/%d] %d\n", idx+1, len(years), year)
		story, err := client.callYearStory(year, topN, timezone, source)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error generating story for %d: %v\n", year, err)
			os.Exit(1)
		}
		stories[strconv.Itoa(year)] = story
	}

	payload := wrappedStoriesData{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Timezone:    timezone,
		Source:      source,
		TopN:        topN,
		Years:       years,
		Stories:     stories,
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

func startMCPProcessClient() (*mcpProcessClient, error) {
	launcher := filepath.Join(Paths.Root, "apps", "mcp-server", "run-mcp.sh")
	if _, err := os.Stat(launcher); err != nil {
		return nil, fmt.Errorf("missing MCP launcher %s: %w", launcher, err)
	}

	cmd := exec.Command(launcher)
	cmd.Dir = Paths.Root
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"MP3_COLLECTION_ROOT="+Paths.Root,
		"MP3_WEB_DATA_DIR="+Paths.WebDataDir,
		"MP3_DATA_DIR="+Paths.DataDir,
	)

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start mcp server: %w", err)
	}

	client := &mcpProcessClient{
		cmd:       cmd,
		stdinPipe: stdinPipe,
		stdin:     bufio.NewWriter(stdinPipe),
		stdout:    bufio.NewReader(stdoutPipe),
		nextID:    1,
	}

	initParams := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "mp3-scripts",
			"version": "1.0.0",
		},
	}
	if _, err := client.call("initialize", initParams); err != nil {
		client.Close()
		return nil, fmt.Errorf("initialize: %w", err)
	}
	_ = client.notify("notifications/initialized", map[string]interface{}{})

	return client, nil
}

func (c *mcpProcessClient) Close() {
	if c == nil || c.cmd == nil {
		return
	}
	if c.stdin != nil {
		_ = c.stdin.Flush()
	}
	if c.stdinPipe != nil {
		_ = c.stdinPipe.Close()
	}

	done := make(chan error, 1)
	go func() {
		done <- c.cmd.Wait()
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		if c.cmd.Process != nil {
			_ = c.cmd.Process.Kill()
		}
		<-done
	}
}

func (c *mcpProcessClient) callYearStory(year, topN int, timezone, source string) (map[string]interface{}, error) {
	resultRaw, err := c.call("tools/call", map[string]interface{}{
		"name": "music_year_story",
		"arguments": map[string]interface{}{
			"year":                  year,
			"topN":                  topN,
			"timezone":              timezone,
			"source":                source,
			"includeDormantReturns": true,
		},
	})
	if err != nil {
		return nil, err
	}

	var toolResult mcpToolCallResult
	if err := json.Unmarshal(resultRaw, &toolResult); err != nil {
		return nil, fmt.Errorf("decode tool result: %w", err)
	}
	if toolResult.IsError {
		msg := "tool returned error"
		if len(toolResult.Content) > 0 && strings.TrimSpace(toolResult.Content[0].Text) != "" {
			msg = toolResult.Content[0].Text
		}
		return nil, fmt.Errorf(msg)
	}
	if toolResult.StructuredContent == nil {
		return nil, fmt.Errorf("tool response missing structuredContent")
	}

	return toolResult.StructuredContent, nil
}

func (c *mcpProcessClient) notify(method string, params interface{}) error {
	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
	}
	if params != nil {
		req["params"] = params
	}
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	if _, err := c.stdin.Write(body); err != nil {
		return err
	}
	if _, err := c.stdin.WriteString("\n"); err != nil {
		return err
	}
	return c.stdin.Flush()
}

func (c *mcpProcessClient) call(method string, params interface{}) (json.RawMessage, error) {
	requestID := c.nextID
	c.nextID++

	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      requestID,
		"method":  method,
	}
	if params != nil {
		req["params"] = params
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if _, err := c.stdin.Write(body); err != nil {
		return nil, err
	}
	if _, err := c.stdin.WriteString("\n"); err != nil {
		return nil, err
	}
	if err := c.stdin.Flush(); err != nil {
		return nil, err
	}

	for {
		line, err := c.stdout.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var resp mcpRPCResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			return nil, fmt.Errorf("parse server response: %w", err)
		}
		if resp.Method != "" {
			continue
		}
		if string(resp.ID) != strconv.Itoa(requestID) {
			continue
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("rpc error %d: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	}
}
