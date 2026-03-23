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
          editUrl: 'https://github.com/theonekeyg/boxer/tree/main/docs/',
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
