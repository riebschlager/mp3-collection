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
  return `${base}${cleanPath}`;
}
