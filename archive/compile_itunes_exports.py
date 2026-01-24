#!/usr/bin/env python3
"""
iTunes Export Compiler
Compiles 368 iTunes library export files into a single CSV
"""

import csv
import os
from pathlib import Path
from typing import List, Dict, Set
import sys

def find_all_export_files(base_dir: str) -> List[Path]:
    """Find all iTunes export files across all folders"""
    base_path = Path(base_dir)
    files = []

    # Files to exclude (script outputs)
    exclude_files = {'validation_report.txt', 'compiled_itunes_library.csv',
                     'compile_itunes_exports.py'}

    # Find .txt files
    txt_files = base_path.glob("**/*.txt")
    files.extend([f for f in txt_files if f.name not in exclude_files])

    # Find Library.export* files
    files.extend(base_path.glob("**/Library.export*"))

    return sorted(files)

def detect_header_pattern(first_line: str) -> tuple[bool, int]:
    """
    Detect if the first line contains a header and where data starts.
    Returns: (has_header, num_header_fields)
    """
    fields = first_line.split('\t')

    # Standard iTunes export header fields
    standard_headers = ['Name', 'Artist', 'Composer', 'Album', 'Genre', 'Size',
                       'Time', 'Track Number', 'Year', 'Date', 'Location']

    # Check if the first few fields look like headers
    header_matches = 0
    for i, field in enumerate(fields[:15]):  # Check first 15 fields
        if field in standard_headers:
            header_matches += 1

    # If we found several standard headers, this line has headers
    if header_matches >= 5:
        # Find where headers end by looking for a common header pattern
        # Headers typically include: Name, Artist, Album, ... Location
        # Then data starts (often with a volume ID at the very end)

        # Try to find "Location" field, which is typically the last header
        try:
            location_idx = fields.index('Location')
            return True, location_idx + 1
        except ValueError:
            # If Location not found, estimate based on header matches
            # Standard format has about 25-27 fields
            return True, min(27, len(fields) // 2)

    return False, 0

def parse_export_file(filepath: Path) -> tuple[List[str], List[Dict], List[str]]:
    """
    Parse a single iTunes export file.
    Handles three cases:
    1. Files with separate header row
    2. Files with header+data on first line
    3. Files with no header (just data)
    Returns: (headers, rows, errors)
    """
    errors = []
    rows = []
    headers = []

    # Standard iTunes export headers (for files without headers)
    standard_headers = [
        'Name', 'Artist', 'Composer', 'Album', 'Grouping', 'Genre',
        'Size', 'Time', 'Disc Number', 'Disc Count', 'Track Number', 'Track Count',
        'Year', 'Date Modified', 'Date', 'Date Added',
        'Bit Rate', 'Sample Rate', 'Volume Adjustment',
        'Kind', 'Equalizer', 'Comments',
        'Play Count', 'Last Played', 'My Rating',
        'Location'
    ]

    try:
        with open(filepath, 'r', encoding='utf-8', errors='replace') as f:
            content = f.read()

        # Split into lines
        lines = content.strip().split('\n')

        if not lines:
            errors.append(f"Empty file: {filepath.name}")
            return headers, rows, errors

        # Analyze first line
        first_line = lines[0]
        has_header, num_header_fields = detect_header_pattern(first_line)

        start_line = 0

        if has_header:
            first_fields = first_line.split('\t')

            # Check if header and data are on the same line
            if len(first_fields) > num_header_fields + 5:  # Has both header and data
                # Extract header
                headers = first_fields[:num_header_fields]

                # Extract data from remaining fields
                # Last field is often a volume ID, data is between headers and vol ID
                data_fields = first_fields[num_header_fields:]

                # Create first row
                row = {}
                for i, header in enumerate(headers):
                    if i < len(data_fields):
                        row[header] = data_fields[i]
                    else:
                        row[header] = ""

                row['_source_file'] = filepath.name
                row['_line_number'] = 1
                rows.append(row)

                start_line = 1  # Continue with line 2
            else:
                # Header is on its own line
                headers = first_fields[:num_header_fields]
                start_line = 1  # Continue with line 2
        else:
            # No header detected - use standard headers
            headers = standard_headers
            start_line = 0  # Process from line 1

        # Parse remaining data rows
        for line_num, line in enumerate(lines[start_line:], start=start_line + 1):
            if not line.strip():
                continue

            fields = line.split('\t')

            # Create a dict for this row
            row = {}
            for i, header in enumerate(headers):
                if i < len(fields):
                    row[header] = fields[i]
                else:
                    row[header] = ""

            # Add source file info
            row['_source_file'] = filepath.name
            row['_line_number'] = line_num

            rows.append(row)

    except Exception as e:
        errors.append(f"Error reading {filepath.name}: {str(e)}")

    return headers, rows, errors

def get_unified_headers(all_headers: List[Set[str]]) -> List[str]:
    """
    Create a unified header list from all files
    """
    # Get all unique headers
    all_fields = set()
    for headers in all_headers:
        all_fields.update(headers)

    # Define preferred order for common fields
    preferred_order = [
        'Name', 'Artist', 'Composer', 'Album', 'Grouping', 'Genre',
        'Size', 'Time', 'Disc Number', 'Disc Count', 'Track Number', 'Track Count',
        'Year', 'Date Modified', 'Date', 'Date Added',
        'Bit Rate', 'Sample Rate', 'Volume Adjustment',
        'Kind', 'Equalizer', 'Comments',
        'Play Count', 'Last Played', 'My Rating',
        'Location'
    ]

    # Start with preferred fields that exist
    unified = [field for field in preferred_order if field in all_fields]

    # Add any remaining fields
    remaining = sorted(all_fields - set(unified) - {'_source_file', '_line_number'})
    unified.extend(remaining)

    # Add metadata fields at the end
    unified.extend(['_source_file', '_line_number'])

    return unified

def main():
    base_dir = os.path.dirname(os.path.abspath(__file__))
    output_file = os.path.join(base_dir, 'compiled_itunes_library.csv')
    validation_report = os.path.join(base_dir, 'validation_report.txt')

    print("iTunes Export Compiler")
    print("=" * 60)

    # Find all files
    print(f"\nSearching for export files in: {base_dir}")
    export_files = find_all_export_files(base_dir)
    print(f"Found {len(export_files)} export files")

    # Parse all files
    all_rows = []
    all_headers_sets = []
    all_errors = []
    file_formats = []  # Track format detection for each file
    stats = {
        'files_processed': 0,
        'files_with_errors': 0,
        'total_rows': 0,
        'empty_files': 0,
        'header_plus_data': 0,
        'separate_header': 0,
        'no_header': 0,
        'duplicates_removed': 0,
        'unique_songs': 0
    }

    print("\nParsing files...")
    for i, filepath in enumerate(export_files, 1):
        if i % 50 == 0:
            print(f"  Processed {i}/{len(export_files)} files...")

        # Read first line to detect format
        try:
            with open(filepath, 'r', encoding='utf-8', errors='replace') as f:
                first_line = f.readline().strip()

            has_header, num_fields = detect_header_pattern(first_line)
            field_count = len(first_line.split('\t'))

            if has_header and field_count > num_fields + 5:
                format_type = "header+data"
                stats['header_plus_data'] += 1
            elif has_header:
                format_type = "separate_header"
                stats['separate_header'] += 1
            else:
                format_type = "no_header"
                stats['no_header'] += 1

            file_formats.append((filepath.name, format_type, field_count))
        except:
            format_type = "unknown"
            file_formats.append((filepath.name, format_type, 0))

        headers, rows, errors = parse_export_file(filepath)

        stats['files_processed'] += 1

        if errors:
            stats['files_with_errors'] += 1
            all_errors.extend(errors)

        if not rows:
            stats['empty_files'] += 1
        else:
            all_headers_sets.append(set(headers))
            all_rows.extend(rows)
            stats['total_rows'] += len(rows)

    print(f"  Processed {len(export_files)}/{len(export_files)} files")

    # Create unified headers
    unified_headers = get_unified_headers(all_headers_sets)

    # Remove exact duplicates (excluding metadata fields)
    print(f"\nChecking for duplicates...")
    original_count = len(all_rows)

    # Create a set to track unique rows (based on content, not metadata)
    seen = set()
    deduped_rows = []
    duplicates_found = []

    for row in all_rows:
        # Create a tuple of all values except metadata fields
        content_tuple = tuple(
            row.get(field, '') for field in unified_headers
            if field not in ['_source_file', '_line_number']
        )

        if content_tuple in seen:
            # This is a duplicate
            duplicates_found.append({
                'name': row.get('Name', 'Unknown'),
                'artist': row.get('Artist', 'Unknown'),
                'source': row.get('_source_file', 'Unknown')
            })
        else:
            seen.add(content_tuple)
            deduped_rows.append(row)

    stats['duplicates_removed'] = original_count - len(deduped_rows)
    stats['unique_songs'] = len(deduped_rows)

    print(f"  Found {stats['duplicates_removed']} duplicate rows")
    print(f"  Keeping {stats['unique_songs']} unique songs")

    # Write CSV
    print(f"\nWriting compiled CSV to: {output_file}")
    with open(output_file, 'w', newline='', encoding='utf-8') as f:
        writer = csv.DictWriter(f, fieldnames=unified_headers, extrasaction='ignore')
        writer.writeheader()
        for row in deduped_rows:
            writer.writerow(row)

    # Write validation report
    print(f"Writing validation report to: {validation_report}")
    with open(validation_report, 'w', encoding='utf-8') as f:
        f.write("iTunes Export Compilation - Validation Report\n")
        f.write("=" * 60 + "\n\n")

        f.write("STATISTICS\n")
        f.write("-" * 60 + "\n")
        f.write(f"Files found: {len(export_files)}\n")
        f.write(f"Files processed: {stats['files_processed']}\n")
        f.write(f"Files with errors: {stats['files_with_errors']}\n")
        f.write(f"Empty files: {stats['empty_files']}\n")
        f.write(f"Total rows parsed: {stats['total_rows']}\n")
        f.write(f"Duplicate rows removed: {stats['duplicates_removed']}\n")
        f.write(f"Unique songs in CSV: {stats['unique_songs']}\n")
        f.write(f"Unique fields found: {len(unified_headers)}\n")
        f.write("\n")

        f.write("FILE FORMAT DETECTION\n")
        f.write("-" * 60 + "\n")
        f.write(f"Files with header+data on first line: {stats['header_plus_data']}\n")
        f.write(f"Files with separate header row: {stats['separate_header']}\n")
        f.write(f"Files with no header (data only): {stats['no_header']}\n")
        f.write("\n")

        f.write("FIELD NAMES\n")
        f.write("-" * 60 + "\n")
        for field in unified_headers:
            f.write(f"  - {field}\n")
        f.write("\n")

        f.write("FILE FORMAT DETAILS\n")
        f.write("-" * 60 + "\n")
        for filename, format_type, field_count in file_formats:
            f.write(f"  {filename:30s} | {format_type:15s} | {field_count:3d} fields\n")
        f.write("\n")

        if all_errors:
            f.write("ERRORS\n")
            f.write("-" * 60 + "\n")
            for error in all_errors:
                f.write(f"  {error}\n")
        else:
            f.write("No errors encountered!\n")

    # Print summary
    print("\n" + "=" * 60)
    print("COMPILATION COMPLETE!")
    print("=" * 60)
    print(f"Total rows parsed: {stats['total_rows']:,}")
    print(f"Duplicates removed: {stats['duplicates_removed']:,}")
    print(f"Unique songs in CSV: {stats['unique_songs']:,}")
    print(f"Files processed: {stats['files_processed']}")
    print(f"Empty files skipped: {stats['empty_files']}")
    print(f"Errors encountered: {len(all_errors)}")
    print(f"\nFile formats detected:")
    print(f"  Header+data on first line: {stats['header_plus_data']}")
    print(f"  Separate header row: {stats['separate_header']}")
    print(f"  No header (data only): {stats['no_header']}")
    print(f"\nOutput file: {output_file}")
    print(f"Validation report: {validation_report}")

    if stats['unique_songs'] > 0:
        print("\n✓ Success! Your iTunes library has been compiled.")
        return 0
    else:
        print("\n✗ No data was compiled. Check the validation report.")
        return 1

if __name__ == "__main__":
    sys.exit(main())
