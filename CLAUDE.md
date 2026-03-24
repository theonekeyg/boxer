# Boxer — Project Instructions

## Sandbox networking

Never copy host files into container filesystems or resolv configurations.
Host files like `/etc/resolv.conf` reflect the host's local setup (e.g. systemd-resolved stub at `127.0.0.53`, a local DNS/proxy server) and are meaningless or harmful inside an isolated network namespace.

For container DNS: use explicit public resolvers (8.8.8.8 / 8.8.4.4) or allow the operator to override via config/env. Do not read from the host.

## Auto-generated files

Never directly edit auto-generated files (e.g. `packages/core/docs/swagger.yaml`, `docs/docs/api/*.mdx`, `docs/docs/api/*.json`).
Changes will be overwritten on the next generation run.

Always fix the source instead:
- For swagger.yaml: modify the Go source annotations in `packages/core/` and re-run `swag`.
- For `docs/docs/api/` MDX/JSON: re-run `pnpm docusaurus gen-api-docs all` after fixing the spec or plugin config.

If a generated file needs persistent customisation (e.g. the API intro page), implement it at the generation layer — via generator config, templates, or `markdownGenerators` hooks — not by patching the output.
