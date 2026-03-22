import {themes as prismThemes} from 'prism-react-renderer';
import type {Config} from '@docusaurus/types';
import type * as Preset from '@docusaurus/preset-classic';
import type * as Redocusaurus from 'redocusaurus';

const config: Config = {
  title: 'Boxer',
  tagline: 'Sandboxed container execution powered by gVisor',
  favicon: 'img/favicon.ico',

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

  presets: [
    [
      'redocusaurus',
      {
        specs: [{spec: '../packages/core/docs/swagger.yaml', route: '/api/'}],
        theme: {primaryColor: '#1a73e8'},
      } satisfies Redocusaurus.PresetEntry,
    ],
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
      title: 'Boxer',
      items: [
        {
          type: 'docSidebar',
          sidebarId: 'docs',
          position: 'left',
          label: 'Docs',
        },
        {
          to: '/api/',
          label: 'API Reference',
          position: 'left',
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
      copyright: `Copyright © ${new Date().getFullYear()} Boxer Contributors. Built with Docusaurus.`,
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.dracula,
      additionalLanguages: ['bash', 'python', 'json'],
    },
  } satisfies Preset.ThemeConfig,
};

export default config;
