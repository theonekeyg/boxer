# Uploading boxer-sdk to npm

## Requirements

- [pnpm](https://pnpm.io/) installed
- npm account logged in (`npm login`)

## Steps

```bash
cd packages/sdk/typescript

# Build distribution artifacts (creates dist/)
pnpm run build

# Publish to npm
npm publish --access public
```

## Dry run (optional, to verify before publishing)

```bash
npm publish --dry-run
```

## Notes

- Bump `version` in `package.json` before each release — npm does not allow overwriting existing versions.
- API tokens can be created at: Account → Access Tokens on npmjs.com. Use an **Automation** token for CI.
- To publish with a token non-interactively: `NODE_AUTH_TOKEN=your-token npm publish --access public`.
