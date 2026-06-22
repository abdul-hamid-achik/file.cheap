import { defineConfig } from 'vitepress'
import { generateSidebar } from 'vitepress-sidebar'

const sidebar = generateSidebar({
  root: '/',
  excludeFiles: ['index.md'],
  excludeFolders: ['node_modules'],
  capitalizeFirstLetters: true,
  hyphenToSpace: true,
})

export default defineConfig({
  title: 'fcheap',
  description: 'Local-first stash tool for saving, restoring, compressing, and analyzing files for agent workflows',
  lang: 'en-US',
  cleanUrls: true,
  lastUpdated: true,
  themeConfig: {
    siteTitle: 'file.cheap',
    sidebar,
    nav: [
      { text: 'Guide', link: '/guide/getting-started' },
      { text: 'CLI', link: '/cli/save' },
      { text: 'MCP', link: '/mcp/overview' },
      { text: 'Studio', link: '/studio/overview' },
      { text: 'Docs', link: '/cli/docs' },
    ],
    socialLinks: [
      { icon: 'github', link: 'https://github.com/abdul-hamid-achik/file.cheap' },
    ],
    footer: {
      message: 'Released under the MIT License.',
      copyright: 'Copyright © 2026 Abdul Hamid Achik',
    },
    outline: {
      level: [2, 3],
    },
    search: {
      provider: 'local',
    },
  },
})