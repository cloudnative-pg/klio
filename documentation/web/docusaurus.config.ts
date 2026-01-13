import {themes as prismThemes} from 'prism-react-renderer';
import type {Config} from '@docusaurus/types';
import type * as Preset from '@docusaurus/preset-classic';

// This runs in Node.js - Don't use client-side code here (browser APIs, JSX...)

const config: Config = {
  title: 'Klio',
  favicon: 'img/favicon.ico',
  tagline: 'EDB Postgres Backup & Recovery Manager for CloudNativePG',

  // Set the production url of your site here
  url: 'https://enterprisedb.github.io',
  // Set the /<baseUrl>/ pathname under which your site is served
  // For GitHub pages deployment, it is often '/<projectName>/'
  baseUrl: '/',
  trailingSlash: true,

  // GitHub pages deployment config.
  organizationName: 'EnterpriseDB',
  projectName: 'klio',
  deploymentBranch: 'gh-pages',

  onBrokenLinks: 'throw',
  onBrokenMarkdownLinks: 'throw',

  // Even if you don't use internationalization, you can use this field to set
  // useful metadata like html lang. For example, if your site is Chinese, you
  // may want to replace "en" with "zh-Hans".
  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },

  markdown: {
    mermaid: true
  },

  presets: [
    [
      'classic',
      {
        docs: {
          path: 'docs',
          sidebarPath: './sidebars.ts',
            includeCurrentVersion: true, // Include the current version in the sidebar
            versions:{
              current:{
                  label: 'Dev',
                  badge: true,
                  banner: "unreleased",
              },
            }
        },
        theme: {
            customCss: './src/css/custom.css',
        },
      } satisfies Preset.Options,
    ],
  ],
  themes: [
    [
      require.resolve("@easyops-cn/docusaurus-search-local"),
      /** @type {import("@easyops-cn/docusaurus-search-local").PluginOptions} */
      ({
        hashed: true,
        docsDir: ['docs'],
        searchResultLimits: 8,
        searchResultContextMaxLength: 50,
        language: ["en"],
        // Only index headings and content
        indexBlog: false,
        indexPages: false,
      }),
    ],
    '@docusaurus/theme-mermaid',
  ],
  themeConfig: {
    announcementBar: {
      id: 'tech_preview',
      content:
        '⚠️ Klio is distributed as a Tech Preview. See <a href="https://www.enterprisedb.com/legal/EDB-Eula" target="_blank" rel="noopener noreferrer">EDB EULA</a> section 9.4 for details. ⚠️',
      backgroundColor: '#ffa500',
      textColor: '#000000',
      isCloseable: true,
    },
    navbar: {
      logo: {
        alt: 'EDB Logo',
        src: 'img/logo.svg',
      },
      items: [
          {
              type: 'docSidebar',
              sidebarId: 'docs',
              position: 'left',
              label: 'Users Docs',
          },
          {
              type: 'docSidebar',
              sidebarId: 'developers_docs',
              position: 'left',
              label: 'Developers Docs',
          },
        {
            type: 'docsVersionDropdown',
            position: 'right',
        },
      ],
    },
    footer: {
        logo: {
            alt: 'EDB Logo',
            src: "img/logo.svg",
            href: "https://enterprisedb.com",
        },
      style: 'dark',
      copyright: `© ${new Date().getFullYear()} EDB. All rights reserved.`,
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.dracula,
    },
    colorMode: {
      defaultMode: 'light',
      disableSwitch: true,
      respectPrefersColorScheme: true,
    }
  } satisfies Preset.ThemeConfig,
};

export default config;
