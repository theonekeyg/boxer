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
