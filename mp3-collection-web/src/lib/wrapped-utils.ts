/**
 * Utility functions for Wrapped feature data processing
 */

export interface Track {
  track: string;
  artist: string;
  playCount: number;
}

export interface MonthData {
  month: string;
  totalScrobbles: number;
  uniqueTracks: number;
  topTracks: Track[];
}

export interface YearData {
  year: number;
  totalScrobbles: number;
  uniqueTracks: number;
  topTracks: Track[];
  months: MonthData[];
}

export interface ArtistStat {
  artist: string;
  playCount: number;
  topTrack: string;
}

export interface Badge {
  id: string;
  name: string;
  description: string;
  icon: string;
}

/**
 * Derive top artists from top tracks by aggregating play counts
 */
export function getTopArtists(tracks: Track[], limit = 5): ArtistStat[] {
  const artistCounts = new Map<string, ArtistStat>();

  tracks.forEach((track) => {
    const existing = artistCounts.get(track.artist);
    if (existing) {
      existing.playCount += track.playCount;
    } else {
      artistCounts.set(track.artist, {
        artist: track.artist,
        playCount: track.playCount,
        topTrack: track.track,
      });
    }
  });

  return Array.from(artistCounts.values())
    .sort((a, b) => b.playCount - a.playCount)
    .slice(0, limit);
}

/**
 * Calculate total minutes listened based on scrobble count
 * Average song length estimated at 3.5 minutes
 */
export function getTotalMinutes(scrobbles: number): number {
  const avgSongMinutes = 3.5;
  return Math.round(scrobbles * avgSongMinutes);
}

/**
 * Format minutes into human-readable time (e.g., "5 hours" or "2 days")
 */
export function formatListeningTime(minutes: number): string {
  const hours = Math.round(minutes / 60);
  const days = Math.round(hours / 24);

  if (days >= 1) {
    return `${days} day${days > 1 ? 's' : ''}`;
  } else if (hours >= 1) {
    return `${hours} hour${hours > 1 ? 's' : ''}`;
  } else {
    return `${minutes} minute${minutes > 1 ? 's' : ''}`;
  }
}

/**
 * Find the peak listening month from monthly data
 */
export function getPeakMonth(months: MonthData[]): MonthData | null {
  if (months.length === 0) return null;
  return months.reduce((max, month) =>
    month.totalScrobbles > max.totalScrobbles ? month : max
  );
}

/**
 * Format month string (YYYY-MM) to readable format (e.g., "January 2012")
 */
export function formatMonth(monthStr: string): string {
  const [year, month] = monthStr.split('-');
  const date = new Date(parseInt(year), parseInt(month) - 1, 1);
  return date.toLocaleDateString('en-US', { month: 'long', year: 'numeric' });
}

/**
 * Calculate variety score (uniqueness of listening)
 * Higher score = more variety
 */
export function getVarietyScore(uniqueTracks: number, totalScrobbles: number): number {
  return Math.round((uniqueTracks / totalScrobbles) * 100);
}

/**
 * Get listening consistency (percentage of months with activity)
 */
export function getListeningConsistency(months: MonthData[]): number {
  const activeMonths = months.filter((m) => m.totalScrobbles > 0).length;
  return Math.round((activeMonths / 12) * 100);
}

/**
 * Calculate average scrobbles per day
 */
export function getAvgScrobblesPerDay(totalScrobbles: number): number {
  return Math.round(totalScrobbles / 365);
}

/**
 * Determine achievement badges based on listening patterns
 */
export function getBadges(yearData: YearData): Badge[] {
  const badges: Badge[] = [];
  const varietyScore = getVarietyScore(yearData.uniqueTracks, yearData.totalScrobbles);
  const topTrackCount = yearData.topTracks[0]?.playCount || 0;
  const avgScrobblesPerDay = getAvgScrobblesPerDay(yearData.totalScrobbles);

  // Explorer badge (high variety)
  if (varietyScore >= 70) {
    badges.push({
      id: 'explorer',
      name: 'Explorer',
      description: 'Listened to a wide variety of music',
      icon: '🗺️',
    });
  }

  // Deep Diver badge (top track played many times)
  if (topTrackCount >= 15) {
    badges.push({
      id: 'deep-diver',
      name: 'Deep Diver',
      description: 'Found your favorites and stuck with them',
      icon: '🌊',
    });
  }

  // Music Devotee badge (listened almost every day)
  if (avgScrobblesPerDay >= 10) {
    badges.push({
      id: 'devotee',
      name: 'Music Devotee',
      description: 'Music was part of your daily routine',
      icon: '🎵',
    });
  }

  // Discoverer badge (lots of unique tracks)
  if (yearData.uniqueTracks >= 500) {
    badges.push({
      id: 'discoverer',
      name: 'Discoverer',
      description: `Explored ${yearData.uniqueTracks.toLocaleString()} unique tracks`,
      icon: '✨',
    });
  }

  return badges;
}

/**
 * Generate a fun fact based on listening data
 */
export function getFunFact(yearData: YearData): string {
  const topTrack = yearData.topTracks[0];
  const peakMonth = getPeakMonth(yearData.months);
  const totalMinutes = getTotalMinutes(yearData.totalScrobbles);
  const totalHours = Math.round(totalMinutes / 60);
  const varietyScore = getVarietyScore(yearData.uniqueTracks, yearData.totalScrobbles);

  // Collection of possible fun facts
  const facts: string[] = [];

  if (topTrack && topTrack.playCount >= 20) {
    facts.push(`You played "${topTrack.track}" ${topTrack.playCount} times! That's dedication.`);
  }

  if (peakMonth) {
    const monthName = formatMonth(peakMonth.month);
    facts.push(`${monthName} was your busiest month with ${peakMonth.totalScrobbles.toLocaleString()} scrobbles!`);
  }

  if (totalHours >= 100) {
    const days = Math.round(totalHours / 24);
    facts.push(`You listened to ${days} full days of music this year!`);
  }

  if (varietyScore >= 75) {
    facts.push(`Your variety score of ${varietyScore}% means you're a true music explorer!`);
  }

  if (yearData.uniqueTracks >= 1000) {
    facts.push(`You discovered ${yearData.uniqueTracks.toLocaleString()} unique tracks - that's impressive!`);
  }

  // Return a random fun fact, or a default one if none match
  return facts.length > 0 ? facts[Math.floor(Math.random() * facts.length)] : `You listened to ${yearData.totalScrobbles.toLocaleString()} songs this year!`;
}

/**
 * Get season name from month number (1-12)
 */
export function getSeasonFromMonth(monthNum: number): string {
  if (monthNum >= 3 && monthNum <= 5) return 'Spring';
  if (monthNum >= 6 && monthNum <= 8) return 'Summer';
  if (monthNum >= 9 && monthNum <= 11) return 'Fall';
  return 'Winter';
}

/**
 * Get all available years from timeline data
 */
export function getAvailableYears(timelineData: any): number[] {
  return timelineData.years.map((y: any) => y.year).sort((a: number, b: number) => a - b);
}
