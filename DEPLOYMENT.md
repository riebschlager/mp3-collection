# GitHub Pages Deployment

## Overview

The repository deploys the Astro site in `mp3-collection-web/` to GitHub Pages using GitHub Actions.

Live URL:
`https://riebschlager.github.io/mp3-collection`

## Workflow Source of Truth

- Workflow file: `.github/workflows/deploy.yml`
- Astro config: `mp3-collection-web/astro.config.mjs`

Current workflow behavior:
- Trigger on pushes to `main` or `master`
- Trigger only when these paths change:
  - `mp3-collection-web/**`
  - `web-data/**`
  - `.github/workflows/deploy.yml`
- Also supports manual trigger (`workflow_dispatch`)
- Builds with Node `20`
- Checks out Git LFS objects (`actions/checkout` with `lfs: true`)
- Runs `npm ci` and `npm run build` in `mp3-collection-web/`

## Prerequisites

- Repository configured with GitHub Pages
- Node.js 20+ locally (to mirror CI)
- Git LFS installed locally (`git lfs install`)

## Deployment Flow

### Automatic

1. Update data and/or web app files.
2. Commit and push to `main` or `master`.
3. GitHub Actions builds and deploys `mp3-collection-web/dist`.

### Manual

1. Open [GitHub Actions](https://github.com/riebschlager/mp3-collection/actions).
2. Select `Deploy to GitHub Pages`.
3. Click `Run workflow`.

## Local Verification Before Push

```bash
cd mp3-collection-web
npm install
npm run build
npm run preview
```

Note:
- The web app reads data from `public/data` (symlink to `../../web-data`).
- If data changed but `web-data/**` was not updated, deployment will not include new data.

## Troubleshooting

### Deployment did not run

- Confirm your push touched one of the watched paths.
- Confirm branch is `main` or `master`.
- Check workflow run status in GitHub Actions.

### Broken links/assets on deployed site

- Verify `base: '/mp3-collection'` in `mp3-collection-web/astro.config.mjs`.
- Ensure links use base-aware URL helpers (already used in layout/pages).

### Build fails in CI but works locally

- Re-test with Node 20 locally.
- Re-run clean install: `npm ci` inside `mp3-collection-web/`.
