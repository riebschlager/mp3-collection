package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// RawTrack represents a row in the iTunes export CSV
type RawTrack struct {
	Name             string
	Artist           string
	Album            string
	Composer         string
	Genre            string
	Year             string
	Size             string
	Time             string
	BitRate          string
	SampleRate       string
	TrackNumber      string
	TrackCount       string
	DiscNumber       string
	DiscCount        string
	MyRating         string
	DateAdded        string
	DateModified     string
	LastPlayed       string
	Location         string
	Kind             string
	Grouping         string
	Comments         string
	VolumeAdjustment string
	Equalizer        string
}

// Helper to map CSV header to struct fields could be complex, 
// but since we know the header format, we can just read into a map or order.
// However, CSV DictReader in Python handles headers automatically.
// In Go, we'll read the header row first and map indices.

func ReadCSV(path string) ([]map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	
	// Read header
	header, err := r.Read()
	if err != nil {
		return nil, err
	}

	var rows []map[string]string

	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		row := make(map[string]string)
		for i, val := range record {
			if i < len(header) {
				row[header[i]] = val
			}
		}
		rows = append(rows, row)	}
	return rows, nil
}

// IsValidName checks if a name is valid (not just question marks or empty)
func IsValidName(name string) bool {
	if name == "" {
		return false
	}
	cleanName := strings.TrimSpace(name)
	if cleanName == "" || cleanName == "?" {
		return false
	}
	
	allQ := true
	for _, c := range cleanName {
		if c != '?' {
			allQ = false
			break
		}
	}
	return !allQ
}

// SanitizeArtistName removes quotes and handles trailing articles
func SanitizeArtistName(name string) string {
	if name == "" {
		return ""
	}

	name = strings.TrimSpace(name)
	name = trimQuotes(name)
	name = strings.TrimSpace(name)

	articles := []string{", The", ", A", ", An", ", Le", ", La", ", Los", ", Las", ", El"}
	for _, article := range articles {
		if strings.HasSuffix(strings.ToLower(name), strings.ToLower(article)) {
			articleText := article[2:] // Remove ", "
			nameWithoutArticle := name[:len(name)-len(article)]
			return fmt.Sprintf("%s %s", articleText, nameWithoutArticle)
		}
	}
	return name
}

// SanitizeAlbumName removes quotes
func SanitizeAlbumName(name string) string {
	if name == "" {
		return ""
	}
	return trimQuotes(strings.TrimSpace(name))
}

func trimQuotes(s string) string {
	// Remove leading/trailing quotes repeatedly
	for {
		changed := false
		if strings.HasPrefix(s, "\"\"\"") {
			s = s[3:]
			changed = true
		} else if strings.HasPrefix(s, "\"") {
			s = s[1:]
			changed = true
		}
		
		if strings.HasSuffix(s, "\"\"\"") {
			s = s[:len(s)-3]
			changed = true
		} else if strings.HasSuffix(s, "\"") {
			s = s[:len(s)-1]
			changed = true
		}
		
		if !changed {
			break
		}
	}
	return s
}

// Slugify converts text to URL-friendly slug
func Slugify(text string) string {
	if text == "" {
		return "unknown"
	}
	text = strings.ToLower(text)
	
	// Remove non-word characters (keep alphanumeric, spaces, hyphens)
	reg := regexp.MustCompile(`[^\w\s-]`)
	text = reg.ReplaceAllString(text, "")
	
	// Replace spaces and multiple hyphens with single hyphen
	regSpace := regexp.MustCompile(`[-\s]+`)
	text = regSpace.ReplaceAllString(text, "-")
	
	text = strings.Trim(text, "-")
	if text == "" {
		return "unknown"
	}
	return text
}

// SanitizeGenre cleans genre strings
func SanitizeGenre(genre string) string {
	if genre == "" {
		return ""
	}
	g := trimQuotes(strings.TrimSpace(genre))
	
	// Check if there is at least one letter
	hasLetter, _ := regexp.MatchString(`[A-Za-z]`, g)
	if hasLetter {
		return g
	}
	return ""
}

// SanitizeYear validates the year
func SanitizeYear(val string) int {
	y := SafeInt(val)
	if y <= 0 {
		return 0
	}
	currentYear := time.Now().Year()
	if y >= 1000 && y <= currentYear {
		return y
	}
	return 0
}

func SafeInt(val string) int {
	if val == "" {
		return 0
	}
	// Handle float strings like "2020.0" if necessary, though CSV usually has strings.
	// Python's int(float(value)) handles "2020.0" -> 2020.
	f, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return 0
	}
	return int(f)
}

func SafeStr(val string) string {
	if strings.TrimSpace(val) == "" {
		return ""
	}
	return strings.TrimSpace(val)
}

func FormatDuration(seconds int) string {
	if seconds == 0 {
		return "0:00"
	}
	minutes := seconds / 60
	secs := seconds % 60
	return fmt.Sprintf("%d:%02d", minutes, secs)
}
