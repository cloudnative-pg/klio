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

  // Even if you don't use internationalization, you can use this field to set
  // useful metadata like html lang. For example, if your site is Chinese, you
  // may want to replace "en" with "zh-Hans".
  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },

  markdown: {
    hooks: {
      onBrokenMarkdownLinks: 'throw',
    },
    mermaid: true,
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
        '⚠️ Klio is distributed as a Tech Preview. ⚠️',
      backgroundColor: '#ffa500',
      textColor: '#000000',
      isCloseable: true,
    },
    navbar: {
      logo: {
        alt: 'CloudNativePG Logo',
        src: 'img/cloudnativepg-hero.svg',
        href: 'https://cloudnative-pg.io',
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
            alt: 'CloudNativePG Logo',
            src: "img/cloudnativepg-landscape-white.png",
            href: "https://cloudnative-pg.io",
        },
      style: 'dark',
      copyright: `
      Copyright © ${new Date().getFullYear()} CloudNativePG a Series of LF Projects, LLC.<br><br>

      For website terms of use, trademark policy and other project policies please see
      <a href="https://lfprojects.org/policies/">LF Projects, LLC Policies</a>.<br>
      <a href="https://www.linuxfoundation.org/trademark-usage/">The Linux Foundation has registered trademarks and uses trademarks</a>.<br>
      <a href="https://www.postgresql.org/about/policies/trademarks">Postgres, PostgreSQL and the Slonik Logo are
        trademarks or registered trademarks of the PostgreSQL Community Association of Canada, and
        used with their permission</a>.`,
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
