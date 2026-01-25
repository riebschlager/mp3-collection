#!/usr/bin/env python3
"""
Extract a de-duplicated list of albums from the compiled iTunes library CSV.
Outputs the album list as JSON.
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



def extract_albums(csv_path, output_path=None):
    """
    Extract unique albums with artists from the iTunes library CSV.

    Args:
        csv_path: Path to the compiled_itunes_library.csv file
        output_path: Path to write the JSON output (optional)

    Returns:
        List of unique albums with artist information
    """
    # Use dict to track album -> set of artists
    albums_dict = {}

    with open(csv_path, 'r', encoding='utf-8') as f:
        reader = csv.DictReader(f)
        for row in reader:
            album = row.get('Album', '').strip()
            artist = sanitize_artist_name(row.get('Artist', '').strip())

            # Only add entries with valid album names (not just question marks)
            if album and is_valid_name(album):
                if album not in albums_dict:
                    albums_dict[album] = set()
                if artist:  # Only add non-empty artists
                    albums_dict[album].add(artist)

    # Convert to list of dicts with sorted artists
    album_list = []
    for album, artists in sorted(albums_dict.items()):
        album_list.append({
            'album': album,
            'artists': sorted(artists)
        })

    # Prepare output data
    output_data = {
        'total_albums': len(album_list),
        'albums': album_list
    }

    # Write to file if output path specified
    if output_path:
        with open(output_path, 'w', encoding='utf-8') as f:
            json.dump(output_data, f, indent=2, ensure_ascii=False)
        print(f"Wrote {len(album_list)} unique albums to {output_path}")

    return album_list


if __name__ == '__main__':
    # Default paths
    csv_file = Path(__file__).parent.parent / 'archive' / 'compiled_itunes_library.csv'
    output_file = Path(__file__).parent.parent / 'data' / 'albums.json'

    # Create data directory if it doesn't exist
    output_file.parent.mkdir(exist_ok=True)

    # Extract and save albums
    albums = extract_albums(csv_file, output_file)

    print(f"\nFound {len(albums)} unique albums")
    print(f"\nFirst 10 albums:")
    for entry in albums[:10]:
        artists_str = ', '.join(entry['artists']) if entry['artists'] else 'Unknown Artist'
        print(f"  - {entry['album']} — {artists_str}")
