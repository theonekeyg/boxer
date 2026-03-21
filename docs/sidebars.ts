import type {SidebarsConfig} from '@docusaurus/plugin-content-docs';

const sidebars: SidebarsConfig = {
  docs: [
    'intro',
    {
      type: 'category',
      label: 'Guides',
      items: [
        'guides/files',
        'guides/network',
        'guides/resource-limits',
      ],
    },
    {
      type: 'category',
      label: 'Python SDK',
      items: ['sdk/python'],
    },
    {
      type: 'category',
      label: 'Examples',
      items: [
        'examples/hello-world',
        'examples/upload-and-run',
        'examples/humaneval',
      ],
    },
  ],
};

export default sidebars;
