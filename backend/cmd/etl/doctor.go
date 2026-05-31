package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"github.com/spf13/cobra"
)

type doctorResult struct {
	level  string
	check  string
	detail string
}

type etlPaths struct {
	Root        string
	BackendDir  string
	SchemaPath  string
	Database    string
	ItunesDir   string
	CompiledCSV string
	LastFMDir   string
	SpotifyDir  string
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Validate ETL path config and required inputs",
	Run: func(cmd *cobra.Command, args []string) {
		runDoctor()
	},
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}

func runDoctor() {
	paths := resolveETLPaths()
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

	fmt.Println("etl doctor")
	fmt.Printf("Root:      %s\n", paths.Root)
	fmt.Printf("Backend:   %s\n", paths.BackendDir)
	fmt.Printf("Schema:    %s\n", paths.SchemaPath)
	fmt.Printf("Database:  %s\n", paths.Database)
	fmt.Printf("iTunes:    %s\n", paths.ItunesDir)
	fmt.Printf("Compiled:  %s\n", paths.CompiledCSV)
	fmt.Printf("Last.fm:   %s\n", paths.LastFMDir)
	fmt.Printf("Spotify:   %s\n\n", paths.SpotifyDir)

	requireFile(record, "schema file", paths.SchemaPath)
	requireFile(record, "SQLite database", paths.Database)
	requireDir(record, "itunes input dir", paths.ItunesDir)
	requireFile(record, "compiled iTunes CSV", paths.CompiledCSV)
	requireDir(record, "lastfm input dir", paths.LastFMDir)
	requireDir(record, "spotify input dir", paths.SpotifyDir)

	if count, err := countMatchingFiles(paths.ItunesDir, isItunesExportFile); err != nil {
		record("WARN", "itunes export discovery", err.Error())
	} else if count == 0 {
		record("WARN", "itunes export discovery", "no export files found")
	} else {
		record("PASS", "itunes export discovery", fmt.Sprintf("%d files found", count))
	}

	if count, err := countGlob(filepath.Join(paths.SpotifyDir, "Streaming_History_Audio_*.json")); err != nil {
		record("WARN", "spotify history discovery", err.Error())
	} else if count == 0 {
		record("WARN", "spotify history discovery", "no audio history files found")
	} else {
		record("PASS", "spotify history discovery", fmt.Sprintf("%d files found", count))
	}

	lastfmStats := filepath.Join(paths.LastFMDir, fmt.Sprintf("lastfmstats-%s.json", lastFMUsername()))
	if _, err := os.Stat(lastfmStats); err != nil {
		record("WARN", "lastfm scrobble file", fmt.Sprintf("missing %s", lastfmStats))
	} else {
		record("PASS", "lastfm scrobble file", fmt.Sprintf("found %s", lastfmStats))
	}

	if strings.TrimSpace(os.Getenv("LASTFM_API_KEY")) == "" {
		record("WARN", "LASTFM_API_KEY", "not set (required for fetch-lastfm and fetch-images)")
	} else {
		record("PASS", "LASTFM_API_KEY", "set")
	}

	if err := validateSQLite(paths.Database); err != nil {
		record("FAIL", "SQLite schema", err.Error())
	} else {
		record("PASS", "SQLite schema", "required tables found")
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

func resolveETLPaths() etlPaths {
	root := findProjectRoot()
	backendDir := filepath.Join(root, "backend")

	return etlPaths{
		Root:        root,
		BackendDir:  backendDir,
		SchemaPath:  resolveFromRoot(root, envOrDefault("MP3_SCHEMA_PATH", filepath.Join("backend", "schema.sql"))),
		Database:    resolveFromRoot(root, envOrDefault("MP3_DATABASE_PATH", filepath.Join("data", "mp3_collection.db"))),
		ItunesDir:   resolveFromRoot(root, envOrDefault("MP3_ARCHIVE_DIR", filepath.Join("data", "inputs", "itunes"))),
		CompiledCSV: resolveFromRoot(root, envOrDefault("MP3_COMPILED_CSV", filepath.Join("data", "derived", "compiled", "compiled_itunes_library.csv"))),
		LastFMDir:   resolveFromRoot(root, envOrDefault("MP3_LASTFM_DIR", filepath.Join("data", "inputs", "lastfm"))),
		SpotifyDir:  resolveFromRoot(root, envOrDefault("MP3_SPOTIFY_DIR", filepath.Join("data", "inputs", "spotify"))),
	}
}

func findProjectRoot() string {
	if envRoot := strings.TrimSpace(os.Getenv("MP3_PROJECT_ROOT")); envRoot != "" {
		return filepath.Clean(envRoot)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}

	dir := filepath.Clean(cwd)
	for {
		if pathExists(filepath.Join(dir, "backend", "schema.sql")) && pathExists(filepath.Join(dir, "apps", "web", "package.json")) {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	if filepath.Base(cwd) == "backend" {
		return filepath.Dir(cwd)
	}
	return cwd
}

func envOrDefault(key, defaultValue string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return defaultValue
}

func resolveFromRoot(root, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(root, path))
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func requireFile(record func(string, string, string), check, path string) {
	info, err := os.Stat(path)
	if err != nil {
		record("FAIL", check, err.Error())
		return
	}
	if info.IsDir() {
		record("FAIL", check, "path exists but is a directory")
		return
	}
	record("PASS", check, "found")
}

func requireDir(record func(string, string, string), check, path string) {
	info, err := os.Stat(path)
	if err != nil {
		record("FAIL", check, err.Error())
		return
	}
	if !info.IsDir() {
		record("FAIL", check, "path exists but is not a directory")
		return
	}
	record("PASS", check, "found")
}

func countMatchingFiles(root string, matches func(string) bool) (int, error) {
	count := 0
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if matches(d.Name()) {
			count++
		}
		return nil
	})
	return count, err
}

func isItunesExportFile(name string) bool {
	return strings.HasSuffix(name, ".txt") || strings.HasPrefix(name, "Library.export")
}

func countGlob(pattern string) (int, error) {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return 0, err
	}
	return len(matches), nil
}

func lastFMUsername() string {
	if username := strings.TrimSpace(os.Getenv("LASTFM_USERNAME")); username != "" {
		return username
	}
	return "riebschlager"
}

func validateSQLite(dbPath string) error {
	database, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer database.Close()

	requiredTables := []string{
		"artists",
		"albums",
		"tracks",
		"listening_history",
		"transition_edges",
		"era_similarities",
	}

	for _, table := range requiredTables {
		var name string
		err := database.QueryRow("SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&name)
		if err != nil {
			return fmt.Errorf("missing table %q", table)
		}
	}

	return nil
}
