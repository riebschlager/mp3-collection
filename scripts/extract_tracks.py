#!/usr/bin/env python3
"""
Extract a list of tracks from the compiled iTunes library CSV.
Outputs tracks with their associated artists and albums as JSON.
"""

import csv
import json
from pathlib import Path


def is_valid_name(name):
    """Check if a name is valid (not just question marks)"""
    if not name:
        return False
    clean_name = str(name).strip()
    # Check if name is only question marks or empty
    return clean_name and clean_name != '?' and not all(c == '?' for c in clean_name)


def sanitize_artist_name(name):
    """Remove leading/trailing quotation marks and move trailing articles to the beginning"""
    if not name:
        return name
    
    # Remove leading and trailing triple quotes (""") and regular quotes (")
    name = str(name).strip()
    while name.startswith('"""') or name.startswith('"'):
        if name.startswith('"""'):
            name = name[3:]
        else:
            name = name[1:]
    while name.endswith('"""') or name.endswith('"'):
        if name.endswith('"""'):
            name = name[:-3]
        else:
            name = name[:-1]
    name = name.strip()
    
    # Move trailing articles to the beginning
    # Check for patterns like ", The", ", A", ", An"
    articles = [', The', ', A', ', An', ', Le', ', La', ', Los', ', Las', ', El']
    for article in articles:
        if name.lower().endswith(article.lower()):
            # Extract the article (without the comma and space)
            article_text = article[2:]  # Remove ", " prefix
            # Remove the article from the end
            name_without_article = name[:-len(article)]
            # Move article to the beginning
            name = f"{article_text} {name_without_article}"
            break
    
    return name.strip()



def extract_tracks(csv_path, output_path=None):
    """
    Extract tracks with artists and albums from the iTunes library CSV.

    Args:
        csv_path: Path to the compiled_itunes_library.csv file
        output_path: Path to write the JSON output (optional)

    Returns:
        List of tracks with artist and album information
    """
    tracks_list = []

    with open(csv_path, 'r', encoding='utf-8') as f:
        reader = csv.DictReader(f)
        for row in reader:
            track_name = row.get('Name', '').strip()
            artist = sanitize_artist_name(row.get('Artist', '').strip())
            album = row.get('Album', '').strip()

            # Skip tracks with invalid artist or album names (e.g., just question marks)
            if not is_valid_name(artist) or not is_valid_name(album):
                continue

            # Only add entries with non-empty track names
            if track_name:
                tracks_list.append({
                    'track': track_name,
                    'artist': artist if artist else None,
                    'album': album if album else None
                })

    # Prepare output data
    output_data = {
        'total_tracks': len(tracks_list),
        'tracks': tracks_list
    }

    # Write to file if output path specified
    if output_path:
        with open(output_path, 'w', encoding='utf-8') as f:
            json.dump(output_data, f, indent=2, ensure_ascii=False)
        print(f"Wrote {len(tracks_list)} tracks to {output_path}")

    return tracks_list


if __name__ == '__main__':
    # Default paths
    csv_file = Path(__file__).parent.parent / 'archive' / 'compiled_itunes_library.csv'
    output_file = Path(__file__).parent.parent / 'data' / 'tracks.json'

    # Create data directory if it doesn't exist
    output_file.parent.mkdir(exist_ok=True)

    # Extract and save tracks
    tracks = extract_tracks(csv_file, output_file)

    print(f"\nFound {len(tracks)} tracks")
    print(f"\nFirst 10 tracks:")
    for entry in tracks[:10]:
        artist_str = entry['artist'] if entry['artist'] else 'Unknown Artist'
        album_str = entry['album'] if entry['album'] else 'Unknown Album'
        print(f"  - {entry['track']}")
        print(f"    Artist: {artist_str}")
        print(f"    Album: {album_str}")
