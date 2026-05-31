import { mkdir, readFile, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(__dirname, '../../..');
const inputPath = path.join(repoRoot, 'data/derived/compiled/compiled_itunes_library.csv');
const outputPath = path.join(repoRoot, 'data/derived/web/itunes-library.json');

const columns = [
  'Name',
  'Artist',
  'Composer',
  'Album',
  'Grouping',
  'Genre',
  'Size',
  'Time',
  'Disc Number',
  'Disc Count',
  'Track Number',
  'Track Count',
  'Year',
  'Date Modified',
  'Date',
  'Date Added',
  'Bit Rate',
  'Sample Rate',
  'Volume Adjustment',
  'Kind',
  'Equalizer',
  'Comments',
  'Play Count',
  'Last Played',
  'My Rating',
  'Location',
  '_source_file',
  '_line_number',
];

function parseCsv(input) {
  const rows = [];
  let row = [];
  let field = '';
  let quoted = false;

  for (let i = 0; i < input.length; i += 1) {
    const char = input[i];
    const next = input[i + 1];

    if (quoted) {
      if (char === '"' && next === '"') {
        field += '"';
        i += 1;
      } else if (char === '"') {
        quoted = false;
      } else {
        field += char;
      }
      continue;
    }

    if (char === '"') {
      quoted = true;
    } else if (char === ',') {
      row.push(field);
      field = '';
    } else if (char === '\n') {
      row.push(field);
      rows.push(row);
      row = [];
      field = '';
    } else if (char !== '\r') {
      field += char;
    }
  }

  if (field || row.length) {
    row.push(field);
    rows.push(row);
  }

  return rows;
}

function toRecord(headers, row) {
  const record = {};
  headers.forEach((header, index) => {
    record[header] = row[index] ?? '';
  });
  return record;
}

function intValue(value) {
  const parsed = Number.parseInt(String(value ?? '').trim(), 10);
  return Number.isFinite(parsed) ? parsed : 0;
}

function naturalCompare(a, b) {
  return a.localeCompare(b, undefined, { numeric: true, sensitivity: 'base' });
}

function compactText(value) {
  return String(value ?? '').trim();
}

function sourceLabel(sourceFile) {
  const raw = compactText(sourceFile);
  if (!raw) return 'Unknown Source';
  return raw.replace(/\.txt$/i, '').replace(/^Library\.export/i, 'Library.export');
}

function formatDuration(totalSeconds) {
  const seconds = Math.max(0, totalSeconds);
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const remaining = seconds % 60;
  if (hours > 0) {
    return `${hours}:${String(minutes).padStart(2, '0')}:${String(remaining).padStart(2, '0')}`;
  }
  return `${minutes}:${String(remaining).padStart(2, '0')}`;
}

const csv = await readFile(inputPath, 'utf8');
const parsed = parseCsv(csv);
const headers = parsed.shift();

if (!headers || headers.length === 0) {
  throw new Error(`No CSV header found in ${inputPath}`);
}

const missing = columns.filter((column) => !headers.includes(column));
if (missing.length) {
  throw new Error(`Compiled CSV is missing expected columns: ${missing.join(', ')}`);
}

const sourceMap = new Map();
const tracks = parsed
  .filter((row) => row.length > 1)
  .map((row, index) => {
    const record = toRecord(headers, row);
    const sourceFile = compactText(record._source_file);
    const id = `itunes-${String(index + 1).padStart(5, '0')}`;
    const duration = intValue(record.Time);
    const size = intValue(record.Size);

    if (!sourceMap.has(sourceFile)) {
      sourceMap.set(sourceFile, []);
    }
    sourceMap.get(sourceFile).push(id);

    return {
      id,
      name: compactText(record.Name),
      artist: compactText(record.Artist),
      composer: compactText(record.Composer),
      album: compactText(record.Album),
      grouping: compactText(record.Grouping),
      genre: compactText(record.Genre),
      size,
      duration,
      durationFormatted: formatDuration(duration),
      discNumber: intValue(record['Disc Number']) || undefined,
      discCount: intValue(record['Disc Count']) || undefined,
      trackNumber: intValue(record['Track Number']) || undefined,
      trackCount: intValue(record['Track Count']) || undefined,
      year: compactText(record.Year),
      dateModified: compactText(record['Date Modified'] || record.Date),
      dateAdded: compactText(record['Date Added']),
      bitRate: compactText(record['Bit Rate']),
      sampleRate: compactText(record['Sample Rate']),
      volumeAdjustment: compactText(record['Volume Adjustment']),
      kind: compactText(record.Kind),
      equalizer: compactText(record.Equalizer),
      comments: compactText(record.Comments),
      playCount: intValue(record['Play Count']),
      lastPlayed: compactText(record['Last Played']),
      rating: intValue(record['My Rating']),
      location: compactText(record.Location),
      sourceFile,
      sourceLine: intValue(record._line_number),
    };
  });

const playlists = [...sourceMap.entries()]
  .sort(([a], [b]) => naturalCompare(a, b))
  .map(([sourceFile, ids]) => ({
    id: sourceFile,
    name: sourceLabel(sourceFile),
    sourceFile,
    trackCount: ids.length,
    trackIds: ids,
  }));

const totalDurationSeconds = tracks.reduce((sum, track) => sum + track.duration, 0);
const totalSizeBytes = tracks.reduce((sum, track) => sum + track.size, 0);
const data = {
  meta: {
    generatedFrom: 'data/derived/compiled/compiled_itunes_library.csv',
    totalTracks: tracks.length,
    totalPlaylists: playlists.length,
    totalDurationSeconds,
    totalDurationFormatted: formatDuration(totalDurationSeconds),
    totalSizeBytes,
    sourceCounts: Object.fromEntries(playlists.map((playlist) => [playlist.sourceFile, playlist.trackCount])),
  },
  playlists,
  tracks,
};

await mkdir(path.dirname(outputPath), { recursive: true });
await writeFile(outputPath, `${JSON.stringify(data)}\n`);

console.log(`Wrote ${path.relative(repoRoot, outputPath)}`);
console.log(`${tracks.length} tracks`);
console.log(`${playlists.length} playlists`);
