// @ts-check
import { themes as prismThemes } from 'prism-react-renderer';

/** @type {import('@docusaurus/types').Config} */
const config = {
  title: 'FogSense Device Simulator',
  tagline: 'One container = one simulated IoT device, described entirely by a YAML spec',
  favicon: 'img/logo.svg',

  future: {
    v4: true,
  },

  url: process.env.DOCUSAURUS_URL || 'https://aprountzos.github.io',
  baseUrl: process.env.DOCUSAURUS_BASE_URL || '/',
  trailingSlash: false,

  organizationName: 'aprountzos',
  projectName: 'fogsense-device-runtime',

  onBrokenLinks: 'throw',

  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },

  markdown: {
    mermaid: true,
  },
  themes: ['@docusaurus/theme-mermaid'],

  presets: [
    [
      'classic',
      /** @type {import('@docusaurus/preset-classic').Options} */
      ({
        docs: {
          sidebarPath: './sidebars.js',
          routeBasePath: 'docs',
        },
        blog: false,
        theme: {
          customCss: './src/css/custom.css',
        },
      }),
    ],
  ],

  themeConfig:
    /** @type {import('@docusaurus/preset-classic').ThemeConfig} */
    ({
      image: 'img/logo.svg',
      colorMode: {
        defaultMode: 'light',
        respectPrefersColorScheme: false,
      },
      navbar: {
        title: 'FogSense Simulator',
        logo: {
          alt: 'FogSense Logo',
          src: 'img/logo.svg',
        },
        items: [
          {
            type: 'docSidebar',
            sidebarId: 'docsSidebar',
            position: 'left',
            label: 'Documentation',
          },
          {
            href: 'https://github.com/aprountzos/fogsense-device-runtime',
            label: 'GitHub',
            position: 'right',
          },
          {
            href: 'https://github.com/aprountzos/fogsense-device-runtime/releases',
            label: 'Releases',
            position: 'right',
          },
        ],
      },
      footer: {
        style: 'dark',
        links: [
          {
            title: 'Documentation',
            items: [
              { label: 'Getting Started', to: '/docs/getting-started' },
              { label: 'Device Spec', to: '/docs/device-spec/overview' },
              { label: 'API Reference', to: '/docs/api/device-api' },
            ],
          },
          {
            title: 'Guides',
            items: [
              { label: 'Running with Docker', to: '/docs/guides/docker' },
              { label: 'Mock Gateway', to: '/docs/guides/mock-gateway' },
              { label: 'Testing', to: '/docs/guides/testing' },
            ],
          },
          {
            title: 'Community & Project',
            items: [
              {
                label: 'GitHub',
                href: 'https://github.com/aprountzos/fogsense-device-runtime',
              },
              {
                label: 'Contributing Guide',
                href: 'https://github.com/aprountzos/fogsense-device-runtime/blob/main/CONTRIBUTING.md',
              },
              {
                label: 'Code of Conduct',
                href: 'https://github.com/aprountzos/fogsense-device-runtime/blob/main/CODE_OF_CONDUCT.md',
              },
              {
                label: 'Security Policy',
                href: 'https://github.com/aprountzos/fogsense-device-runtime/blob/main/SECURITY.md',
              },
              {
                label: 'MIT License',
                href: 'https://github.com/aprountzos/fogsense-device-runtime/blob/main/LICENSE',
              },
            ],
          },
        ],
        copyright: `Copyright © ${new Date().getFullYear()} FogSense Project. Built with Docusaurus.`,
      },
      prism: {
        theme: prismThemes.github,
        darkTheme: prismThemes.dracula,
        additionalLanguages: ['bash', 'json', 'yaml', 'docker'],
      },
      mermaid: {
        theme: {
          light: 'neo',
          dark: 'neo-dark',
        },
      },
    }),
};

export default config;
