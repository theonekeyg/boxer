import {readFileSync} from 'fs';
import {join} from 'path';
import {themes as prismThemes} from 'prism-react-renderer';
import type {Config} from '@docusaurus/types';
import type * as Preset from '@docusaurus/preset-classic';
import type {ScalarOptions} from '@scalar/docusaurus';

const algoliaAppId = process.env.ALGOLIA_APP_ID ?? '';
const algoliaSearchKey = process.env.ALGOLIA_SEARCH_API_KEY ?? '';
const algoliaIndexName = process.env.ALGOLIA_INDEX_NAME ?? 'boxer';

const swaggerSpec = readFileSync(
  join(__dirname, '../packages/core/docs/swagger.yaml'),
  'utf8',
);

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
    [
      '@scalar/docusaurus',
      {
        label: 'API Reference',
        route: '/api/',
        configuration: {
          spec: {content: swaggerSpec},
          theme: 'default',
          customCss: `
            :root {
              --scalar-color-1: #2E343B;
              --scalar-color-accent: #FF8D28;
              --scalar-background-1: #ffffff;
              --scalar-background-2: #f8f9fa;
              --scalar-background-3: #f1f3f4;
              --scalar-border-color: #e1e4e8;
            }
            .dark-mode {
              --scalar-color-1: #e6edf3;
              --scalar-color-accent: #FF8D28;
              --scalar-background-1: #1a1f24;
              --scalar-background-2: #22272e;
              --scalar-background-3: #2d333b;
              --scalar-border-color: #373e47;
            }
            .scalar-app {
              font-family: var(--ifm-font-family-base);
            }
          `,
        },
      } satisfies ScalarOptions,
    ],
  ],

  presets: [
    [
      'classic',
      {
        docs: {
          sidebarPath: './sidebars.ts',
          editUrl: 'https://github.com/theonekeyg/boxer/tree/main/docs/',
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
            {label: 'API Reference', to: '/api/'},
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
