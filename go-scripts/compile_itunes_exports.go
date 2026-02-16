package main

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

var compilerExcludeFiles = map[string]struct{}{
	"validation_report.txt":       {},
	"compiled_itunes_library.csv": {},
}

var compilerHeaderProbeFields = []string{
	"Name", "Artist", "Composer", "Album", "Genre", "Size",
	"Time", "Track Number", "Year", "Date", "Location",
}

var compilerStandardHeaders = []string{
	"Name", "Artist", "Composer", "Album", "Grouping", "Genre",
	"Size", "Time", "Disc Number", "Disc Count", "Track Number", "Track Count",
	"Year", "Date Modified", "Date", "Date Added",
	"Bit Rate", "Sample Rate", "Volume Adjustment",
	"Kind", "Equalizer", "Comments",
	"Play Count", "Last Played", "My Rating",
	"Location",
}

type compileStats struct {
	FilesProcessed  int
	FilesWithErrors int
	TotalRows       int
	EmptyFiles      int
	HeaderPlusData  int
	SeparateHeader  int
	NoHeader        int
	Duplicates      int
	UniqueSongs     int
}

type formatDetail struct {
	Filename   string
	FormatType string
	FieldCount int
}

func runCompileITunesExports() {
	baseDir := Paths.ArchiveDir
	if len(os.Args) >= 3 {
		baseDir = strings.TrimSpace(os.Args[2])
	}

	outputFile := filepath.Join(baseDir, "compiled_itunes_library.csv")
	validationReport := filepath.Join(baseDir, "validation_report.txt")

	fmt.Println("iTunes Export Compiler")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("\nSearching for export files in: %s\n", baseDir)

	exportFiles, err := findAllExportFiles(baseDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error discovering export files: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Found %d export files\n", len(exportFiles))

	allRows := make([]map[string]string, 0, 1000)
	allHeaderSets := make([]map[string]struct{}, 0, len(exportFiles))
	allErrors := make([]string, 0)
	fileFormats := make([]formatDetail, 0, len(exportFiles))
	stats := compileStats{}

	fmt.Println("\nParsing files...")
	for i, exportPath := range exportFiles {
		if (i+1)%50 == 0 {
			fmt.Printf("  Processed %d/%d files...\n", i+1, len(exportFiles))
		}

		formatType, fieldCount := detectFileFormat(exportPath)
		switch formatType {
		case "header+data":
			stats.HeaderPlusData++
		case "separate_header":
			stats.SeparateHeader++
		case "no_header":
			stats.NoHeader++
		}
		fileFormats = append(fileFormats, formatDetail{
			Filename:   filepath.Base(exportPath),
			FormatType: formatType,
			FieldCount: fieldCount,
		})

		headers, rows, parseErrors := parseExportFile(exportPath)
		stats.FilesProcessed++

		if len(parseErrors) > 0 {
			stats.FilesWithErrors++
			allErrors = append(allErrors, parseErrors...)
		}

		if len(rows) == 0 {
			stats.EmptyFiles++
			continue
		}

		headerSet := make(map[string]struct{}, len(headers))
		for _, h := range headers {
			headerSet[h] = struct{}{}
		}
		allHeaderSets = append(allHeaderSets, headerSet)
		allRows = append(allRows, rows...)
		stats.TotalRows += len(rows)
	}

	fmt.Printf("  Processed %d/%d files\n", len(exportFiles), len(exportFiles))

	unifiedHeaders := getUnifiedHeaders(allHeaderSets)

	fmt.Println("\nChecking for duplicates...")
	nonMetaHeaders := make([]string, 0, len(unifiedHeaders))
	for _, h := range unifiedHeaders {
		if h != "_source_file" && h != "_line_number" {
			nonMetaHeaders = append(nonMetaHeaders, h)
		}
	}

	seen := make(map[string]struct{}, len(allRows))
	dedupedRows := make([]map[string]string, 0, len(allRows))
	for _, row := range allRows {
		key := rowContentKey(row, nonMetaHeaders)
		if _, exists := seen[key]; exists {
			stats.Duplicates++
			continue
		}
		seen[key] = struct{}{}
		dedupedRows = append(dedupedRows, row)
	}
	stats.UniqueSongs = len(dedupedRows)

	fmt.Printf("  Found %d duplicate rows\n", stats.Duplicates)
	fmt.Printf("  Keeping %d unique songs\n", stats.UniqueSongs)

	fmt.Printf("\nWriting compiled CSV to: %s\n", outputFile)
	if err := writeCompiledCSV(outputFile, unifiedHeaders, dedupedRows); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing compiled CSV: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Writing validation report to: %s\n", validationReport)
	if err := writeValidationReport(validationReport, exportFiles, stats, unifiedHeaders, fileFormats, allErrors); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing validation report: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("COMPILATION COMPLETE!")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Total rows parsed: %s\n", formatWithCommas(stats.TotalRows))
	fmt.Printf("Duplicates removed: %s\n", formatWithCommas(stats.Duplicates))
	fmt.Printf("Unique songs in CSV: %s\n", formatWithCommas(stats.UniqueSongs))
	fmt.Printf("Files processed: %d\n", stats.FilesProcessed)
	fmt.Printf("Empty files skipped: %d\n", stats.EmptyFiles)
	fmt.Printf("Errors encountered: %d\n", len(allErrors))
	fmt.Println("\nFile formats detected:")
	fmt.Printf("  Header+data on first line: %d\n", stats.HeaderPlusData)
	fmt.Printf("  Separate header row: %d\n", stats.SeparateHeader)
	fmt.Printf("  No header (data only): %d\n", stats.NoHeader)
	fmt.Printf("\nOutput file: %s\n", outputFile)
	fmt.Printf("Validation report: %s\n", validationReport)

	if stats.UniqueSongs > 0 {
		fmt.Println("\n\u2713 Success! Your iTunes library has been compiled.")
		return
	}

	fmt.Println("\n\u2717 No data was compiled. Check the validation report.")
	os.Exit(1)
}

func findAllExportFiles(baseDir string) ([]string, error) {
	files := make([]string, 0, 512)

	err := filepath.WalkDir(baseDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}

		name := d.Name()
		if _, excluded := compilerExcludeFiles[name]; excluded {
			return nil
		}

		if strings.HasSuffix(name, ".txt") {
			files = append(files, path)
		}
		if strings.HasPrefix(name, "Library.export") {
			files = append(files, path)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(files)
	return files, nil
}

func detectHeaderPattern(firstLine string) (bool, int) {
	fields := strings.Split(firstLine, "\t")

	probeSet := make(map[string]struct{}, len(compilerHeaderProbeFields))
	for _, h := range compilerHeaderProbeFields {
		probeSet[h] = struct{}{}
	}

	headerMatches := 0
	maxProbe := len(fields)
	if maxProbe > 15 {
		maxProbe = 15
	}
	for i := 0; i < maxProbe; i++ {
		if _, ok := probeSet[fields[i]]; ok {
			headerMatches++
		}
	}

	if headerMatches >= 5 {
		for i, field := range fields {
			if field == "Location" {
				return true, i + 1
			}
		}
		half := len(fields) / 2
		if half > 27 {
			half = 27
		}
		return true, half
	}

	return false, 0
}

func parseExportFile(path string) ([]string, []map[string]string, []string) {
	errorsOut := make([]string, 0)
	rows := make([]map[string]string, 0)
	headers := make([]string, 0)

	raw, err := os.ReadFile(path)
	if err != nil {
		errorsOut = append(errorsOut, fmt.Sprintf("Error reading %s: %v", filepath.Base(path), err))
		return headers, rows, errorsOut
	}

	content := normalizeNewlines(string(raw))
	lines := strings.Split(strings.TrimSpace(content), "\n")
	if len(lines) == 0 {
		errorsOut = append(errorsOut, fmt.Sprintf("Empty file: %s", filepath.Base(path)))
		return headers, rows, errorsOut
	}

	firstLine := lines[0]
	hasHeader, numHeaderFields := detectHeaderPattern(firstLine)
	startLine := 0

	if hasHeader {
		firstFields := strings.Split(firstLine, "\t")
		if len(firstFields) > numHeaderFields+5 {
			headers = append(headers, firstFields[:numHeaderFields]...)

			dataFields := firstFields[numHeaderFields:]
			row := make(map[string]string, len(headers)+2)
			for i, header := range headers {
				if i < len(dataFields) {
					row[header] = dataFields[i]
				} else {
					row[header] = ""
				}
			}
			row["_source_file"] = filepath.Base(path)
			row["_line_number"] = "1"
			rows = append(rows, row)
			startLine = 1
		} else {
			headers = append(headers, firstFields[:numHeaderFields]...)
			startLine = 1
		}
	} else {
		headers = append(headers, compilerStandardHeaders...)
		startLine = 0
	}

	for i, line := range lines[startLine:] {
		lineNum := startLine + i + 1
		if strings.TrimSpace(line) == "" {
			continue
		}

		fields := strings.Split(line, "\t")
		row := make(map[string]string, len(headers)+2)
		for i, header := range headers {
			if i < len(fields) {
				row[header] = fields[i]
			} else {
				row[header] = ""
			}
		}
		row["_source_file"] = filepath.Base(path)
		row["_line_number"] = strconv.Itoa(lineNum)
		rows = append(rows, row)
	}

	return headers, rows, errorsOut
}

func getUnifiedHeaders(allHeaderSets []map[string]struct{}) []string {
	allFields := make(map[string]struct{})
	for _, set := range allHeaderSets {
		for h := range set {
			allFields[h] = struct{}{}
		}
	}

	unified := make([]string, 0, len(allFields)+2)
	for _, field := range compilerStandardHeaders {
		if _, ok := allFields[field]; ok {
			unified = append(unified, field)
		}
	}

	remaining := make([]string, 0)
	for field := range allFields {
		if field == "_source_file" || field == "_line_number" {
			continue
		}
		found := false
		for _, existing := range unified {
			if existing == field {
				found = true
				break
			}
		}
		if !found {
			remaining = append(remaining, field)
		}
	}
	sort.Strings(remaining)
	unified = append(unified, remaining...)
	unified = append(unified, "_source_file", "_line_number")

	return unified
}

func detectFileFormat(path string) (string, int) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "unknown", 0
	}

	content := normalizeNewlines(string(raw))
	firstLine := content
	if idx := strings.IndexByte(content, '\n'); idx >= 0 {
		firstLine = content[:idx]
	}
	firstLine = strings.TrimSpace(firstLine)

	hasHeader, numFields := detectHeaderPattern(firstLine)
	fieldCount := len(strings.Split(firstLine, "\t"))

	if hasHeader && fieldCount > numFields+5 {
		return "header+data", fieldCount
	}
	if hasHeader {
		return "separate_header", fieldCount
	}
	return "no_header", fieldCount
}

func rowContentKey(row map[string]string, nonMetaHeaders []string) string {
	var b strings.Builder
	for _, h := range nonMetaHeaders {
		v := row[h]
		b.WriteString(strconv.Itoa(len(v)))
		b.WriteByte(':')
		b.WriteString(v)
		b.WriteByte('\x1f')
	}
	return b.String()
}

func writeCompiledCSV(path string, headers []string, rows []map[string]string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	writer := csv.NewWriter(f)
	writer.UseCRLF = true

	if err := writer.Write(headers); err != nil {
		return err
	}

	record := make([]string, len(headers))
	for _, row := range rows {
		for i, h := range headers {
			record[i] = row[h]
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}

	writer.Flush()
	return writer.Error()
}

func writeValidationReport(
	reportPath string,
	exportFiles []string,
	stats compileStats,
	headers []string,
	formats []formatDetail,
	allErrors []string,
) error {
	f, err := os.Create(reportPath)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	defer w.Flush()

	fmt.Fprintln(w, "iTunes Export Compilation - Validation Report")
	fmt.Fprintln(w, strings.Repeat("=", 60))
	fmt.Fprintln(w)

	fmt.Fprintln(w, "STATISTICS")
	fmt.Fprintln(w, strings.Repeat("-", 60))
	fmt.Fprintf(w, "Files found: %d\n", len(exportFiles))
	fmt.Fprintf(w, "Files processed: %d\n", stats.FilesProcessed)
	fmt.Fprintf(w, "Files with errors: %d\n", stats.FilesWithErrors)
	fmt.Fprintf(w, "Empty files: %d\n", stats.EmptyFiles)
	fmt.Fprintf(w, "Total rows parsed: %d\n", stats.TotalRows)
	fmt.Fprintf(w, "Duplicate rows removed: %d\n", stats.Duplicates)
	fmt.Fprintf(w, "Unique songs in CSV: %d\n", stats.UniqueSongs)
	fmt.Fprintf(w, "Unique fields found: %d\n", len(headers))
	fmt.Fprintln(w)

	fmt.Fprintln(w, "FILE FORMAT DETECTION")
	fmt.Fprintln(w, strings.Repeat("-", 60))
	fmt.Fprintf(w, "Files with header+data on first line: %d\n", stats.HeaderPlusData)
	fmt.Fprintf(w, "Files with separate header row: %d\n", stats.SeparateHeader)
	fmt.Fprintf(w, "Files with no header (data only): %d\n", stats.NoHeader)
	fmt.Fprintln(w)

	fmt.Fprintln(w, "FIELD NAMES")
	fmt.Fprintln(w, strings.Repeat("-", 60))
	for _, field := range headers {
		fmt.Fprintf(w, "  - %s\n", field)
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "FILE FORMAT DETAILS")
	fmt.Fprintln(w, strings.Repeat("-", 60))
	for _, fd := range formats {
		fmt.Fprintf(w, "  %-30s | %-15s | %3d fields\n", fd.Filename, fd.FormatType, fd.FieldCount)
	}
	fmt.Fprintln(w)

	if len(allErrors) > 0 {
		fmt.Fprintln(w, "ERRORS")
		fmt.Fprintln(w, strings.Repeat("-", 60))
		for _, e := range allErrors {
			fmt.Fprintf(w, "  %s\n", e)
		}
	} else {
		fmt.Fprintln(w, "No errors encountered!")
	}

	return nil
}

func normalizeNewlines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s
}

func formatWithCommas(n int) string {
	s := strconv.Itoa(n)
	if n < 1000 {
		return s
	}

	var b strings.Builder
	prefix := len(s) % 3
	if prefix == 0 {
		prefix = 3
	}
	b.WriteString(s[:prefix])
	for i := prefix; i < len(s); i += 3 {
		b.WriteByte(',')
		b.WriteString(s[i : i+3])
	}
	return b.String()
}
