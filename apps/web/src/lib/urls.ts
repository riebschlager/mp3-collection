/**
 * URL utility functions that respect the BASE_URL configuration
 * Used for ensuring links work correctly when deployed to subdirectories
 */

export function getBaseUrl(): string {
  return import.meta.env.BASE_URL;
}

export function buildUrl(path: string): string {
  const base = getBaseUrl();
  const cleanPath = path.startsWith('/') ? path.slice(1) : path;
  const separator = base.endsWith('/') ? '' : '/';
  return `${base}${separator}${cleanPath}`;
}

export interface LibraryUrlState {
  genre?: string;
  artist?: string;
  album?: string;
  q?: string;
  sort?: string;
  dir?: string;
}

export function buildLibraryUrl(state: LibraryUrlState = {}): string {
  const params = new URLSearchParams();

  (['genre', 'artist', 'album', 'q', 'sort', 'dir'] as const).forEach((key) => {
    const value = state[key];
    if (value) {
      params.set(key, value);
    }
  });

  const base = buildUrl('library');
  const query = params.toString();
  return query ? `${base}?${query}` : base;
}
