# MP3 Collection Web

Astro frontend for browsing the MP3 archive and listening-history outputs.

## Routes

- `/`: overview dashboard with collection stats and most-played tracks
- `/artists`: artist index
- `/artists/[slug]`: artist detail page with grouped albums/tracks
- `/albums`: album index
- `/albums/[slug]`: album detail page with tracklist
- `/tracks/[page]`: paginated track browser
- `/history`: yearly/monthly listening timeline view
- `/wrapped`: wrapped-year gallery
- `/wrapped/[year]`: slide-based yearly wrapped story

## Data Source

The app reads static JSON from `public/data`, which is a symlink to the repository `web-data` folder:

```text
public/data -> ../../../web-data
```

If the symlink is missing, recreate it from `apps/web/`:

```bash
ln -s ../../../web-data public/data
```

## Commands

Run from `apps/web/`:

```bash
npm install
npm run dev
```

Additional commands:
- `npm run build`: production build to `dist/`
- `npm run preview`: preview production build
- `npm run astro -- --help`: Astro CLI help

## GitHub Pages Configuration

`astro.config.mjs` is configured for GitHub Pages:
- `site: 'https://riebschlager.github.io'`
- `base: '/mp3-collection'`
- `output: 'static'`

Route links in the app use `import.meta.env.BASE_URL` helpers to stay base-path safe.

## Current Implementation Notes

- Several dynamic pages currently assume `49` track chunks:
  - `src/pages/tracks/[page].astro`
  - `src/pages/artists/[slug].astro`
  - `src/pages/albums/[slug].astro`
- If `web-data/chunks` count changes, update those constants to match.
