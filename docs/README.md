# Boxer Docs

Built with [Docusaurus](https://docusaurus.io/).

## Local Development

```bash
pnpm install
pnpm start
```

Opens `http://localhost:3000/boxer/`. Most changes are reflected live.

## Build

```bash
pnpm build
```

Generates static files into the `build/` directory.

## Deployment

Deployment is automated via GitHub Actions (`.github/workflows/docs.yml`).
Every push to `main` builds the site and deploys it to GitHub Pages.

## Search

Search is powered by [Algolia DocSearch](https://docsearch.algolia.com/). It is free for open-source documentation.

### Applying for DocSearch

1. Apply at https://docsearch.algolia.com/apply/ with:
   - **Documentation URL**: `https://theonekeyg.github.io/boxer/`
   - **Repository**: `https://github.com/theonekeyg/boxer`
2. Algolia will email `appId`, `apiKey` (search key), and `indexName` within a few days.
3. In the Algolia Crawler dashboard, paste the contents of `algolia-crawler-config.json` as the crawler configuration.

### Setting up secrets

Add three secrets to **GitHub → Settings → Secrets and variables → Actions**:

| Secret name | Value |
|---|---|
| `ALGOLIA_APP_ID` | From Algolia dashboard |
| `ALGOLIA_SEARCH_API_KEY` | Search-only API key (safe to expose in browser) |
| `ALGOLIA_WRITE_API_KEY` | Write API key (used only by CI scraper, never in browser) |
| `ALGOLIA_INDEX_NAME` | Index name (e.g. `boxer`) |

The next push to `main` will automatically include search in the build.

### Local development with search

Create `docs/.env.local` (already gitignored):

```
ALGOLIA_APP_ID=your_app_id
ALGOLIA_SEARCH_API_KEY=your_search_key
ALGOLIA_INDEX_NAME=boxer
```

Then run `pnpm build && pnpm serve` — search requires a production build.
