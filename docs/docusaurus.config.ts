import {themes as prismThemes} from 'prism-react-renderer';
import type {Config} from '@docusaurus/types';
import type * as Preset from '@docusaurus/preset-classic';
import type {PluginOptions as OpenApiPluginOptions} from 'docusaurus-plugin-openapi-docs';

const algoliaAppId = process.env.ALGOLIA_APP_ID ?? '';
const algoliaSearchKey = process.env.ALGOLIA_SEARCH_API_KEY ?? '';
const algoliaIndexName = process.env.ALGOLIA_INDEX_NAME ?? 'boxer';

const config: Config = {
  title: 'Boxer',
  tagline: 'Sandboxed container execution powered by gVisor',
  favicon: 'img/icon-wb.svg',

  future: {
    v4: true,
  },

  url: 'https://theonekeyg.github.io',
  baseUrl: '/boxer/',
  organizationName: 'theonekeyg',
  projectName: 'boxer',
  trailingSlash: false,

  onBrokenLinks: 'throw',
  markdown: {
    hooks: {
      onBrokenMarkdownLinks: 'warn',
    },
  },

  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },

  plugins: [
    function webpackNodeFallback() {
      return {
        name: 'webpack-node-fallback',
        configureWebpack() {
          return {
            resolve: {
              fallback: {
                path: false,
                fs: false,
                os: false,
                child_process: false,
                http: false,
                https: false,
                net: false,
                tls: false,
                stream: false,
                zlib: false,
                url: false,
                assert: false,
                util: false,
                crypto: false,
                buffer: false,
              },
            },
          };
        },
      };
    },
    [
      'docusaurus-plugin-openapi-docs',
      {
        id: 'openapi',
        docsPluginId: 'classic',
        config: {
          boxer: {
            specPath: '../packages/core/docs/swagger.yaml',
            outputDir: 'docs/api',
            sidebarOptions: {
              groupPathsBy: 'tag',
              categoryLinkSource: 'tag',
            },
            markdownGenerators: {
              createInfoPageMD: ({info: {title, version, license}}) => `
import Heading from "@theme/Heading";

<span className={"theme-doc-version-badge badge badge--secondary"} children={"Version: ${version}"}></span>

<Heading as={"h1"} className={"openapi__heading"} children={"${title}"}></Heading>

Boxer is a sandboxed container execution service backed by [gVisor](https://gvisor.dev/). Send an HTTP request with an image and command — Boxer pulls the image, runs the command inside an isolated gVisor sandbox, and returns stdout, stderr, exit code, and wall time.

## Base URL

\`\`\`
http://localhost:8080
\`\`\`

By default the server listens on \`:8080\`. Override with \`listen_addr\` in your config or the \`$BOXER_CONFIG\` environment variable.

## Authentication

No authentication is required. Boxer is designed to run as a local or internal service — network-level access control is left to the operator.

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| \`POST\` | \`/run\` | Execute a command in a sandboxed container |
| \`POST\` | \`/files\` | Upload a file to the file store |
| \`GET\` | \`/files\` | Download a file from the file store |
| \`GET\` | \`/healthz\` | Health check |

## Quick Start

Run a Python one-liner in an isolated sandbox:

\`\`\`bash
curl -s http://localhost:8080/run \\
  -H 'Content-Type: application/json' \\
  -d '{
    "image": "python:3.12-slim",
    "cmd": ["python3", "-c", "print(42)"]
  }'
\`\`\`

\`\`\`json
{
  "exec_id": "boxer-abc123",
  "exit_code": 0,
  "stdout": "42\\n",
  "stderr": "",
  "wall_ms": 312
}
\`\`\`

## File Workflow

To pass files into a container, upload them first then reference them in \`/run\`:

\`\`\`bash
# 1. Upload a script
curl -s http://localhost:8080/files \\
  -F 'file=@script.py' \\
  -F 'path=workspace/script.py'

# 2. Run it — the file is bind-mounted read-only at /workspace/script.py
curl -s http://localhost:8080/run \\
  -H 'Content-Type: application/json' \\
  -d '{
    "image": "python:3.12-slim",
    "cmd": ["python3", "/workspace/script.py"],
    "files": ["workspace/script.py"]
  }'
\`\`\`

To capture output files written by the container, set \`persist: true\` and retrieve them via \`GET /files?path=output/{exec_id}/{filename}\`.

## Error Handling

All errors return a JSON body with an \`error\` field:

\`\`\`json
{ "error": "image pull failed: not found" }
\`\`\`

| Status | Meaning |
|--------|---------|
| \`400\` | Invalid request body or referenced file not found |
| \`408\` | Wall-clock timeout exceeded |
| \`413\` | Upload exceeds configured size limit |
| \`500\` | Internal error — image pull failed, \`runsc\` error |
| \`507\` | stdout or stderr exceeded the configured output limit |

A \`200\` response does **not** imply the command succeeded — always check \`exit_code\`.

${license?.name ? `<div style={{"marginBottom":"var(--ifm-paragraph-margin-bottom)"}}>
  <h3 style={{"marginBottom":"0.25rem"}}>License</h3>
  <span>${license.name}</span>
</div>` : ''}
`,
            },
          },
        },
      } satisfies OpenApiPluginOptions,
    ],
  ],

  themes: ['docusaurus-theme-openapi-docs'],

  presets: [
    [
      'classic',
      {
        docs: {
          sidebarPath: './sidebars.ts',
          editUrl: ({docPath}) =>
            docPath.startsWith('api/')
              ? undefined
              : `https://github.com/theonekeyg/boxer/tree/main/docs/docs/${docPath}`,
          docItemComponent: '@theme/ApiItem',
        },
        blog: false,
        theme: {
          customCss: './src/css/custom.css',
        },
      } satisfies Preset.Options,
    ],
  ],

  themeConfig: {
    colorMode: {
      respectPrefersColorScheme: true,
    },
    navbar: {
      logo: {
        alt: 'Boxer',
        src: 'img/icon-wb.svg',
        style: {height: '32px'},
      },
      items: [
        {
          type: 'docSidebar',
          sidebarId: 'docs',
          position: 'left',
          label: 'Docs',
        },
        {
          type: 'docSidebar',
          sidebarId: 'api',
          position: 'left',
          label: 'API Reference',
        },
        {
          href: 'https://github.com/theonekeyg/boxer',
          label: 'GitHub',
          position: 'right',
        },
      ],
    },
    footer: {
      style: 'dark',
      links: [
        {
          title: 'Docs',
          items: [
            {label: 'Getting Started', to: '/docs/intro'},
            {label: 'Python SDK', to: '/docs/sdk/python'},
            {label: 'TypeScript SDK', to: '/docs/sdk/typescript'},
            {label: 'API Reference', to: '/docs/api/boxer-api'},
          ],
        },
        {
          title: 'Examples',
          items: [
            {label: 'Hello World', to: '/docs/examples/hello-world'},
            {label: 'Upload & Run', to: '/docs/examples/upload-and-run'},
            {label: 'HumanEval', to: '/docs/examples/humaneval'},
          ],
        },
        {
          title: 'More',
          items: [
            {label: 'GitHub', href: 'https://github.com/theonekeyg/boxer'},
          ],
        },
      ],
      copyright: `Copyright © ${new Date().getFullYear()} Boxer Contributors.`,
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.dracula,
      additionalLanguages: ['bash', 'python', 'json'],
    },
    ...(algoliaAppId && algoliaSearchKey ? {
      algolia: {
        appId: algoliaAppId,
        apiKey: algoliaSearchKey,
        indexName: algoliaIndexName,
        contextualSearch: true,
        searchPagePath: 'search',
      },
    } : {}),
  } satisfies Preset.ThemeConfig,
};

export default config;
