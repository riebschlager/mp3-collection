#!/usr/bin/env python3
"""
Extract a de-duplicated list of artists from the compiled iTunes library CSV.
Outputs artists with their associated albums as JSON.
"""

import csv
import json
from pathlib import Path


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


def extract_artists(csv_path, output_path=None):
    """
    Extract unique artists with their albums from the iTunes library CSV.

    Args:
        csv_path: Path to the compiled_itunes_library.csv file
        output_path: Path to write the JSON output (optional)

    Returns:
        List of unique artists with album information
    """
    # Use dict to track artist -> set of albums
    artists_dict = {}

    with open(csv_path, 'r', encoding='utf-8') as f:
        reader = csv.DictReader(f)
        for row in reader:
            artist = sanitize_artist_name(row.get('Artist', '').strip())
            album = row.get('Album', '').strip()

            # Only add entries with non-empty artist names
            if artist:
                if artist not in artists_dict:
                    artists_dict[artist] = set()
                if album:  # Only add non-empty albums
                    artists_dict[artist].add(album)

    # Convert to list of dicts with sorted albums
    artist_list = []
    for artist, albums in sorted(artists_dict.items()):
        artist_list.append({
            'artist': artist,
            'albums': sorted(albums)
        })

    # Prepare output data
    output_data = {
        'total_artists': len(artist_list),
        'artists': artist_list
    }

    # Write to file if output path specified
    if output_path:
        with open(output_path, 'w', encoding='utf-8') as f:
            json.dump(output_data, f, indent=2, ensure_ascii=False)
        print(f"Wrote {len(artist_list)} unique artists to {output_path}")

    return artist_list


if __name__ == '__main__':
    # Default paths
    csv_file = Path(__file__).parent.parent / 'archive' / 'compiled_itunes_library.csv'
    output_file = Path(__file__).parent.parent / 'data' / 'artists.json'

    # Create data directory if it doesn't exist
    output_file.parent.mkdir(exist_ok=True)

    # Extract and save artists
    artists = extract_artists(csv_file, output_file)

    print(f"\nFound {len(artists)} unique artists")
    print(f"\nFirst 10 artists:")
    for entry in artists[:10]:
        album_count = len(entry['albums'])
        albums_preview = ', '.join(entry['albums'][:3])
        if album_count > 3:
            albums_preview += f" ... (+{album_count - 3} more)"
        print(f"  - {entry['artist']} ({album_count} albums)")
        print(f"    {albums_preview}")
