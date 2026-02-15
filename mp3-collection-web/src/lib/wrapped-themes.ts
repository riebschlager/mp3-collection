/**
 * Theme configurations for different eras of Spotify Wrapped
 * Each year gets a theme inspired by the design trends of that time
 */

export interface WrappedTheme {
  gradient: string;
  accent: string;
  secondary: string;
  era: 'retro' | 'memphis' | 'modern';
}

export const wrappedThemes: Record<number, WrappedTheme> = {
  // 2005-2010: Retro Digital Era
  // Purple, pink, orange gradients with sunset aesthetic
  2005: {
    gradient: 'from-purple-500 via-pink-500 to-purple-600',
    accent: 'purple-600',
    secondary: 'pink-500',
    era: 'retro',
  },
  2006: {
    gradient: 'from-orange-400 via-red-400 to-pink-500',
    accent: 'orange-600',
    secondary: 'red-500',
    era: 'retro',
  },
  2007: {
    gradient: 'from-pink-400 via-purple-400 to-indigo-500',
    accent: 'pink-600',
    secondary: 'purple-500',
    era: 'retro',
  },
  2008: {
    gradient: 'from-violet-500 via-purple-500 to-fuchsia-500',
    accent: 'violet-600',
    secondary: 'fuchsia-500',
    era: 'retro',
  },
  2009: {
    gradient: 'from-rose-400 via-pink-500 to-orange-500',
    accent: 'rose-600',
    secondary: 'orange-500',
    era: 'retro',
  },
  2010: {
    gradient: 'from-amber-400 via-orange-500 to-red-500',
    accent: 'amber-600',
    secondary: 'red-500',
    era: 'retro',
  },

  // 2011-2016: Memphis Style Era
  // Bold colors, high contrast, playful patterns
  2011: {
    gradient: 'from-blue-500 via-cyan-500 to-teal-500',
    accent: 'blue-600',
    secondary: 'cyan-500',
    era: 'memphis',
  },
  2012: {
    gradient: 'from-green-500 via-emerald-500 to-teal-600',
    accent: 'green-600',
    secondary: 'emerald-500',
    era: 'memphis',
  },
  2013: {
    gradient: 'from-yellow-400 via-orange-500 to-red-500',
    accent: 'yellow-600',
    secondary: 'orange-500',
    era: 'memphis',
  },
  2014: {
    gradient: 'from-indigo-500 via-blue-600 to-purple-600',
    accent: 'indigo-600',
    secondary: 'blue-600',
    era: 'memphis',
  },
  2015: {
    gradient: 'from-fuchsia-500 via-pink-600 to-rose-600',
    accent: 'fuchsia-600',
    secondary: 'pink-600',
    era: 'memphis',
  },
  2016: {
    gradient: 'from-cyan-500 via-blue-500 to-indigo-600',
    accent: 'cyan-600',
    secondary: 'blue-500',
    era: 'memphis',
  },

  // 2025-2026: Modern Hyperloop Era
  // Bright technicolor, neon accents, glassmorphism
  2025: {
    gradient: 'from-cyan-400 via-blue-500 to-purple-600',
    accent: 'cyan-500',
    secondary: 'purple-600',
    era: 'modern',
  },
  2026: {
    gradient: 'from-pink-500 via-fuchsia-500 to-purple-600',
    accent: 'pink-500',
    secondary: 'fuchsia-600',
    era: 'modern',
  },
};

export function getTheme(year: number): WrappedTheme {
  return wrappedThemes[year] || wrappedThemes[2025]; // Default to modern theme
}

export function getEraName(era: 'retro' | 'memphis' | 'modern'): string {
  switch (era) {
    case 'retro':
      return 'Retro Digital';
    case 'memphis':
      return 'Memphis Style';
    case 'modern':
      return 'Hyperloop';
  }
}
