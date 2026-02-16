package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type doctorResult struct {
	level  string
	check  string
	detail string
}

func runDoctor() {
	results := make([]doctorResult, 0, 16)
	failures := 0
	warnings := 0

	record := func(level, check, detail string) {
		results = append(results, doctorResult{
			level:  level,
			check:  check,
			detail: detail,
		})
		switch level {
		case "FAIL":
			failures++
		case "WARN":
			warnings++
		}
	}

	fmt.Println("mp3-scripts doctor")
	fmt.Printf("Root:      %s\n", Paths.Root)
	fmt.Printf("Inputs:    %s\n", Paths.ArchiveDir)
	fmt.Printf("Compiled:  %s\n", Paths.CompiledDir)
	fmt.Printf("Core data: %s\n", Paths.DataDir)
	fmt.Printf("Web data:  %s\n", Paths.WebDataDir)
	fmt.Printf("Last.fm:   %s\n", Paths.LastFMDir)
	fmt.Printf("Spotify:   %s\n\n", Paths.SpotifyDir)

	if err := os.MkdirAll(Paths.DataDir, 0755); err != nil {
		record("FAIL", "data output dir", err.Error())
	} else {
		record("PASS", "data output dir", "writable")
	}

	if err := os.MkdirAll(Paths.CompiledDir, 0755); err != nil {
		record("FAIL", "compiled output dir", err.Error())
	} else {
		record("PASS", "compiled output dir", "writable")
	}

	if err := os.MkdirAll(Paths.WebDataDir, 0755); err != nil {
		record("FAIL", "web-data output dir", err.Error())
	} else {
		record("PASS", "web-data output dir", "writable")
	}

	if info, err := os.Stat(Paths.ArchiveDir); err != nil || !info.IsDir() {
		if err == nil {
			record("FAIL", "itunes input dir", "path exists but is not a directory")
		} else {
			record("FAIL", "itunes input dir", err.Error())
		}
	} else {
		record("PASS", "itunes input dir", "found")
	}

	exportFiles, err := findAllExportFiles(Paths.ArchiveDir)
	if err != nil {
		record("WARN", "itunes export discovery", err.Error())
	} else if len(exportFiles) == 0 {
		record("WARN", "itunes export discovery", "no export files found")
	} else {
		record("PASS", "itunes export discovery", fmt.Sprintf("%d files found", len(exportFiles)))
	}

	if _, err := os.Stat(CompiledPath("compiled_itunes_library.csv")); err != nil {
		record("WARN", "compiled iTunes CSV", "not found (run compile-itunes-exports)")
	} else {
		record("PASS", "compiled iTunes CSV", "found")
	}

	spotifyFiles, err := filepath.Glob(SpotifyPath("Streaming_History_Audio_*.json"))
	if err != nil {
		record("WARN", "spotify history discovery", err.Error())
	} else if len(spotifyFiles) == 0 {
		record("WARN", "spotify history discovery", "no audio history files found")
	} else {
		record("PASS", "spotify history discovery", fmt.Sprintf("%d files found", len(spotifyFiles)))
	}

	lastfmPath := LastFMStatsPath("")
	if _, err := os.Stat(lastfmPath); err != nil {
		record("WARN", "lastfm scrobble file", fmt.Sprintf("missing %s", lastfmPath))
	} else {
		record("PASS", "lastfm scrobble file", fmt.Sprintf("found %s", lastfmPath))
	}

	if strings.TrimSpace(os.Getenv("LASTFM_API_KEY")) == "" {
		record("WARN", "LASTFM_API_KEY", "not set (required for fetch-lastfm and fetch-images)")
	} else {
		record("PASS", "LASTFM_API_KEY", "set")
	}

	fmt.Println("Checks:")
	for _, result := range results {
		fmt.Printf("  [%s] %-26s %s\n", result.level, result.check, result.detail)
	}

	fmt.Printf("\nSummary: %d fail, %d warn, %d pass\n", failures, warnings, len(results)-failures-warnings)
	if failures > 0 {
		os.Exit(1)
	}
}
