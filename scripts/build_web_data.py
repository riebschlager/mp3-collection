#!/usr/bin/env python3
"""
Build optimized web data from the compiled iTunes library CSV.
Creates enriched JSON chunks with all metadata, artist/album indexes, and metadata facets.
"""

import csv
import json
import re
from datetime import datetime
from pathlib import Path
from collections import defaultdict


def is_valid_name(name):
    """Check if a name is valid (not just question marks)"""
    if not name:
        return False
    clean_name = str(name).strip()
    # Check if name is only question marks or empty
    return clean_name and clean_name != '?' and not all(c == '?' for c in clean_name)


def slugify(text):
    """Convert text to URL-friendly slug"""
    if not text:
        return 'unknown'
    text = str(text).lower()
    # Remove non-word characters (keep alphanumeric, spaces, hyphens)
    text = re.sub(r'[^\w\s-]', '', text)
    # Replace spaces and multiple hyphens with single hyphen
    text = re.sub(r'[-\s]+', '-', text)
    result = text.strip('-')
    # If result is empty (all special chars), return 'unknown'
    return result if result else 'unknown'


def safe_int(value, default=0):
    """Safely convert value to int, return default if empty/invalid"""
    if not value or str(value).strip() == '':
        return default
    try:
        return int(float(value))
    except (ValueError, TypeError):
        return default


def safe_str(value):
    """Return string or None if empty"""
    if not value or str(value).strip() == '':
        return None
    return str(value).strip()


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


def sanitize_genre(genre):
    """Return cleaned genre or None for invalid genres.

    Rules:
    - Strip surrounding quotes.
    - Require at least one ASCII letter to consider it a valid genre.
    - Return None for numeric-only or punctuation-only values.
    """
    if not genre:
        return None
    g = str(genre).strip()
    # strip surrounding quotes
    while g.startswith('"""') or g.startswith('"'):
        if g.startswith('"""'):
            g = g[3:]
        else:
            g = g[1:]
    while g.endswith('"""') or g.endswith('"'):
        if g.endswith('"""'):
            g = g[:-3]
        else:
            g = g[:-1]
    g = g.strip()

    # Accept only if there's at least one letter (avoid numeric-only genres)
    if re.search(r'[A-Za-z]', g):
        return g
    return None


def sanitize_year(value):
    """Return a valid year (int) or None if out of reasonable bounds.

    Rules:
    - Parse the value as integer.
    - Accept years between 1000 and current year inclusive.
    - Return None for invalid or out-of-range years.
    """
    y = safe_int(value, default=0)
    if y <= 0:
        return None
    current_year = datetime.now().year
    if 1000 <= y <= current_year:
        return y
    return None


def format_duration(seconds):
    """Format duration in seconds to MM:SS format"""
    if not seconds:
        return '0:00'
    minutes = seconds // 60
    secs = seconds % 60
    return f'{minutes}:{secs:02d}'


def build_web_data(csv_path, output_dir):
    """Build optimized JSON files for web display"""

    output_dir = Path(output_dir)
    chunks_dir = output_dir / 'chunks'
    chunks_dir.mkdir(parents=True, exist_ok=True)

    print(f"Reading CSV from: {csv_path}")
    print(f"Output directory: {output_dir}\n")

    # Data structures
    tracks = []
    artists_map = defaultdict(lambda: {'name': '', 'albums': set(), 'trackIds': []})
    albums_map = defaultdict(lambda: {'name': '', 'artists': set(), 'trackIds': []})
    genres = set()
    years = set()

    # Statistics
    total_size = 0
    total_duration = 0
    total_bitrate_sum = 0
    bitrate_count = 0

    # Read CSV with all fields
    with open(csv_path, 'r', encoding='utf-8') as f:
        reader = csv.DictReader(f)
        for row in reader:
            track_name = row.get('Name', '').strip()
            if not track_name:
                continue

            # Extract all fields
            artist_name = sanitize_artist_name(safe_str(row.get('Artist'))) or 'Unknown Artist'
            album_name = safe_str(row.get('Album')) or 'Unknown Album'
            
            # Skip tracks with invalid artist or album names (e.g., just question marks)
            if not is_valid_name(artist_name) or not is_valid_name(album_name):
                continue
            
            composer = safe_str(row.get('Composer'))
            genre = sanitize_genre(safe_str(row.get('Genre')))
            year = sanitize_year(row.get('Year'))

            # Technical metadata
            size = safe_int(row.get('Size'))
            duration = safe_int(row.get('Time'))
            bit_rate = safe_int(row.get('Bit Rate'))
            sample_rate = safe_int(row.get('Sample Rate'))

            # Track/disc info
            track_number = safe_int(row.get('Track Number'))
            track_count = safe_int(row.get('Track Count'))
            disc_number = safe_int(row.get('Disc Number'))
            disc_count = safe_int(row.get('Disc Count'))

            # User data
            play_count = safe_int(row.get('Play Count'))
            rating = safe_int(row.get('My Rating'))

            # Dates and other metadata
            date_added = safe_str(row.get('Date Added'))
            date_modified = safe_str(row.get('Date Modified'))
            last_played = safe_str(row.get('Last Played'))
            location = safe_str(row.get('Location'))
            kind = safe_str(row.get('Kind'))
            grouping = safe_str(row.get('Grouping'))
            comments = safe_str(row.get('Comments'))
            volume_adjustment = safe_int(row.get('Volume Adjustment'))
            equalizer = safe_str(row.get('Equalizer'))

            # Create track object
            track_id = f"track-{len(tracks):05d}"
            artist_slug = slugify(artist_name)
            album_slug = slugify(album_name)

            track = {
                'id': track_id,
                'name': track_name,
                'artist': artist_name,
                'artistSlug': artist_slug,
                'composer': composer,
                'album': album_name,
                'albumSlug': album_slug,
                'grouping': grouping,
                'genre': genre,
                'year': year if year is not None else None,
                'size': size,
                'duration': duration,
                'durationFormatted': format_duration(duration),
                'bitRate': bit_rate if bit_rate > 0 else None,
                'sampleRate': sample_rate if sample_rate > 0 else None,
                'trackNumber': track_number if track_number > 0 else None,
                'trackCount': track_count if track_count > 0 else None,
                'discNumber': disc_number if disc_number > 0 else None,
                'discCount': disc_count if disc_count > 0 else None,
                'playCount': play_count,
                'lastPlayed': last_played,
                'rating': rating,
                'dateAdded': date_added,
                'dateModified': date_modified,
                'location': location,
                'kind': kind,
                'volumeAdjustment': volume_adjustment if volume_adjustment != 0 else None,
                'equalizer': equalizer,
                'comments': comments,
            }

            tracks.append(track)

            # Build indexes
            artists_map[artist_slug]['name'] = artist_name
            artists_map[artist_slug]['albums'].add(album_name)
            artists_map[artist_slug]['trackIds'].append(track_id)

            albums_map[album_slug]['name'] = album_name
            albums_map[album_slug]['artists'].add(artist_name)
            albums_map[album_slug]['trackIds'].append(track_id)

            # Collect metadata
            if genre:
                genres.add(genre)
            if year is not None:
                years.add(year)

            # Statistics
            total_size += size
            total_duration += duration
            if bit_rate > 0:
                total_bitrate_sum += bit_rate
                bitrate_count += 1

    print(f"Processed {len(tracks)} tracks")
    print(f"Found {len(artists_map)} unique artists")
    print(f"Found {len(albums_map)} unique albums")
    print(f"Found {len(genres)} genres")
    print(f"Found {len(years)} years\n")

    # Split tracks into chunks (1000 per chunk)
    chunk_size = 1000
    total_chunks = (len(tracks) + chunk_size - 1) // chunk_size

    print(f"Creating {total_chunks} track chunks...")
    for i in range(total_chunks):
        chunk_tracks = tracks[i * chunk_size:(i + 1) * chunk_size]
        chunk_data = {
            'chunk': i + 1,
            'totalChunks': total_chunks,
            'count': len(chunk_tracks),
            'tracks': chunk_tracks
        }

        chunk_file = chunks_dir / f'tracks-{i+1:03d}.json'
        with open(chunk_file, 'w', encoding='utf-8') as f:
            json.dump(chunk_data, f, ensure_ascii=False)

        if (i + 1) % 10 == 0 or i == total_chunks - 1:
            print(f"  Wrote chunk {i+1}/{total_chunks}")

    # Write artist index
    print("\nCreating artist index...")
    artists_index = {
        'total': len(artists_map),
        'artists': sorted([
            {
                'slug': slug,
                'name': data['name'],
                'albumCount': len(data['albums']),
                'trackCount': len(data['trackIds']),
                'albums': sorted(list(data['albums']))
            }
            for slug, data in artists_map.items()
        ], key=lambda x: x['name'].lower())
    }

    with open(output_dir / 'artists-index.json', 'w', encoding='utf-8') as f:
        json.dump(artists_index, f, ensure_ascii=False, indent=2)
    print(f"  Wrote {len(artists_map)} artists to artists-index.json")

    # Write album index
    print("\nCreating album index...")
    albums_index = {
        'total': len(albums_map),
        'albums': sorted([
            {
                'slug': slug,
                'name': data['name'],
                'artistCount': len(data['artists']),
                'trackCount': len(data['trackIds']),
                'artists': sorted(list(data['artists']))
            }
            for slug, data in albums_map.items()
        ], key=lambda x: x['name'].lower())
    }

    with open(output_dir / 'albums-index.json', 'w', encoding='utf-8') as f:
        json.dump(albums_index, f, ensure_ascii=False, indent=2)
    print(f"  Wrote {len(albums_map)} albums to albums-index.json")

    # Write metadata
    print("\nCreating metadata file...")
    avg_bitrate = (total_bitrate_sum / bitrate_count) if bitrate_count > 0 else 0
    total_hours = total_duration / 3600

    metadata = {
        'totalTracks': len(tracks),
        'totalArtists': len(artists_map),
        'totalAlbums': len(albums_map),
        'genres': sorted(list(genres)),
        'years': sorted(list(years)),
        'stats': {
            'totalSizeBytes': total_size,
            'totalSizeGB': round(total_size / (1024**3), 2),
            'totalDurationSeconds': total_duration,
            'totalDurationHours': round(total_hours, 1),
            'totalDurationFormatted': f"{int(total_hours)}h {int((total_hours % 1) * 60)}m",
            'avgBitRate': round(avg_bitrate, 1),
            'tracksWithPlayCount': sum(1 for t in tracks if t['playCount'] > 0),
            'tracksWithRating': sum(1 for t in tracks if t['rating'] > 0),
        }
    }

    with open(output_dir / 'metadata.json', 'w', encoding='utf-8') as f:
        json.dump(metadata, f, ensure_ascii=False, indent=2)
    print(f"  Wrote metadata.json")

    # Print summary
    print("\n" + "="*60)
    print("BUILD COMPLETE!")
    print("="*60)
    print(f"Total Tracks:     {metadata['totalTracks']:,}")
    print(f"Total Artists:    {metadata['totalArtists']:,}")
    print(f"Total Albums:     {metadata['totalAlbums']:,}")
    print(f"Total Size:       {metadata['stats']['totalSizeGB']} GB")
    print(f"Total Duration:   {metadata['stats']['totalDurationFormatted']}")
    print(f"Avg Bit Rate:     {metadata['stats']['avgBitRate']} kbps")
    print(f"Tracks Played:    {metadata['stats']['tracksWithPlayCount']:,}")
    print(f"Tracks Rated:     {metadata['stats']['tracksWithRating']:,}")
    print("="*60)
    print(f"\nOutput directory: {output_dir.absolute()}")
    print(f"Track chunks:     {total_chunks} files in {chunks_dir.name}/")
    print(f"Index files:      artists-index.json, albums-index.json, metadata.json")
    print("\nReady for Astro build!")


if __name__ == '__main__':
    # Default paths
    csv_file = Path(__file__).parent.parent / 'archive' / 'compiled_itunes_library.csv'
    output_dir = Path(__file__).parent.parent / 'web-data'

    if not csv_file.exists():
        print(f"ERROR: CSV file not found: {csv_file}")
        exit(1)

    build_web_data(csv_file, output_dir)
