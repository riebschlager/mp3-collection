/**
 * Album artwork utilities
 * Generates deterministic, colorful placeholders for albums
 */

/**
 * Generate a consistent color from a string (album + artist name)
 * Returns HSL values for vibrant, distinct colors
 */
export function getAlbumColor(albumName: string, artistName: string): string {
  const combined = `${albumName}-${artistName}`.toLowerCase();
  let hash = 0;

  for (let i = 0; i < combined.length; i++) {
    hash = combined.charCodeAt(i) + ((hash << 5) - hash);
  }

  // Generate hue (0-360) for variety, keep saturation and lightness in good ranges
  const hue = Math.abs(hash % 360);
  const saturation = 65 + (Math.abs(hash) % 20); // 65-85%
  const lightness = 50 + (Math.abs(hash >> 8) % 15); // 50-65%

  return `hsl(${hue}, ${saturation}%, ${lightness}%)`;
}

/**
 * Get a gradient for album artwork
 */
export function getAlbumGradient(albumName: string, artistName: string): string {
  const color1 = getAlbumColor(albumName, artistName);
  const color2 = getAlbumColor(artistName, albumName); // Reverse for variation

  return `linear-gradient(135deg, ${color1} 0%, ${color2} 100%)`;
}

/**
 * Get initials from album name for display
 */
export function getAlbumInitials(albumName: string): string {
  if (!albumName || albumName === 'Unknown Album') {
    return '?';
  }

  const words = albumName
    .replace(/[^a-zA-Z0-9\s]/g, '') // Remove special chars
    .split(/\s+/)
    .filter(w => w.length > 0);

  if (words.length === 0) return '?';
  if (words.length === 1) return words[0].substring(0, 2).toUpperCase();

  // Take first letter of first two words
  return (words[0][0] + words[1][0]).toUpperCase();
}
