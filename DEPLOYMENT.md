# GitHub Pages Deployment Guide

## Overview

This project is configured for continuous deployment to GitHub Pages. The web application will be automatically built and deployed whenever changes are pushed to the repository.

## Setup Instructions

### Prerequisites

1. **GitHub Account & Repository**
   - Repository: `riebschlager/mp3-collection`
   - GitHub Pages enabled (should be automatic with public repo)

2. **Local Setup**
   - Node.js 18+ installed
   - npm or yarn for package management

### Initial Configuration (Already Done)

The following configurations are already in place:

1. **Astro Configuration** (`mp3-collection-web/astro.config.mjs`)
   ```javascript
   export default defineConfig({
     site: 'https://riebschlager.github.io',
     base: '/mp3-collection',
     output: 'static',
     // ...
   });
   ```

2. **GitHub Actions Workflow** (`.github/workflows/deploy.yml`)
   - Automatically builds and deploys on push to main/master
   - Watches for changes in `mp3-collection-web/`, `web-data/`, and workflow files

### Deployment Workflow

#### Automatic Deployment

1. Make changes locally
2. Commit and push to `main` or `master` branch:
   ```bash
   git add .
   git commit -m "Update: description of changes"
   git push origin main
   ```
3. GitHub Actions automatically:
   - Installs dependencies
   - Builds the Astro project
   - Uploads artifacts to GitHub Pages
   - Deploys the site

#### Manual Deployment

1. Navigate to [Actions](https://github.com/riebschlager/mp3-collection/actions)
2. Select "Deploy to GitHub Pages" workflow
3. Click "Run workflow" button
4. Select the branch to deploy from
5. Click "Run workflow"

### Accessing the Live Site

Once deployed, visit:
```
https://riebschlager.github.io/mp3-collection
```

### Local Testing

Before pushing, test the production build locally:

```bash
cd mp3-collection-web

# Install dependencies (if not already done)
npm install

# Build for production
npm run build

# Preview the production build
npm run preview
```

Then visit `http://localhost:3000` (or the displayed URL) to verify everything works.

## Troubleshooting

### Site Not Deployed

1. Check GitHub Actions tab for workflow runs
2. Review workflow logs for build errors
3. Ensure the `site` and `base` URLs in `astro.config.mjs` are correct
4. Verify repository has GitHub Pages enabled (Settings → Pages)

### Wrong Base Path

The site is deployed at `/mp3-collection`, not the root. If you see broken links:
- Verify `base: '/mp3-collection'` in `astro.config.mjs`
- Update any hardcoded paths to use relative imports

### Build Failures

Check the GitHub Actions logs:
1. Go to [Actions](https://github.com/riebschlager/mp3-collection/actions)
2. Click on the failed workflow run
3. Expand the build step to see error messages
4. Common issues:
   - Missing dependencies: ensure `package.json` is committed
   - Build errors: verify local `npm run build` works before pushing

## Related Commands

```bash
# In mp3-collection-web/ directory

# Development
npm run dev              # Start dev server at localhost:4321

# Production
npm run build           # Build for production
npm run preview         # Preview production build locally

# Astro CLI
npm run astro -- cmd    # Run raw Astro CLI commands
```

## Files Modified for Deployment

- `mp3-collection-web/astro.config.mjs` - Added GitHub Pages configuration
- `.github/workflows/deploy.yml` - Created GitHub Actions workflow
- `README.md` - Added deployment documentation

