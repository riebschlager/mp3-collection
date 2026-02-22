package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

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

func (c *mcpProcessClient) callTool(name string, arguments map[string]interface{}) (map[string]interface{}, error) {
	resultRaw, err := c.call("tools/call", map[string]interface{}{
		"name":      name,
		"arguments": arguments,
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

func (c *mcpProcessClient) callYearStory(year, topN int, timezone, source, discoveryBaseline string) (map[string]interface{}, error) {
	return c.callTool("music_year_story", map[string]interface{}{
		"year":                  year,
		"topN":                  topN,
		"timezone":              timezone,
		"source":                source,
		"discoveryBaseline":     discoveryBaseline,
		"includeDormantReturns": true,
	})
}

func (c *mcpProcessClient) callMonthStory(year, month, topN int, timezone, source, discoveryBaseline string, includeDormantReturns bool) (map[string]interface{}, error) {
	return c.callTool("music_month_story", map[string]interface{}{
		"year":                  year,
		"month":                 month,
		"topN":                  topN,
		"timezone":              timezone,
		"source":                source,
		"discoveryBaseline":     discoveryBaseline,
		"includeDormantReturns": includeDormantReturns,
	})
}

func (c *mcpProcessClient) callBatchYearStory(years []int, topN int, timezone, source, discoveryBaseline string) (map[string]map[string]interface{}, map[string]interface{}, error) {
	toolResult, err := c.callTool("music_batch_year_story", map[string]interface{}{
		"years":                 years,
		"topN":                  topN,
		"timezone":              timezone,
		"source":                source,
		"discoveryBaseline":     discoveryBaseline,
		"includeDormantReturns": true,
	})
	if err != nil {
		return nil, nil, err
	}

	storiesByYearRaw, ok := toolResult["storiesByYear"].(map[string]interface{})
	if !ok || len(storiesByYearRaw) == 0 {
		return nil, nil, fmt.Errorf("batch tool response missing storiesByYear")
	}

	stories := make(map[string]map[string]interface{}, len(storiesByYearRaw))
	for key, raw := range storiesByYearRaw {
		story, ok := raw.(map[string]interface{})
		if !ok {
			return nil, nil, fmt.Errorf("batch tool response has invalid story payload for year %s", key)
		}
		stories[key] = story
	}

	summary, _ := toolResult["summary"].(map[string]interface{})
	return stories, summary, nil
}

func (c *mcpProcessClient) callTransitionGraph(startDate, endDate, source string, sessionGapMinutes, minTransitionCount, maxEdges int, includeSelfLoops bool) (map[string]interface{}, error) {
	args := map[string]interface{}{
		"startDate":          startDate,
		"endDate":            endDate,
		"scope":              "both",
		"sessionGapMinutes":  sessionGapMinutes,
		"minTransitionCount": minTransitionCount,
		"maxEdges":           maxEdges,
		"includeSelfLoops":   includeSelfLoops,
	}
	if strings.TrimSpace(source) != "" {
		args["source"] = source
	}
	return c.callTool("music_transition_graph", args)
}

func (c *mcpProcessClient) callCompareEras(
	aStartDate, aEndDate, aLabel string,
	bStartDate, bEndDate, bLabel string,
	source string,
	topN int,
) (map[string]interface{}, error) {
	args := map[string]interface{}{
		"eraA": map[string]interface{}{
			"startDate": aStartDate,
			"endDate":   aEndDate,
			"label":     aLabel,
		},
		"eraB": map[string]interface{}{
			"startDate": bStartDate,
			"endDate":   bEndDate,
			"label":     bLabel,
		},
		"topN": topN,
	}
	if strings.TrimSpace(source) != "" {
		args["source"] = source
	}
	return c.callTool("music_compare_eras", args)
}

func (c *mcpProcessClient) callStreaksAndBursts(eraLabel, startDate, endDate, source, timezone string, topN int) (map[string]interface{}, error) {
	args := map[string]interface{}{
		"eraLabel":  eraLabel,
		"startDate": startDate,
		"endDate":   endDate,
		"topN":      topN,
	}
	if strings.TrimSpace(source) != "" {
		args["source"] = source
	}
	if strings.TrimSpace(timezone) != "" {
		args["timezone"] = timezone
	}
	return c.callTool("music_streaks_and_bursts", args)
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
